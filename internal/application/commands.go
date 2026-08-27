package application

import (
	"stage-rigging-safety-release/internal/domain"
	"time"
)

type Metadata struct {
	ExpectedVersion int    `json:"expectedVersion"`
	IdempotencyKey  string `json:"idempotencyKey"`
	Actor           string `json:"actor"`
	Role            string `json:"role"`
}
type CreateCampaignCommand struct {
	ID             string `json:"id,omitempty"`
	TheatreName    string `json:"theatreName"`
	InspectionYear int    `json:"inspectionYear"`
	LeadInspector  string `json:"leadInspector"`
	IdempotencyKey string `json:"idempotencyKey"`
	Actor          string `json:"actor"`
	Role           string `json:"role"`
}
type AddAssetCommand struct {
	Metadata
	Asset domain.RiggingAsset `json:"asset"`
}
type AddAssetsCommand struct {
	Metadata
	Assets []domain.RiggingAsset `json:"assets"`
}
type ConfirmPlanCommand struct {
	Metadata
	Checkpoints   []domain.Checkpoint `json:"checkpoints"`
	PreviewDigest string              `json:"previewDigest"`
}
type PlanPreflightCommand struct {
	ExpectedVersion int                 `json:"expectedVersion"`
	Actor           string              `json:"actor"`
	Role            string              `json:"role"`
	Checkpoints     []domain.Checkpoint `json:"checkpoints"`
}
type MeasurementInput struct {
	CheckpointCode         string    `json:"checkpointCode"`
	Value                  float64   `json:"value"`
	Unit                   string    `json:"unit"`
	InstrumentCode         string    `json:"instrumentCode"`
	InstrumentCalibratedOn time.Time `json:"instrumentCalibratedOn"`
	InstrumentValidUntil   time.Time `json:"instrumentValidUntil"`
	MeasuredAt             time.Time `json:"measuredAt"`
	Observation            string    `json:"observation"`
}
type SubmitMeasurementCommand struct {
	Metadata
	AssetID                string             `json:"assetId"`
	Measurements           []MeasurementInput `json:"measurements,omitempty"`
	CheckpointCode         string             `json:"checkpointCode"`
	Value                  float64            `json:"value"`
	Unit                   string             `json:"unit"`
	InstrumentCode         string             `json:"instrumentCode"`
	InstrumentCalibratedOn time.Time          `json:"instrumentCalibratedOn"`
	InstrumentValidUntil   time.Time          `json:"instrumentValidUntil"`
	MeasuredAt             time.Time          `json:"measuredAt"`
	Observation            string             `json:"observation"`
}
type RemedyCommand struct {
	Metadata
	DefectID string `json:"defectId"`
	Remedy   string `json:"remedy"`
	Owner    string `json:"owner"`
}
type RetestCommand struct {
	Metadata
	DefectID               string    `json:"defectId"`
	Value                  float64   `json:"value"`
	Unit                   string    `json:"unit"`
	InstrumentCode         string    `json:"instrumentCode"`
	InstrumentCalibratedOn time.Time `json:"instrumentCalibratedOn"`
	InstrumentValidUntil   time.Time `json:"instrumentValidUntil"`
	MeasuredAt             time.Time `json:"measuredAt"`
	Observation            string    `json:"observation"`
}
type ReturnResolution struct {
	ItemID              string   `json:"itemId"`
	HandlingNote        string   `json:"handlingNote"`
	EvidenceRevisionIDs []string `json:"evidenceRevisionIds"`
}
type ReviewSubmitCommand struct {
	Metadata
	Resolutions []ReturnResolution `json:"resolutions,omitempty"`
}
type ReviewReturnItemInput struct {
	Category       string `json:"category"`
	Reason         string `json:"reason"`
	AssetID        string `json:"assetId,omitempty"`
	CheckpointCode string `json:"checkpointCode,omitempty"`
}
type ReviewDecisionCommand struct {
	Metadata
	Decision string                  `json:"decision"`
	Reason   string                  `json:"reason"`
	Items    []ReviewReturnItemInput `json:"items,omitempty"`
}
type FreezeCommand struct {
	Metadata
	CandidateDigest string `json:"candidateDigest"`
}
type IssuePermitCommand struct {
	Metadata
	ValidUntil time.Time `json:"validUntil"`
}
