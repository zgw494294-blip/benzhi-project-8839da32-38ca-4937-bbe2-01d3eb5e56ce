package inspection

import (
	"fmt"
	"stage-rigging-safety-release/internal/domain"
	"time"
)

type Readiness struct {
	Ready    bool           `json:"ready"`
	Blockers []string       `json:"blockers"`
	Coverage CoverageReport `json:"coverage"`
}

func ReviewReadiness(c domain.InspectionCampaign) Readiness {
	return ReviewReadinessAt(c, time.Now().UTC())
}

func ReviewReadinessAt(c domain.InspectionCampaign, now time.Time) Readiness {
	coverage := Coverage(c)
	r := Readiness{Ready: coverage.Complete, Coverage: coverage, Blockers: append([]string(nil), coverage.Blockers...)}
	for _, d := range c.Defects {
		if d.Status != domain.DefectClosed {
			r.Ready = false
			r.Blockers = append(r.Blockers, fmt.Sprintf("缺陷 %s 尚未闭环：%s", d.ID, d.Reason))
		}
	}
	plan, hasPlan := c.ActivePlan()
	if hasPlan {
		for _, asset := range c.Assets {
			for _, cp := range plan.Checkpoints {
				if !cp.AppliesTo(asset.AssetType) {
					continue
				}
				m, ok := c.LatestMeasurement(asset.ID, cp.Code)
				if !ok {
					continue
				}
				if _, _, err := ValidateMeasurement(c, m, now); err != nil {
					r.Ready = false
					r.Blockers = append(r.Blockers, fmt.Sprintf("%s / %s / %s：%s", asset.AssetCode, cp.Code, m.InstrumentCode, err.Error()))
					continue
				}
				if err := ValidateInstrument(c, cp, m); err != nil {
					r.Ready = false
					r.Blockers = append(r.Blockers, fmt.Sprintf("%s / %s / %s：%s", asset.AssetCode, cp.Code, m.InstrumentCode, err.Error()))
				}
			}
		}
	}
	if c.Review != nil && c.Review.Decision == "returned" {
		for _, item := range c.Review.Items {
			if item.HandlingNote == "" || len(item.ResolutionRevisionIDs) == 0 || item.ResolvedAt == nil {
				r.Ready = false
				r.Blockers = append(r.Blockers, fmt.Sprintf("退回项 %s 尚未销项（设备 %s，检查点 %s）", item.ID, item.AssetID, item.CheckpointCode))
			}
		}
	}
	if c.Status != domain.StatusExecuting && c.Status != domain.StatusReviewPending {
		r.Ready = false
		r.Blockers = append(r.Blockers, "任务不处于执行或待复核状态")
	}
	return r
}
