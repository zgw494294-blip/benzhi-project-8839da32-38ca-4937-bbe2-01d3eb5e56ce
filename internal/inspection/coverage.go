package inspection

import "stage-rigging-safety-release/internal/domain"

type AssetCoverage struct {
	AssetID      string   `json:"assetId"`
	AssetCode    string   `json:"assetCode"`
	Required     int      `json:"required"`
	Completed    int      `json:"completed"`
	Passing      int      `json:"passing"`
	Percentage   int      `json:"percentage"`
	MissingCodes []string `json:"missingCodes"`
	FailedCodes  []string `json:"failedCodes"`
}

type CoverageReport struct {
	Assets   []AssetCoverage `json:"assets"`
	Complete bool            `json:"complete"`
	Blockers []string        `json:"blockers"`
}

func Coverage(c domain.InspectionCampaign) CoverageReport {
	report := CoverageReport{Complete: true, Assets: []AssetCoverage{}, Blockers: []string{}}
	plan, ok := c.ActivePlan()
	if !ok {
		return CoverageReport{Complete: false, Blockers: []string{"尚未确认执行方案"}, Assets: []AssetCoverage{}}
	}
	for _, asset := range c.Assets {
		row := AssetCoverage{AssetID: asset.ID, AssetCode: asset.AssetCode, MissingCodes: []string{}, FailedCodes: []string{}}
		for _, cp := range plan.Checkpoints {
			if !cp.AppliesTo(asset.AssetType) {
				continue
			}
			row.Required++
			m, found := c.LatestMeasurement(asset.ID, cp.Code)
			if !found {
				row.MissingCodes = append(row.MissingCodes, cp.Code)
				continue
			}
			row.Completed++
			if m.Passed {
				row.Passing++
			} else {
				row.FailedCodes = append(row.FailedCodes, cp.Code)
			}
		}
		if row.Required > 0 {
			row.Percentage = row.Completed * 100 / row.Required
		}
		if len(row.MissingCodes) > 0 {
			report.Complete = false
			report.Blockers = append(report.Blockers, asset.AssetCode+" 缺少必需证据")
		}
		if len(row.FailedCodes) > 0 {
			report.Complete = false
			report.Blockers = append(report.Blockers, asset.AssetCode+" 存在未通过结果")
		}
		report.Assets = append(report.Assets, row)
	}
	if len(c.Assets) == 0 {
		report.Complete = false
		report.Blockers = append(report.Blockers, "任务范围没有设备")
	}
	return report
}
