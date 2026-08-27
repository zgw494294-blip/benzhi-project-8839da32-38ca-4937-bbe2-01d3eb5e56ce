package domain

import "time"

type OperatingPermit struct {
	PermitNumber       string    `json:"permitNumber"`
	CampaignID         string    `json:"campaignId"`
	FrozenDigest       string    `json:"frozenDigest"`
	ScopeAssetCodes    []string  `json:"scopeAssetCodes"`
	IssuedBy           string    `json:"issuedBy"`
	IssuedAt           time.Time `json:"issuedAt"`
	ValidUntil         time.Time `json:"validUntil"`
	VerificationStatus string    `json:"verificationStatus"`
}

type FrozenSnapshot struct {
	CampaignID    string                `json:"campaignId"`
	TheatreName   string                `json:"theatreName"`
	Year          int                   `json:"inspectionYear"`
	Assets        []RiggingAsset        `json:"assets"`
	Plan          InspectionPlan        `json:"plan"`
	Measurements  []MeasurementRevision `json:"measurements"`
	Defects       []DefectCase          `json:"defects"`
	Decision      ReviewDecision        `json:"reviewDecision"`
	ReviewHistory []ReviewRound         `json:"reviewHistory"`
	FrozenAt      time.Time             `json:"frozenAt"`
}

type FreezeRecord struct {
	Digest          string         `json:"digest"`
	CandidateDigest string         `json:"candidateDigest"`
	MaterialCounts  map[string]int `json:"materialCounts"`
	Snapshot        FrozenSnapshot `json:"snapshot"`
}
