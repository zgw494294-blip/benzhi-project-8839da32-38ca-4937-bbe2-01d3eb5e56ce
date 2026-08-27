package application

import (
	"context"
	"fmt"

	"stage-rigging-safety-release/internal/domain"
	"stage-rigging-safety-release/internal/inspection"
)

func (s *Service) AddAsset(ctx context.Context, id string, cmd AddAssetCommand) (*domain.InspectionCampaign, error) {
	if err := requireRole(cmd.Role, "inspector"); err != nil {
		return nil, err
	}
	op := operationSignature("asset.added", cmd.Asset)
	return s.mutate(ctx, id, cmd.Metadata, op, func(c *domain.InspectionCampaign) (any, error) {
		if cmd.Asset.ID == "" {
			cmd.Asset.ID = identifier("AST")
		}
		cmd.Asset.CampaignID = id
		if err := c.AddAsset(cmd.Asset); err != nil {
			return nil, err
		}
		return cmd.Asset, nil
	})
}

func (s *Service) ConfirmPlan(ctx context.Context, id string, cmd ConfirmPlanCommand) (*domain.InspectionCampaign, error) {
	if err := requireRole(cmd.Role, "inspector"); err != nil {
		return nil, err
	}
	op := operationSignature("plan.confirmed", struct {
		Checkpoints []domain.Checkpoint
		Digest      string
	}{cmd.Checkpoints, cmd.PreviewDigest})
	return s.mutate(ctx, id, cmd.Metadata, op, func(c *domain.InspectionCampaign) (any, error) {
		if c.Status != domain.StatusDraft {
			return nil, domain.ErrInvalidState
		}
		if len(c.Assets) == 0 {
			return nil, domain.NewRuleError("asset_required", "至少登记一台设备")
		}
		preflight := inspection.PreflightPlan(*c, cmd.Checkpoints)
		if !preflight.Valid {
			details := make([]string, 0, len(preflight.Issues))
			for _, issue := range preflight.Issues {
				details = append(details, issue.Message)
			}
			return nil, domain.NewRuleError("plan_preflight_failed", "方案适用性预检未通过", details...)
		}
		if cmd.PreviewDigest == "" || cmd.PreviewDigest != preflight.Digest {
			return nil, domain.NewRuleError("plan_digest_mismatch", "方案或设备范围已变化，请重新预检", preflight.Digest)
		}
		plan := domain.InspectionPlan{ID: identifier("PLAN"), CampaignID: id, Revision: len(c.Plans) + 1, Status: "confirmed", Checkpoints: domain.NormalizeCheckpoints(cmd.Checkpoints), ConfirmedBy: cmd.Actor, ContentDigest: preflight.Digest}
		now := s.now()
		plan.ConfirmedAt = &now
		c.Plans = append(c.Plans, plan)
		if err := c.Transition(domain.StatusExecuting); err != nil {
			return nil, err
		}
		return plan, nil
	})
}

func (s *Service) SubmitMeasurement(ctx context.Context, id string, cmd SubmitMeasurementCommand) (*domain.InspectionCampaign, error) {
	if err := requireRole(cmd.Role, "inspector"); err != nil {
		return nil, err
	}
	op := operationSignature("measurement.submitted", cmd)
	return s.mutate(ctx, id, cmd.Metadata, op, func(c *domain.InspectionCampaign) (any, error) {
		if c.Status != domain.StatusExecuting {
			return nil, domain.ErrInvalidState
		}
		m := domain.MeasurementRevision{ID: identifier("MEAS"), CampaignID: id, AssetID: cmd.AssetID, CheckpointCode: cmd.CheckpointCode, Value: cmd.Value, Unit: cmd.Unit, InstrumentCode: cmd.InstrumentCode, InstrumentCalibratedOn: cmd.InstrumentCalibratedOn, InstrumentValidUntil: cmd.InstrumentValidUntil, MeasuredAt: cmd.MeasuredAt, Observation: cmd.Observation, Revision: 1}
		m.Normalize()
		if prior, ok := c.LatestMeasurement(cmd.AssetID, cmd.CheckpointCode); ok {
			m.Revision = prior.Revision + 1
			m.SupersedesID = prior.ID
		}
		cp, _, err := inspection.ValidateMeasurement(*c, m, s.now())
		if err != nil {
			return nil, err
		}
		if err := inspection.ValidateInstrument(*c, cp, m); err != nil {
			return nil, err
		}
		evaluation := inspection.Evaluate(cp, m)
		m.Passed, m.Evaluation = evaluation.Passed, evaluation.Reason
		c.Measurements = append(c.Measurements, m)
		if defect := inspection.DefectFor(id, m, evaluation); defect != nil {
			c.Defects = append(c.Defects, *defect)
		}
		return m, nil
	})
}

func (s *Service) RecordRemedy(ctx context.Context, id string, cmd RemedyCommand) (*domain.InspectionCampaign, error) {
	if err := requireRole(cmd.Role, "inspector"); err != nil {
		return nil, err
	}
	op := operationSignature("defect.remedy_recorded", cmd)
	return s.mutate(ctx, id, cmd.Metadata, op, func(c *domain.InspectionCampaign) (any, error) {
		if c.Status != domain.StatusExecuting {
			return nil, domain.ErrInvalidState
		}
		d, ok := c.Defect(cmd.DefectID)
		if !ok {
			return nil, domain.ErrNotFound
		}
		if err := d.RecordRemedyRound(identifier("REM"), cmd.Remedy, cmd.Owner, s.now()); err != nil {
			return nil, err
		}
		return d, nil
	})
}

func (s *Service) SubmitRetest(ctx context.Context, id string, cmd RetestCommand) (*domain.InspectionCampaign, error) {
	if err := requireRole(cmd.Role, "inspector"); err != nil {
		return nil, err
	}
	op := operationSignature("defect.retested", cmd)
	return s.mutate(ctx, id, cmd.Metadata, op, func(c *domain.InspectionCampaign) (any, error) {
		if c.Status != domain.StatusExecuting {
			return nil, domain.ErrInvalidState
		}
		d, ok := c.Defect(cmd.DefectID)
		if !ok {
			return nil, domain.ErrNotFound
		}
		var original domain.MeasurementRevision
		found := false
		for _, m := range c.Measurements {
			if m.ID == d.SourceRevisionID {
				original = m
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("%w: 原始实测不存在", domain.ErrValidation)
		}
		latest, ok := c.LatestMeasurement(d.AssetID, d.CheckpointCode)
		if !ok {
			return nil, domain.NewRuleError("evidence_chain_missing", "缺少检查点最新证据")
		}
		if len(d.RemedyRounds) == 0 || d.Status != domain.DefectRemediated {
			return nil, domain.NewRuleError("remedy_first", "每次复验前必须登记新一轮整改措施")
		}
		lastRemedy := d.RemedyRounds[len(d.RemedyRounds)-1]
		if !cmd.MeasuredAt.After(lastRemedy.RecordedAt) {
			return nil, domain.NewRuleError("retest_before_remedy", "复验时间必须晚于最近整改记录")
		}
		m := domain.MeasurementRevision{ID: identifier("MEAS"), CampaignID: id, AssetID: d.AssetID, CheckpointCode: d.CheckpointCode, Revision: latest.Revision + 1, Value: cmd.Value, Unit: cmd.Unit, InstrumentCode: cmd.InstrumentCode, InstrumentCalibratedOn: cmd.InstrumentCalibratedOn, InstrumentValidUntil: cmd.InstrumentValidUntil, MeasuredAt: cmd.MeasuredAt, Observation: cmd.Observation, SupersedesID: latest.ID}
		m.Normalize()
		cp, _, err := inspection.ValidateMeasurement(*c, m, s.now())
		if err != nil {
			return nil, err
		}
		if err := inspection.ValidateInstrument(*c, cp, m); err != nil {
			return nil, err
		}
		evaluated := inspection.Evaluate(cp, m)
		m.Passed, m.Evaluation = evaluated.Passed, evaluated.Reason
		if err := inspection.ValidateRetest(*d, original, latest, m, evaluated, s.now()); err != nil {
			return nil, err
		}
		c.Measurements = append(c.Measurements, m)
		if err = d.RecordRetest(m, evaluated.Passed, s.now()); err != nil {
			return nil, err
		}
		return m, nil
	})
}

func DefaultCheckpoints() []domain.Checkpoint {
	return []domain.Checkpoint{
		{Code: "load_test", Name: "额定载荷测试", AssetTypes: []domain.AssetType{domain.AssetCounterweightBar, domain.AssetPoweredBar, domain.AssetWinch}, Unit: "kg", Comparison: domain.CompareMinimum, Threshold: 500, Critical: true, InstrumentMaxAge: 365},
		{Code: "brake_distance", Name: "制动距离", AssetTypes: []domain.AssetType{domain.AssetPoweredBar, domain.AssetWinch}, Unit: "mm", Comparison: domain.CompareMaximum, Threshold: 80, Critical: true, InstrumentMaxAge: 365},
		{Code: "upper_limit", Name: "上限位动作", AssetTypes: []domain.AssetType{domain.AssetCounterweightBar, domain.AssetPoweredBar, domain.AssetWinch}, Unit: "bool", Comparison: domain.CompareBoolean, Threshold: 1, Critical: true, InstrumentMaxAge: 365},
		{Code: "lower_limit", Name: "下限位动作", AssetTypes: []domain.AssetType{domain.AssetCounterweightBar, domain.AssetPoweredBar, domain.AssetWinch}, Unit: "bool", Comparison: domain.CompareBoolean, Threshold: 1, Critical: true, InstrumentMaxAge: 365},
		{Code: "wire_wear", Name: "钢丝绳磨损率", AssetTypes: []domain.AssetType{domain.AssetCounterweightBar, domain.AssetPoweredBar, domain.AssetWinch}, Unit: "percent", Comparison: domain.CompareMaximum, Threshold: 5, Critical: false, InstrumentMaxAge: 365},
	}
}
