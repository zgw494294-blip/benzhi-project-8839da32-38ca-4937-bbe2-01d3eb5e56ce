package inspection

import (
	"stage-rigging-safety-release/internal/domain"
	"testing"
	"time"
)

func campaignFixture() domain.InspectionCampaign {
	cp := domain.Checkpoint{Code: "brake", Name: "制动距离", AssetTypes: []domain.AssetType{domain.AssetWinch}, Unit: "mm", Comparison: domain.CompareMaximum, Threshold: 80, Critical: true}
	return domain.InspectionCampaign{ID: "C", Status: domain.StatusExecuting, Assets: []domain.RiggingAsset{{ID: "A", AssetCode: "JY-1", AssetType: domain.AssetWinch, RatedLoadKg: 500}}, Plans: []domain.InspectionPlan{{Revision: 1, Status: "confirmed", Checkpoints: []domain.Checkpoint{cp}}}, Measurements: []domain.MeasurementRevision{}, Defects: []domain.DefectCase{}}
}

func TestEvaluateThresholdAndExpiredInstrument(t *testing.T) {
	cp := campaignFixture().Plans[0].Checkpoints[0]
	at := time.Now().UTC()
	m := domain.MeasurementRevision{Value: 79, Unit: "mm", MeasuredAt: at, InstrumentValidUntil: at.Add(time.Hour)}
	if result := Evaluate(cp, m); !result.Passed {
		t.Fatalf("阈值内应合格: %+v", result)
	}
	m.Value = 81
	if result := Evaluate(cp, m); result.Passed || result.Severity != domain.SeverityCritical {
		t.Fatalf("超限应形成关键不合格: %+v", result)
	}
	m.Value = 70
	m.InstrumentValidUntil = at.Add(-time.Hour)
	if result := Evaluate(cp, m); result.Passed || result.Reason != "实测时仪器已过有效期" {
		t.Fatalf("过期仪器应阻断: %+v", result)
	}
}

func TestCoverageRequiresLatestPassingEvidence(t *testing.T) {
	c := campaignFixture()
	r := Coverage(c)
	if r.Complete || len(r.Assets[0].MissingCodes) != 1 {
		t.Fatalf("缺失证据应不完整: %+v", r)
	}
	c.Measurements = append(c.Measurements, domain.MeasurementRevision{ID: "M1", AssetID: "A", CheckpointCode: "brake", Revision: 1, Passed: false})
	r = Coverage(c)
	if r.Complete || len(r.Assets[0].FailedCodes) != 1 {
		t.Fatalf("失败证据应阻断: %+v", r)
	}
	c.Measurements = append(c.Measurements, domain.MeasurementRevision{ID: "M2", AssetID: "A", CheckpointCode: "brake", Revision: 2, Passed: true})
	r = Coverage(c)
	if !r.Complete || r.Assets[0].Percentage != 100 {
		t.Fatalf("最新合格修订应完成覆盖: %+v", r)
	}
}
