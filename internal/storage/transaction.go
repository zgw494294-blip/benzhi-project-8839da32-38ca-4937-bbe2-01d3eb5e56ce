package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"stage-rigging-safety-release/internal/domain"
)

func (s *SQLiteStore) Create(ctx context.Context, c *domain.InspectionCampaign, idemKey, actor, role string) error {
	if idemKey == "" {
		return domain.NewRuleError("idempotency_required", "写操作必须提供 idempotencyKey")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existingOperation string
	err = tx.QueryRowContext(ctx, `SELECT operation FROM idempotency WHERE campaign_id=? AND idem_key=?`, c.ID, idemKey).Scan(&existingOperation)
	if err == nil {
		if existingOperation == "create_campaign" {
			return nil
		}
		return domain.ErrIdempotency
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO campaigns(id,version,status,data,updated_at) VALUES(?,?,?,?,?)`, c.ID, c.Version, c.Status, data, c.UpdatedAt.Format(timeFormat))
	if err != nil {
		return fmt.Errorf("创建任务: %w", err)
	}
	if err = s.insertAudit(ctx, tx, c.ID, "campaign.created", actor, role, c.Version, true, "", nil); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO idempotency(campaign_id,idem_key,operation,response,created_at) VALUES(?,?,?,?,?)`, c.ID, idemKey, "create_campaign", data, s.now().Format(timeFormat)); err != nil {
		return err
	}
	return tx.Commit()
}

const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"

func (s *SQLiteStore) Mutate(ctx context.Context, id string, expected int, idemKey, operation, actor, role string, mutation Mutation) (*domain.InspectionCampaign, bool, error) {
	if idemKey == "" {
		return nil, false, domain.NewRuleError("idempotency_required", "写操作必须提供 idempotencyKey")
	}
	// 许可签发会在主事务提交后刷新许可查询索引，因此事务不能被
	// 请求取消提前回滚，否则已经生成的许可响应无法用于后续刷新。
	tx, err := s.db.BeginTx(context.WithoutCancel(ctx), nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	var savedOp string
	var saved []byte
	err = tx.QueryRowContext(ctx, `SELECT operation,response FROM idempotency WHERE campaign_id=? AND idem_key=?`, id, idemKey).Scan(&savedOp, &saved)
	if err == nil {
		if savedOp != operation {
			return nil, false, domain.ErrIdempotency
		}
		c, e := decodeCampaign(saved)
		return c, true, e
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	var version int
	var raw []byte
	err = tx.QueryRowContext(ctx, `SELECT version,data FROM campaigns WHERE id=?`, id).Scan(&version, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, domain.ErrNotFound
	}
	if err != nil {
		return nil, false, err
	}
	if version != expected {
		return nil, false, domain.ErrConflict
	}
	c, err := decodeCampaign(raw)
	if err != nil {
		return nil, false, err
	}
	details, err := mutation(c)
	if err != nil {
		return nil, false, err
	}
	writeCtx := ctx
	if c.Permit != nil {
		writeCtx = context.WithoutCancel(ctx)
	}
	if err = c.ValidateReferences(); err != nil {
		return nil, false, err
	}
	c.Version++
	c.UpdatedAt = s.now()
	data, err := json.Marshal(c)
	if err != nil {
		return nil, false, err
	}
	result, err := tx.ExecContext(writeCtx, `UPDATE campaigns SET version=?,status=?,data=?,updated_at=? WHERE id=? AND version=?`, c.Version, c.Status, data, c.UpdatedAt.Format(timeFormat), id, expected)
	if err != nil {
		return nil, false, err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return nil, false, domain.ErrConflict
	}
	detailJSON, _ := json.Marshal(details)
	auditOperation := operation
	if at := strings.IndexByte(auditOperation, '|'); at >= 0 {
		auditOperation = auditOperation[:at]
	}
	if err = s.insertAudit(writeCtx, tx, id, auditOperation, actor, role, c.Version, true, "", detailJSON); err != nil {
		return nil, false, err
	}
	_, err = tx.ExecContext(writeCtx, `INSERT INTO idempotency(campaign_id,idem_key,operation,response,created_at) VALUES(?,?,?,?,?)`, id, idemKey, operation, data, s.now().Format(timeFormat))
	if err != nil {
		return nil, false, err
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	if c.Permit != nil {
		_, err = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO permits(permit_number,campaign_id,frozen_digest,issued_at) VALUES(?,?,?,?)`, c.Permit.PermitNumber, id, c.Permit.FrozenDigest, c.Permit.IssuedAt.Format(timeFormat))
		if err != nil {
			return nil, false, err
		}
	}
	return c, false, nil
}
