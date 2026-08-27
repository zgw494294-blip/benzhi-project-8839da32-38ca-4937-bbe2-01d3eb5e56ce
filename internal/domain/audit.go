package domain

import (
	"encoding/json"
	"time"
)

type AuditEvent struct {
	Sequence   int64           `json:"sequence"`
	CampaignID string          `json:"campaignId"`
	Action     string          `json:"action"`
	Actor      string          `json:"actor"`
	Role       string          `json:"role"`
	Version    int             `json:"version"`
	Accepted   bool            `json:"accepted"`
	Reason     string          `json:"reason,omitempty"`
	Details    json.RawMessage `json:"details,omitempty"`
	OccurredAt time.Time       `json:"occurredAt"`
}

type ReviewDecision struct {
	Decision string             `json:"decision"`
	Reviewer string             `json:"reviewer"`
	Reason   string             `json:"reason,omitempty"`
	At       time.Time          `json:"at"`
	Round    int                `json:"round"`
	Items    []ReviewReturnItem `json:"items,omitempty"`
}

type ReviewReturnItem struct {
	ID                       string     `json:"id"`
	Category                 string     `json:"category"`
	Reason                   string     `json:"reason"`
	AssetID                  string     `json:"assetId,omitempty"`
	CheckpointCode           string     `json:"checkpointCode,omitempty"`
	EvidenceRevisionAtReturn string     `json:"evidenceRevisionAtReturn,omitempty"`
	ReturnedAt               time.Time  `json:"returnedAt"`
	HandlingNote             string     `json:"handlingNote,omitempty"`
	ResolutionRevisionIDs    []string   `json:"resolutionRevisionIds,omitempty"`
	ResolvedAt               *time.Time `json:"resolvedAt,omitempty"`
}

type ReviewRound struct {
	Round       int             `json:"round"`
	SubmittedAt time.Time       `json:"submittedAt"`
	SubmittedBy string          `json:"submittedBy"`
	Decision    *ReviewDecision `json:"decision,omitempty"`
}
