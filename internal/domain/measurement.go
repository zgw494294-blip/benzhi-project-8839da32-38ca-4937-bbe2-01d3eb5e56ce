package domain

import (
	"strings"
	"time"
	"unicode"
)

func NormalizeInstrumentCode(value string) string {
	return strings.ToUpper(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value)))
}

func (m *MeasurementRevision) Normalize() {
	m.InstrumentCode = NormalizeInstrumentCode(m.InstrumentCode)
	m.CheckpointCode = strings.ToLower(strings.TrimSpace(m.CheckpointCode))
	m.Unit = strings.ToLower(strings.TrimSpace(m.Unit))
	m.Observation = strings.TrimSpace(m.Observation)
}

type MeasurementRevision struct {
	ID                     string    `json:"id"`
	CampaignID             string    `json:"campaignId"`
	AssetID                string    `json:"assetId"`
	CheckpointCode         string    `json:"checkpointCode"`
	Revision               int       `json:"revision"`
	Value                  float64   `json:"value"`
	Unit                   string    `json:"unit"`
	InstrumentCode         string    `json:"instrumentCode"`
	InstrumentCalibratedOn time.Time `json:"instrumentCalibratedOn"`
	InstrumentValidUntil   time.Time `json:"instrumentValidUntil"`
	MeasuredAt             time.Time `json:"measuredAt"`
	Observation            string    `json:"observation"`
	SupersedesID           string    `json:"supersedesId,omitempty"`
	Passed                 bool      `json:"passed"`
	Evaluation             string    `json:"evaluation"`
}

func (m MeasurementRevision) Validate(now time.Time) error {
	if strings.TrimSpace(m.ID) == "" || strings.TrimSpace(m.AssetID) == "" || strings.TrimSpace(m.CheckpointCode) == "" {
		return NewRuleError("invalid_measurement", "实测修订缺少 ID、设备或检查点")
	}
	if strings.TrimSpace(m.InstrumentCode) == "" {
		return NewRuleError("instrument_required", "必须登记仪器编号")
	}
	if m.MeasuredAt.IsZero() || m.MeasuredAt.After(now.Add(5*time.Minute)) {
		return NewRuleError("invalid_measured_at", "实测时间无效")
	}
	if m.InstrumentValidUntil.IsZero() {
		return NewRuleError("instrument_validity_required", "必须登记仪器有效期")
	}
	if m.InstrumentCalibratedOn.IsZero() {
		return NewRuleError("instrument_calibration_required", "必须登记仪器校准日期")
	}
	if m.MeasuredAt.Before(m.InstrumentCalibratedOn) {
		return NewRuleError("measurement_before_calibration", "实测时间不得早于仪器校准时间")
	}
	if m.MeasuredAt.After(m.InstrumentValidUntil) {
		return NewRuleError("instrument_expired", "实测时仪器已过有效期")
	}
	return nil
}

func (c InspectionCampaign) LatestMeasurement(assetID, checkpointCode string) (MeasurementRevision, bool) {
	var latest MeasurementRevision
	found := false
	for _, m := range c.Measurements {
		if m.AssetID == assetID && m.CheckpointCode == checkpointCode && (!found || m.Revision > latest.Revision) {
			latest, found = m, true
		}
	}
	return latest, found
}
