package domain

import (
	"fmt"
	"strings"
	"time"
)

type InspectionCampaign struct {
	ID             string                `json:"id"`
	TheatreName    string                `json:"theatreName"`
	InspectionYear int                   `json:"inspectionYear"`
	LeadInspector  string                `json:"leadInspector"`
	Status         CampaignStatus        `json:"status"`
	Version        int                   `json:"version"`
	CreatedAt      time.Time             `json:"createdAt"`
	UpdatedAt      time.Time             `json:"updatedAt"`
	Assets         []RiggingAsset        `json:"assets"`
	Plans          []InspectionPlan      `json:"plans"`
	Measurements   []MeasurementRevision `json:"measurements"`
	Defects        []DefectCase          `json:"defects"`
	Review         *ReviewDecision       `json:"review,omitempty"`
	ReviewHistory  []ReviewRound         `json:"reviewHistory"`
	Freeze         *FreezeRecord         `json:"freeze,omitempty"`
	Permit         *OperatingPermit      `json:"permit,omitempty"`
}

func NewCampaign(id, theatre string, year int, lead string, now time.Time) (*InspectionCampaign, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(theatre) == "" || strings.TrimSpace(lead) == "" {
		return nil, NewRuleError("campaign_fields_required", "剧场、负责人和任务 ID 不能为空")
	}
	if year < 2000 || year > now.Year()+2 {
		return nil, NewRuleError("invalid_year", "检验年度超出允许范围")
	}
	return &InspectionCampaign{ID: id, TheatreName: strings.TrimSpace(theatre), InspectionYear: year, LeadInspector: strings.TrimSpace(lead), Status: StatusDraft, Version: 1, CreatedAt: now, UpdatedAt: now, Assets: []RiggingAsset{}, Plans: []InspectionPlan{}, Measurements: []MeasurementRevision{}, Defects: []DefectCase{}, ReviewHistory: []ReviewRound{}}, nil
}

func (c InspectionCampaign) ActivePlan() (InspectionPlan, bool) {
	var best InspectionPlan
	found := false
	for _, p := range c.Plans {
		if p.Status == "confirmed" && (!found || p.Revision > best.Revision) {
			best, found = p, true
		}
	}
	return best, found
}

func (c InspectionCampaign) Asset(id string) (RiggingAsset, bool) {
	for _, a := range c.Assets {
		if a.ID == id {
			return a, true
		}
	}
	return RiggingAsset{}, false
}

func (c *InspectionCampaign) Defect(id string) (*DefectCase, bool) {
	for i := range c.Defects {
		if c.Defects[i].ID == id {
			return &c.Defects[i], true
		}
	}
	return nil, false
}

func (c InspectionCampaign) ValidateReferences() error {
	assets := map[string]bool{}
	for _, a := range c.Assets {
		assets[a.ID] = true
	}
	for _, m := range c.Measurements {
		if !assets[m.AssetID] {
			return fmt.Errorf("%w: 实测引用未知设备 %s", ErrValidation, m.AssetID)
		}
	}
	for _, d := range c.Defects {
		if !assets[d.AssetID] {
			return fmt.Errorf("%w: 缺陷引用未知设备 %s", ErrValidation, d.AssetID)
		}
	}
	return nil
}
