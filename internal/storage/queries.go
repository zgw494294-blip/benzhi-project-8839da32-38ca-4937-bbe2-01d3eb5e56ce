package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"stage-rigging-safety-release/internal/domain"
)

func decodeCampaign(data []byte) (*domain.InspectionCampaign, error) {
	var campaign domain.InspectionCampaign
	if err := json.Unmarshal(data, &campaign); err != nil {
		return nil, fmt.Errorf("解析任务数据: %w", err)
	}
	if err := campaign.ValidateReferences(); err != nil {
		return nil, err
	}
	return &campaign, nil
}

func (s *SQLiteStore) Get(ctx context.Context, id string) (*domain.InspectionCampaign, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT data FROM campaigns WHERE id=?`, id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询任务: %w", err)
	}
	return decodeCampaign(data)
}

func (s *SQLiteStore) List(ctx context.Context) ([]domain.InspectionCampaign, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT data FROM campaigns ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("列出任务: %w", err)
	}
	defer rows.Close()
	result := []domain.InspectionCampaign{}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		campaign, err := decodeCampaign(data)
		if err != nil {
			return nil, err
		}
		result = append(result, *campaign)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) FindPermit(ctx context.Context, permit string) (*domain.InspectionCampaign, error) {
	var campaignID string
	err := s.db.QueryRowContext(ctx, `SELECT campaign_id FROM permits WHERE permit_number=?`, permit).Scan(&campaignID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, campaignID)
}
