package inspection

import (
	"fmt"
	"strings"
	"time"

	"stage-rigging-safety-release/internal/domain"
)

type Evaluation struct {
	Passed   bool                  `json:"passed"`
	Reason   string                `json:"reason"`
	Severity domain.DefectSeverity `json:"severity,omitempty"`
}

func Evaluate(cp domain.Checkpoint, measurement domain.MeasurementRevision) Evaluation {
	if !strings.EqualFold(strings.TrimSpace(cp.Unit), strings.TrimSpace(measurement.Unit)) {
		return Evaluation{Reason: fmt.Sprintf("单位不一致：要求 %s，收到 %s", cp.Unit, measurement.Unit), Severity: severity(cp)}
	}
	if measurement.InstrumentValidUntil.Before(measurement.MeasuredAt) {
		return Evaluation{Reason: "实测时仪器已过有效期", Severity: domain.SeverityCritical}
	}
	passed := false
	switch cp.Comparison {
	case domain.CompareMaximum:
		passed = measurement.Value <= cp.Threshold
	case domain.CompareMinimum:
		passed = measurement.Value >= cp.Threshold
	case domain.CompareBoolean:
		passed = measurement.Value == cp.Threshold
	}
	if passed {
		return Evaluation{Passed: true, Reason: "实测值符合锁定阈值"}
	}
	return Evaluation{Reason: fmt.Sprintf("实测值 %.3f %s 不符合阈值 %.3f %s", measurement.Value, cp.Unit, cp.Threshold, cp.Unit), Severity: severity(cp)}
}

func ValidateMeasurement(c domain.InspectionCampaign, m domain.MeasurementRevision, now time.Time) (domain.Checkpoint, domain.RiggingAsset, error) {
	if err := m.Validate(now); err != nil {
		return domain.Checkpoint{}, domain.RiggingAsset{}, err
	}
	asset, ok := c.Asset(m.AssetID)
	if !ok {
		return domain.Checkpoint{}, domain.RiggingAsset{}, domain.NewRuleError("asset_not_found", "实测设备不在任务范围内")
	}
	plan, ok := c.ActivePlan()
	if !ok {
		return domain.Checkpoint{}, domain.RiggingAsset{}, domain.NewRuleError("plan_required", "必须先确认检验方案")
	}
	cp, ok := plan.Checkpoint(m.CheckpointCode)
	if !ok || !cp.AppliesTo(asset.AssetType) {
		return domain.Checkpoint{}, domain.RiggingAsset{}, domain.NewRuleError("checkpoint_not_applicable", "检查点不适用于该设备")
	}
	if !strings.EqualFold(strings.TrimSpace(cp.Unit), strings.TrimSpace(m.Unit)) {
		return domain.Checkpoint{}, domain.RiggingAsset{}, domain.NewRuleError("measurement_unit_mismatch", "实测单位与锁定检查点不一致", fmt.Sprintf("要求 %s，收到 %s", cp.Unit, m.Unit))
	}
	return cp, asset, nil
}

func severity(cp domain.Checkpoint) domain.DefectSeverity {
	if cp.Critical {
		return domain.SeverityCritical
	}
	return domain.SeverityMajor
}
