package inspection

import (
	"fmt"
	"sort"

	"stage-rigging-safety-release/internal/domain"
)

type PlanIssue struct {
	Code           string           `json:"code"`
	Message        string           `json:"message"`
	AssetType      domain.AssetType `json:"assetType,omitempty"`
	AssetCode      string           `json:"assetCode,omitempty"`
	CheckpointCode string           `json:"checkpointCode,omitempty"`
}

type ApplicabilityRow struct {
	AssetType       domain.AssetType `json:"assetType"`
	AssetCount      int              `json:"assetCount"`
	CheckpointCodes []string         `json:"checkpointCodes"`
	MissingCritical []string         `json:"missingCritical"`
}

type PlanPreflight struct {
	Valid           bool               `json:"valid"`
	Digest          string             `json:"digest"`
	CheckpointCount int                `json:"checkpointCount"`
	CriticalCount   int                `json:"criticalCount"`
	Matrix          []ApplicabilityRow `json:"matrix"`
	Issues          []PlanIssue        `json:"issues"`
}

func PreflightPlan(c domain.InspectionCampaign, checkpoints []domain.Checkpoint) PlanPreflight {
	checkpoints = domain.NormalizeCheckpoints(checkpoints)
	report := PlanPreflight{Valid: true, Digest: domain.PlanCandidateDigest(c.Assets, checkpoints), CheckpointCount: len(checkpoints), Matrix: []ApplicabilityRow{}, Issues: []PlanIssue{}}
	plan := domain.InspectionPlan{Revision: 1, Checkpoints: checkpoints}
	if err := plan.Validate(); err != nil {
		if rule, ok := err.(*domain.RuleError); ok {
			for _, detail := range rule.Details {
				report.Issues = append(report.Issues, PlanIssue{Code: rule.Code, Message: detail})
			}
		} else {
			report.Issues = append(report.Issues, PlanIssue{Code: "plan_invalid", Message: err.Error()})
		}
	}
	for _, cp := range checkpoints {
		if cp.Critical {
			report.CriticalCount++
		}
	}
	kindCounts := map[domain.AssetType]int{}
	for _, asset := range c.Assets {
		kindCounts[asset.AssetType]++
	}
	kinds := []domain.AssetType{domain.AssetCounterweightBar, domain.AssetPoweredBar, domain.AssetWinch}
	for _, kind := range kinds {
		if kindCounts[kind] == 0 {
			continue
		}
		row := ApplicabilityRow{AssetType: kind, AssetCount: kindCounts[kind], CheckpointCodes: []string{}, MissingCritical: []string{}}
		for _, cp := range checkpoints {
			if cp.AppliesTo(kind) {
				row.CheckpointCodes = append(row.CheckpointCodes, cp.Code)
			}
		}
		if len(row.CheckpointCodes) == 0 {
			report.Issues = append(report.Issues, PlanIssue{Code: "no_applicable_checkpoint", Message: "该设备类别没有适用检查点", AssetType: kind})
		}
		required := []string{"load_test", "upper_limit", "lower_limit", "wire_wear"}
		if kind == domain.AssetPoweredBar || kind == domain.AssetWinch {
			required = append(required, "brake_distance")
		}
		available := map[string]bool{}
		for _, code := range row.CheckpointCodes {
			available[code] = true
		}
		for _, code := range required {
			if !available[code] {
				row.MissingCritical = append(row.MissingCritical, code)
				report.Issues = append(report.Issues, PlanIssue{Code: "critical_checkpoint_missing", Message: fmt.Sprintf("%s 缺少关键检查点 %s", kind, code), AssetType: kind, CheckpointCode: code})
			}
		}
		report.Matrix = append(report.Matrix, row)
	}
	for _, asset := range c.Assets {
		devices := map[string]bool{}
		for _, item := range asset.SafetyDevices {
			devices[item] = true
		}
		for name, code := range map[string]string{"上限位": "upper_limit", "下限位": "lower_limit", "制动器": "brake_distance"} {
			if devices[name] {
				cp, ok := plan.Checkpoint(code)
				if !ok || !cp.AppliesTo(asset.AssetType) {
					report.Issues = append(report.Issues, PlanIssue{Code: "safety_checkpoint_mismatch", Message: fmt.Sprintf("安全装置 %s 没有匹配检查点", name), AssetCode: asset.AssetCode, AssetType: asset.AssetType, CheckpointCode: code})
				}
			}
		}
	}
	sort.Slice(report.Issues, func(i, j int) bool {
		a, b := report.Issues[i], report.Issues[j]
		return a.Code+a.AssetCode+string(a.AssetType)+a.CheckpointCode+a.Message < b.Code+b.AssetCode+string(b.AssetType)+b.CheckpointCode+b.Message
	})
	report.Valid = len(report.Issues) == 0
	return report
}
