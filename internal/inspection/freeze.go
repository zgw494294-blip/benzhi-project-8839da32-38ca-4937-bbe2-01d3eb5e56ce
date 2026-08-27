package inspection

import (
	"fmt"
	"sort"

	"stage-rigging-safety-release/internal/domain"
)

func ValidateFreezeCandidate(c domain.InspectionCampaign, finals []domain.MeasurementRevision) []string {
	issues := []string{}
	if c.Status != domain.StatusApproved || c.Review == nil || c.Review.Decision != "approved" {
		return []string{"任务尚未获得复核批准"}
	}
	coverage := Coverage(c)
	issues = append(issues, coverage.Blockers...)
	ids := map[string]domain.MeasurementRevision{}
	groups := map[string][]domain.MeasurementRevision{}
	for _, m := range c.Measurements {
		if _, exists := ids[m.ID]; exists {
			issues = append(issues, "存在重复实测修订 ID "+m.ID)
		}
		ids[m.ID] = m
		key := m.AssetID + "\x00" + m.CheckpointCode
		groups[key] = append(groups[key], m)
	}
	for key, chain := range groups {
		sort.Slice(chain, func(i, j int) bool { return chain[i].Revision < chain[j].Revision })
		for i, m := range chain {
			if m.Revision != i+1 {
				issues = append(issues, fmt.Sprintf("证据 %s 修订号不连续：%d", key, m.Revision))
			}
			if i == 0 && m.SupersedesID != "" {
				issues = append(issues, "首个修订错误引用前序 "+m.ID)
			}
			if i > 0 && m.SupersedesID != chain[i-1].ID {
				issues = append(issues, "修订链断裂 "+m.ID)
			}
		}
	}
	finalSeen := map[string]bool{}
	for _, m := range finals {
		key := m.AssetID + "\x00" + m.CheckpointCode
		if finalSeen[key] {
			issues = append(issues, "存在重复最终修订 "+key)
		}
		finalSeen[key] = true
		latest, ok := c.LatestMeasurement(m.AssetID, m.CheckpointCode)
		if !ok || latest.ID != m.ID {
			issues = append(issues, "候选最终修订不是最新证据 "+m.ID)
		}
	}
	for _, defect := range c.Defects {
		if defect.Status != domain.DefectClosed {
			issues = append(issues, "缺陷尚未关闭 "+defect.ID)
			continue
		}
		retest, ok := ids[defect.RetestRevisionID]
		if !ok || !retest.Passed || retest.AssetID != defect.AssetID || retest.CheckpointCode != defect.CheckpointCode {
			issues = append(issues, "缺陷合格复验引用不一致 "+defect.ID)
		}
		if len(defect.RetestRounds) == 0 || defect.RetestRounds[len(defect.RetestRounds)-1].RevisionID != defect.RetestRevisionID {
			issues = append(issues, "缺陷复验轮次与关闭引用不一致 "+defect.ID)
		}
	}
	sort.Strings(issues)
	return issues
}
