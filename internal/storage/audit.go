package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"stage-rigging-safety-release/internal/domain"
)

func (s *SQLiteStore) insertAudit(ctx context.Context, tx *sql.Tx, campaignID, action, actor, role string, version int, accepted bool, reason string, details []byte) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_events(campaign_id,action,actor,role,version,accepted,reason,details,occurred_at) VALUES(?,?,?,?,?,?,?,?,?)`, campaignID, action, actor, role, version, accepted, reason, details, s.now().Format(timeFormat))
	return err
}

func (s *SQLiteStore) AppendDecision(ctx context.Context, campaignID, action, actor, role string, accepted bool, reason string, version int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = s.insertAudit(ctx, tx, campaignID, action, actor, role, version, accepted, reason, nil); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) Timeline(ctx context.Context, campaignID string) ([]domain.AuditEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT sequence,campaign_id,action,actor,role,version,accepted,reason,details,occurred_at FROM audit_events WHERE campaign_id=? ORDER BY sequence`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("查询审计时间线: %w", err)
	}
	defer rows.Close()
	result := []domain.AuditEvent{}
	for rows.Next() {
		var e domain.AuditEvent
		var accepted int
		var occurred string
		var details []byte
		if err := rows.Scan(&e.Sequence, &e.CampaignID, &e.Action, &e.Actor, &e.Role, &e.Version, &accepted, &e.Reason, &details, &occurred); err != nil {
			return nil, err
		}
		e.Accepted = accepted == 1
		e.Details = json.RawMessage(details)
		e.OccurredAt, _ = time.Parse(timeFormat, occurred)
		result = append(result, e)
	}
	return result, rows.Err()
}
