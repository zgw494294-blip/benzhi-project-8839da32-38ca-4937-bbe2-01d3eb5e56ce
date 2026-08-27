package application

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"stage-rigging-safety-release/internal/domain"
	"stage-rigging-safety-release/internal/inspection"
)

type AssetBatchResult struct {
	*domain.InspectionCampaign
	AddedCount    int                        `json:"addedCount"`
	CountsByType  map[domain.AssetType]int   `json:"countsByType"`
	LatestVersion int                        `json:"latestVersion"`
	Campaign      *domain.InspectionCampaign `json:"campaign"`
}

func (s *Service) AddAssets(ctx context.Context, id string, cmd AddAssetsCommand) (AssetBatchResult, error) {
	if err := requireRole(cmd.Role, "inspector"); err != nil {
		return AssetBatchResult{}, err
	}
	if len(cmd.Assets) > 100 {
		return AssetBatchResult{}, domain.NewRuleError("asset_batch_too_large", "单批设备不能超过 100 行")
	}
	op := operationSignature("assets.batch_added", cmd.Assets)
	for i := range cmd.Assets {
		if cmd.Assets[i].ID == "" {
			cmd.Assets[i].ID = identifier("AST")
		}
	}
	c, err := s.mutate(ctx, id, cmd.Metadata, op, func(c *domain.InspectionCampaign) (any, error) {
		if err := c.AddAssets(cmd.Assets); err != nil {
			return nil, err
		}
		return map[string]any{"addedCount": len(cmd.Assets), "assets": cmd.Assets}, nil
	})
	if err != nil {
		return AssetBatchResult{}, err
	}
	counts := map[domain.AssetType]int{}
	for _, asset := range c.Assets {
		counts[asset.AssetType]++
	}
	return AssetBatchResult{InspectionCampaign: c, AddedCount: len(cmd.Assets), CountsByType: counts, LatestVersion: c.Version, Campaign: c}, nil
}

func (s *Service) PreflightPlan(ctx context.Context, id string, cmd PlanPreflightCommand) (inspection.PlanPreflight, error) {
	if err := requireRole(cmd.Role, "inspector"); err != nil {
		return inspection.PlanPreflight{}, err
	}
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return inspection.PlanPreflight{}, err
	}
	if c.Version != cmd.ExpectedVersion {
		return inspection.PlanPreflight{}, domain.ErrConflict
	}
	if c.Status != domain.StatusDraft {
		return inspection.PlanPreflight{}, domain.ErrInvalidState
	}
	if len(c.Assets) == 0 {
		return inspection.PlanPreflight{}, domain.NewRuleError("asset_required", "至少登记一台设备")
	}
	return inspection.PreflightPlan(*c, cmd.Checkpoints), nil
}

type ChecklistItem struct {
	Checkpoint domain.Checkpoint           `json:"checkpoint"`
	Latest     *domain.MeasurementRevision `json:"latest,omitempty"`
	Missing    bool                        `json:"missing"`
}

type DeviceChecklist struct {
	Asset domain.RiggingAsset `json:"asset"`
	Items []ChecklistItem     `json:"items"`
}

func (s *Service) DeviceChecklist(ctx context.Context, id, assetID string) (DeviceChecklist, error) {
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return DeviceChecklist{}, err
	}
	asset, ok := c.Asset(assetID)
	if !ok {
		return DeviceChecklist{}, domain.ErrNotFound
	}
	plan, ok := c.ActivePlan()
	if !ok {
		return DeviceChecklist{}, domain.NewRuleError("plan_required", "尚未锁定检验方案")
	}
	result := DeviceChecklist{Asset: asset, Items: []ChecklistItem{}}
	for _, cp := range plan.Checkpoints {
		if !cp.AppliesTo(asset.AssetType) {
			continue
		}
		item := ChecklistItem{Checkpoint: cp, Missing: true}
		if latest, found := c.LatestMeasurement(asset.ID, cp.Code); found {
			copy := latest
			item.Latest, item.Missing = &copy, false
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

type MeasurementItemResult struct {
	CheckpointCode string `json:"checkpointCode"`
	RevisionID     string `json:"revisionId"`
	Revision       int    `json:"revision"`
	Passed         bool   `json:"passed"`
	Evaluation     string `json:"evaluation"`
	DefectID       string `json:"defectId,omitempty"`
}

type MeasurementBatchResult struct {
	*domain.InspectionCampaign
	Items              []MeasurementItemResult    `json:"items"`
	CompletenessBefore int                        `json:"completenessBefore"`
	CompletenessAfter  int                        `json:"completenessAfter"`
	LatestVersion      int                        `json:"latestVersion"`
	Campaign           *domain.InspectionCampaign `json:"campaign"`
}

func assetCompleteness(c domain.InspectionCampaign, assetID string) int {
	for _, row := range inspection.Coverage(c).Assets {
		if row.AssetID == assetID {
			return row.Percentage
		}
	}
	return 0
}

func (s *Service) SubmitMeasurements(ctx context.Context, id string, cmd SubmitMeasurementCommand) (MeasurementBatchResult, error) {
	if err := requireRole(cmd.Role, "inspector"); err != nil {
		return MeasurementBatchResult{}, err
	}
	inputs := cmd.Measurements
	if len(inputs) == 0 {
		inputs = []MeasurementInput{{CheckpointCode: cmd.CheckpointCode, Value: cmd.Value, Unit: cmd.Unit, InstrumentCode: cmd.InstrumentCode, InstrumentCalibratedOn: cmd.InstrumentCalibratedOn, InstrumentValidUntil: cmd.InstrumentValidUntil, MeasuredAt: cmd.MeasuredAt, Observation: cmd.Observation}}
	}
	if len(inputs) == 0 || len(inputs) > 100 {
		return MeasurementBatchResult{}, domain.NewRuleError("measurement_batch_size", "实测批次必须包含 1 至 100 行")
	}
	op := operationSignature("measurements.batch_submitted", struct {
		AssetID string
		Inputs  []MeasurementInput
	}{cmd.AssetID, inputs})
	before := 0
	created := []MeasurementItemResult{}
	c, err := s.mutate(ctx, id, cmd.Metadata, op, func(c *domain.InspectionCampaign) (any, error) {
		if c.Status != domain.StatusExecuting {
			return nil, domain.ErrInvalidState
		}
		before = assetCompleteness(*c, cmd.AssetID)
		seen := map[string]int{}
		issues := []string{}
		scratch := *c
		scratch.Measurements = append([]domain.MeasurementRevision(nil), c.Measurements...)
		scratch.Defects = append([]domain.DefectCase(nil), c.Defects...)
		created = []MeasurementItemResult{}
		for i, input := range inputs {
			code := strings.ToLower(strings.TrimSpace(input.CheckpointCode))
			if prior, ok := seen[code]; ok {
				issues = append(issues, fmt.Sprintf("第 %d 行：检查点与第 %d 行重复", i+1, prior))
				continue
			}
			seen[code] = i + 1
			m := domain.MeasurementRevision{ID: identifier("MEAS"), CampaignID: id, AssetID: cmd.AssetID, CheckpointCode: code, Value: input.Value, Unit: input.Unit, InstrumentCode: input.InstrumentCode, InstrumentCalibratedOn: input.InstrumentCalibratedOn, InstrumentValidUntil: input.InstrumentValidUntil, MeasuredAt: input.MeasuredAt, Observation: input.Observation, Revision: 1}
			m.Normalize()
			if prior, ok := scratch.LatestMeasurement(cmd.AssetID, m.CheckpointCode); ok {
				m.Revision, m.SupersedesID = prior.Revision+1, prior.ID
			}
			cp, _, validateErr := inspection.ValidateMeasurement(scratch, m, s.now())
			if validateErr == nil {
				validateErr = inspection.ValidateInstrument(scratch, cp, m)
			}
			if validateErr != nil {
				issues = append(issues, fmt.Sprintf("第 %d 行：%s", i+1, validateErr.Error()))
				continue
			}
			evaluation := inspection.Evaluate(cp, m)
			m.Passed, m.Evaluation = evaluation.Passed, evaluation.Reason
			scratch.Measurements = append(scratch.Measurements, m)
			result := MeasurementItemResult{CheckpointCode: m.CheckpointCode, RevisionID: m.ID, Revision: m.Revision, Passed: m.Passed, Evaluation: m.Evaluation}
			if defect := inspection.DefectFor(id, m, evaluation); defect != nil {
				scratch.Defects = append(scratch.Defects, *defect)
				result.DefectID = defect.ID
			}
			created = append(created, result)
		}
		if len(issues) > 0 {
			return nil, domain.NewRuleError("measurement_batch_invalid", "实测批次校验失败", issues...)
		}
		c.Measurements, c.Defects = scratch.Measurements, scratch.Defects
		return map[string]any{"assetId": cmd.AssetID, "items": created, "before": before, "after": assetCompleteness(*c, cmd.AssetID)}, nil
	})
	if err != nil {
		return MeasurementBatchResult{}, err
	}
	// 幂等重放时从保存后的聚合中恢复与本批检查点对应的稳定结果。
	if len(created) == 0 {
		for _, input := range inputs {
			if m, ok := c.LatestMeasurement(cmd.AssetID, strings.ToLower(strings.TrimSpace(input.CheckpointCode))); ok {
				item := MeasurementItemResult{CheckpointCode: m.CheckpointCode, RevisionID: m.ID, Revision: m.Revision, Passed: m.Passed, Evaluation: m.Evaluation}
				for _, d := range c.Defects {
					if d.SourceRevisionID == m.ID {
						item.DefectID = d.ID
					}
				}
				created = append(created, item)
			}
		}
		plan, _ := c.ActivePlan()
		required, completed, newlyCompleted := 0, 0, 0
		for _, cp := range plan.Checkpoints {
			asset, _ := c.Asset(cmd.AssetID)
			if !cp.AppliesTo(asset.AssetType) {
				continue
			}
			required++
			if latest, ok := c.LatestMeasurement(cmd.AssetID, cp.Code); ok {
				completed++
				for _, input := range inputs {
					if strings.EqualFold(strings.TrimSpace(input.CheckpointCode), cp.Code) && latest.Revision == 1 {
						newlyCompleted++
					}
				}
			}
		}
		if required > 0 {
			before = (completed - newlyCompleted) * 100 / required
		}
	}
	sort.Slice(created, func(i, j int) bool { return created[i].CheckpointCode < created[j].CheckpointCode })
	return MeasurementBatchResult{InspectionCampaign: c, Items: created, CompletenessBefore: before, CompletenessAfter: assetCompleteness(*c, cmd.AssetID), LatestVersion: c.Version, Campaign: c}, nil
}
