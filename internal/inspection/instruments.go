package inspection

import (
	"fmt"
	"sort"
	"time"

	"stage-rigging-safety-release/internal/domain"
)

type InstrumentUse struct {
	InstrumentCode string    `json:"instrumentCode"`
	AssetID        string    `json:"assetId"`
	AssetCode      string    `json:"assetCode"`
	CheckpointCode string    `json:"checkpointCode"`
	RevisionID     string    `json:"revisionId"`
	MeasuredAt     time.Time `json:"measuredAt"`
	CalibratedOn   time.Time `json:"calibratedOn"`
	ValidUntil     time.Time `json:"validUntil"`
	Status         string    `json:"status"`
}

type InstrumentSummary struct {
	InstrumentCode   string          `json:"instrumentCode"`
	LatestValidUntil time.Time       `json:"latestValidUntil"`
	Status           string          `json:"status"`
	Uses             []InstrumentUse `json:"uses"`
	Conflicts        []string        `json:"conflicts,omitempty"`
}

func ValidateInstrument(c domain.InspectionCampaign, cp domain.Checkpoint, m domain.MeasurementRevision) error {
	if m.InstrumentValidUntil.Sub(m.InstrumentCalibratedOn) > time.Duration(cp.InstrumentMaxAge)*24*time.Hour {
		return domain.NewRuleError("instrument_cycle_too_long", "仪器校准周期超过锁定检查点上限", fmt.Sprintf("%s 上限 %d 天", cp.Code, cp.InstrumentMaxAge))
	}
	for _, prior := range c.Measurements {
		if domain.NormalizeInstrumentCode(prior.InstrumentCode) != m.InstrumentCode || prior.InstrumentCalibratedOn.IsZero() {
			continue
		}
		if prior.InstrumentCalibratedOn.Equal(m.InstrumentCalibratedOn) && !prior.InstrumentValidUntil.Equal(m.InstrumentValidUntil) {
			return domain.NewRuleError("instrument_calibration_conflict", "同一仪器在相同校准日期登记了不同有效期", m.InstrumentCode)
		}
		if m.InstrumentCalibratedOn.After(prior.InstrumentCalibratedOn) && m.InstrumentValidUntil.Before(prior.InstrumentValidUntil) {
			return domain.NewRuleError("instrument_validity_regression", "同一仪器的有效期发生倒退", m.InstrumentCode)
		}
	}
	return nil
}

func InstrumentSummaries(c domain.InspectionCampaign, now time.Time) []InstrumentSummary {
	assets := map[string]string{}
	for _, asset := range c.Assets {
		assets[asset.ID] = asset.AssetCode
	}
	grouped := map[string]*InstrumentSummary{}
	for _, m := range c.Measurements {
		code := domain.NormalizeInstrumentCode(m.InstrumentCode)
		if code == "" {
			continue
		}
		status := "valid"
		if m.MeasuredAt.After(m.InstrumentValidUntil) {
			status = "expired_at_measurement"
		} else if !m.InstrumentValidUntil.After(now) {
			status = "expired"
		} else if m.InstrumentValidUntil.Before(now.Add(30 * 24 * time.Hour)) {
			status = "expiring_soon"
		}
		item := grouped[code]
		if item == nil {
			item = &InstrumentSummary{InstrumentCode: code, Status: status, Uses: []InstrumentUse{}}
			grouped[code] = item
		}
		if m.InstrumentValidUntil.After(item.LatestValidUntil) {
			item.LatestValidUntil = m.InstrumentValidUntil
		}
		if status != "valid" {
			item.Status = status
		}
		item.Uses = append(item.Uses, InstrumentUse{InstrumentCode: code, AssetID: m.AssetID, AssetCode: assets[m.AssetID], CheckpointCode: m.CheckpointCode, RevisionID: m.ID, MeasuredAt: m.MeasuredAt, CalibratedOn: m.InstrumentCalibratedOn, ValidUntil: m.InstrumentValidUntil, Status: status})
	}
	result := make([]InstrumentSummary, 0, len(grouped))
	for _, item := range grouped {
		sort.Slice(item.Uses, func(i, j int) bool { return item.Uses[i].MeasuredAt.Before(item.Uses[j].MeasuredAt) })
		for i := range item.Uses {
			for j := i + 1; j < len(item.Uses); j++ {
				a, b := item.Uses[i], item.Uses[j]
				if a.CalibratedOn.Equal(b.CalibratedOn) && !a.ValidUntil.Equal(b.ValidUntil) {
					item.Conflicts = append(item.Conflicts, "相同校准日期对应不同有效期")
				}
				if b.CalibratedOn.After(a.CalibratedOn) && b.ValidUntil.Before(a.ValidUntil) || a.CalibratedOn.After(b.CalibratedOn) && a.ValidUntil.Before(b.ValidUntil) {
					item.Conflicts = append(item.Conflicts, "有效期随校准更新发生倒退")
				}
			}
		}
		if len(item.Conflicts) > 0 {
			item.Status = "conflict"
		}
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].InstrumentCode < result[j].InstrumentCode })
	return result
}
