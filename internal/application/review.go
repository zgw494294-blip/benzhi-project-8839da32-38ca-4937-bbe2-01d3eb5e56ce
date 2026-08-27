package application

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"stage-rigging-safety-release/internal/domain"
	"stage-rigging-safety-release/internal/inspection"
)

func (s *Service) SubmitForReview(ctx context.Context, id string, cmd ReviewSubmitCommand) (*domain.InspectionCampaign, error) {
	if err := requireRole(cmd.Role, "inspector"); err != nil {
		return nil, err
	}
	op := operationSignature("review.submitted", cmd.Resolutions)
	return s.mutate(ctx, id, cmd.Metadata, op, func(c *domain.InspectionCampaign) (any, error) {
		if err := applyReturnResolutions(c, cmd.Resolutions, s.now()); err != nil {
			return nil, err
		}
		r := inspection.ReviewReadinessAt(*c, s.now())
		if !r.Ready {
			return nil, domain.NewRuleError("review_blocked", "证据尚不具备送审条件", r.Blockers...)
		}
		if err := c.Transition(domain.StatusReviewPending); err != nil {
			return nil, err
		}
		c.ReviewHistory = append(c.ReviewHistory, domain.ReviewRound{Round: len(c.ReviewHistory) + 1, SubmittedAt: s.now(), SubmittedBy: cmd.Actor})
		return r, nil
	})
}

func applyReturnResolutions(c *domain.InspectionCampaign, resolutions []ReturnResolution, now time.Time) error {
	if c.Review == nil || c.Review.Decision != "returned" {
		if len(resolutions) > 0 {
			return domain.NewRuleError("no_return_items", "当前任务没有待销项的退回内容")
		}
		return nil
	}
	byID := map[string]int{}
	for i := range c.Review.Items {
		byID[c.Review.Items[i].ID] = i
	}
	for _, resolution := range resolutions {
		idx, ok := byID[resolution.ItemID]
		if !ok {
			return domain.NewRuleError("return_item_not_found", "退回项不存在", resolution.ItemID)
		}
		if resolution.HandlingNote == "" || len(resolution.EvidenceRevisionIDs) == 0 {
			return domain.NewRuleError("return_resolution_incomplete", "退回项销项必须包含处理说明和新证据", resolution.ItemID)
		}
		item := &c.Review.Items[idx]
		for _, revisionID := range resolution.EvidenceRevisionIDs {
			var evidence *domain.MeasurementRevision
			for i := range c.Measurements {
				if c.Measurements[i].ID == revisionID {
					evidence = &c.Measurements[i]
					break
				}
			}
			if evidence != nil {
				if revisionID == item.EvidenceRevisionAtReturn || (item.AssetID != "" && evidence.AssetID != item.AssetID) || (item.CheckpointCode != "" && evidence.CheckpointCode != item.CheckpointCode) {
					return domain.NewRuleError("invalid_resolution_evidence", "销项证据不是退回后针对该对象产生的新修订", revisionID)
				}
			} else {
				validRemedy := false
				for _, defect := range c.Defects {
					if (item.AssetID != "" && defect.AssetID != item.AssetID) || (item.CheckpointCode != "" && defect.CheckpointCode != item.CheckpointCode) {
						continue
					}
					for _, remedy := range defect.RemedyRounds {
						if remedy.ID == revisionID && remedy.RecordedAt.After(item.ReturnedAt) {
							validRemedy = true
						}
					}
				}
				if !validRemedy {
					return domain.NewRuleError("invalid_resolution_evidence", "销项证据不是退回后针对该对象产生的新实测、整改或复验修订", revisionID)
				}
			}
			if evidence != nil && item.EvidenceRevisionAtReturn != "" {
				baseline := 0
				for _, m := range c.Measurements {
					if m.ID == item.EvidenceRevisionAtReturn {
						baseline = m.Revision
					}
				}
				if evidence.Revision <= baseline {
					return domain.NewRuleError("stale_resolution_evidence", "销项证据必须晚于退回时的证据位置", revisionID)
				}
			}
		}
		item.HandlingNote = resolution.HandlingNote
		item.ResolutionRevisionIDs = append([]string(nil), resolution.EvidenceRevisionIDs...)
		item.ResolvedAt = &now
	}
	if len(c.ReviewHistory) > 0 {
		for i := len(c.ReviewHistory) - 1; i >= 0; i-- {
			if c.ReviewHistory[i].Decision != nil && c.ReviewHistory[i].Decision.Decision == "returned" {
				copy := *c.Review
				c.ReviewHistory[i].Decision = &copy
				break
			}
		}
	}
	return nil
}

func (s *Service) DecideReview(ctx context.Context, id string, cmd ReviewDecisionCommand) (*domain.InspectionCampaign, error) {
	if err := requireRole(cmd.Role, "reviewer"); err != nil {
		return nil, err
	}
	op := operationSignature("review."+cmd.Decision, cmd)
	return s.mutate(ctx, id, cmd.Metadata, op, func(c *domain.InspectionCampaign) (any, error) {
		if c.Status != domain.StatusReviewPending {
			return nil, domain.ErrInvalidState
		}
		now := s.now()
		switch cmd.Decision {
		case "return":
			if len(cmd.Items) == 0 {
				return nil, domain.NewRuleError("return_items_required", "退回时必须至少创建一条结构化退回项")
			}
			items := make([]domain.ReviewReturnItem, 0, len(cmd.Items))
			plan, _ := c.ActivePlan()
			for i, input := range cmd.Items {
				if input.Category == "" || input.Reason == "" {
					return nil, domain.NewRuleError("return_item_invalid", "退回项类别和原因不能为空", fmt.Sprintf("第 %d 项", i+1))
				}
				item := domain.ReviewReturnItem{ID: identifier("RET"), Category: input.Category, Reason: input.Reason, AssetID: input.AssetID, CheckpointCode: input.CheckpointCode, ReturnedAt: now}
				if input.AssetID != "" {
					asset, ok := c.Asset(input.AssetID)
					if !ok {
						return nil, domain.NewRuleError("return_asset_invalid", "退回项引用的设备不属于当前任务", input.AssetID)
					}
					if input.CheckpointCode != "" {
						cp, ok := plan.Checkpoint(input.CheckpointCode)
						if !ok || !cp.AppliesTo(asset.AssetType) {
							return nil, domain.NewRuleError("return_checkpoint_invalid", "退回项引用的检查点不适用于该设备", input.CheckpointCode)
						}
						if latest, ok := c.LatestMeasurement(input.AssetID, input.CheckpointCode); ok {
							item.EvidenceRevisionAtReturn = latest.ID
						}
					}
				} else if input.CheckpointCode != "" {
					return nil, domain.NewRuleError("return_asset_required", "引用检查点时必须同时引用设备")
				}
				items = append(items, item)
			}
			c.Review = &domain.ReviewDecision{Decision: "returned", Reviewer: cmd.Actor, Reason: cmd.Reason, At: now, Round: len(c.ReviewHistory), Items: items}
			if err := c.Transition(domain.StatusExecuting); err != nil {
				return nil, err
			}
		case "approve":
			r := inspection.ReviewReadinessAt(*c, s.now())
			if !r.Ready {
				return nil, domain.NewRuleError("approval_blocked", "仍有阻断项", r.Blockers...)
			}
			c.Review = &domain.ReviewDecision{Decision: "approved", Reviewer: cmd.Actor, Reason: cmd.Reason, At: now, Round: len(c.ReviewHistory)}
			if err := c.Transition(domain.StatusApproved); err != nil {
				return nil, err
			}
		default:
			return nil, domain.NewRuleError("invalid_decision", "复核决定只能为 return 或 approve")
		}
		if len(c.ReviewHistory) > 0 {
			copy := *c.Review
			c.ReviewHistory[len(c.ReviewHistory)-1].Decision = &copy
		}
		return c.Review, nil
	})
}

func finalMeasurements(c *domain.InspectionCampaign) []domain.MeasurementRevision {
	latest := map[string]domain.MeasurementRevision{}
	for _, m := range c.Measurements {
		k := m.AssetID + "\x00" + m.CheckpointCode
		if old, ok := latest[k]; !ok || m.Revision > old.Revision {
			latest[k] = m
		}
	}
	result := make([]domain.MeasurementRevision, 0, len(latest))
	for _, m := range latest {
		result = append(result, m)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].AssetID != result[j].AssetID {
			return result[i].AssetID < result[j].AssetID
		}
		return result[i].CheckpointCode < result[j].CheckpointCode
	})
	return result
}

type FreezePreview struct {
	CanFreeze       bool                  `json:"canFreeze"`
	CandidateDigest string                `json:"candidateDigest"`
	MaterialCounts  map[string]int        `json:"materialCounts"`
	Snapshot        domain.FrozenSnapshot `json:"snapshot"`
	Issues          []string              `json:"issues"`
}

func (s *Service) freezePreview(c *domain.InspectionCampaign) (FreezePreview, error) {
	plan, ok := c.ActivePlan()
	if !ok {
		return FreezePreview{}, domain.NewRuleError("plan_required", "缺少锁定方案")
	}
	finals := finalMeasurements(c)
	snapshot := domain.FrozenSnapshot{CampaignID: c.ID, TheatreName: c.TheatreName, Year: c.InspectionYear, Assets: append([]domain.RiggingAsset(nil), c.Assets...), Plan: plan, Measurements: finals, Defects: append([]domain.DefectCase(nil), c.Defects...), ReviewHistory: append([]domain.ReviewRound(nil), c.ReviewHistory...), FrozenAt: s.now()}
	if c.Review != nil {
		snapshot.Decision = *c.Review
	}
	issues := inspection.ValidateFreezeCandidate(*c, finals)
	digest, err := domain.SnapshotDigest(snapshot)
	if err != nil {
		return FreezePreview{}, err
	}
	counts := map[string]int{"assets": len(snapshot.Assets), "checkpoints": len(snapshot.Plan.Checkpoints), "finalMeasurements": len(snapshot.Measurements), "defects": len(snapshot.Defects), "reviewRounds": len(snapshot.ReviewHistory)}
	return FreezePreview{CanFreeze: len(issues) == 0, CandidateDigest: digest, MaterialCounts: counts, Snapshot: snapshot, Issues: issues}, nil
}

func (s *Service) PreviewFreeze(ctx context.Context, id string) (FreezePreview, error) {
	c, err := s.repo.Get(ctx, id)
	if err != nil {
		return FreezePreview{}, err
	}
	return s.freezePreview(c)
}

func (s *Service) Freeze(ctx context.Context, id string, cmd FreezeCommand) (*domain.InspectionCampaign, error) {
	if err := requireRole(cmd.Role, "reviewer"); err != nil {
		return nil, err
	}
	op := operationSignature("evidence.frozen", cmd.CandidateDigest)
	return s.mutate(ctx, id, cmd.Metadata, op, func(c *domain.InspectionCampaign) (any, error) {
		if c.Status != domain.StatusApproved || c.Review == nil || c.Review.Decision != "approved" {
			return nil, domain.ErrInvalidState
		}
		preview, err := s.freezePreview(c)
		if err != nil {
			return nil, err
		}
		if !preview.CanFreeze {
			return nil, domain.NewRuleError("freeze_candidate_invalid", "冻结候选清单不完整", preview.Issues...)
		}
		if cmd.CandidateDigest == "" || cmd.CandidateDigest != preview.CandidateDigest {
			return nil, domain.NewRuleError("freeze_digest_drift", "冻结候选摘要已漂移，请重新预览", preview.CandidateDigest)
		}
		c.Freeze = &domain.FreezeRecord{Digest: preview.CandidateDigest, CandidateDigest: preview.CandidateDigest, MaterialCounts: preview.MaterialCounts, Snapshot: preview.Snapshot}
		if err = c.Transition(domain.StatusFrozen); err != nil {
			return nil, err
		}
		return map[string]any{"digest": preview.CandidateDigest, "materialCounts": preview.MaterialCounts}, nil
	})
}

func (s *Service) IssuePermit(ctx context.Context, id string, cmd IssuePermitCommand) (*domain.InspectionCampaign, error) {
	if err := requireRole(cmd.Role, "reviewer"); err != nil {
		return nil, err
	}
	op := operationSignature("permit.issued", cmd.ValidUntil)
	return s.mutate(ctx, id, cmd.Metadata, op, func(c *domain.InspectionCampaign) (any, error) {
		if c.Status != domain.StatusFrozen || c.Freeze == nil {
			return nil, domain.ErrInvalidState
		}
		now := s.now()
		if !cmd.ValidUntil.After(now) {
			return nil, domain.NewRuleError("invalid_validity", "许可有效期必须晚于签发时间")
		}
		codes := make([]string, 0, len(c.Assets))
		for _, a := range c.Assets {
			codes = append(codes, a.AssetCode)
		}
		sort.Strings(codes)
		suffix := strings.ToUpper(c.Freeze.Digest[:8])
		permit := &domain.OperatingPermit{PermitNumber: fmt.Sprintf("RIG-%d-%s", c.InspectionYear, suffix), CampaignID: c.ID, FrozenDigest: c.Freeze.Digest, ScopeAssetCodes: codes, IssuedBy: cmd.Actor, IssuedAt: now, ValidUntil: cmd.ValidUntil, VerificationStatus: "valid"}
		c.Permit = permit
		if err := c.Transition(domain.StatusLicensed); err != nil {
			return nil, err
		}
		return permit, nil
	})
}

func (s *Service) VerifyPermit(ctx context.Context, number string, requestedAsset ...string) (Verification, error) {
	number = strings.TrimSpace(number)
	now := s.now()
	if utf8.RuneCountInString(number) < 8 || utf8.RuneCountInString(number) > 80 {
		return Verification{}, domain.NewRuleError("invalid_permit_number", "许可编号长度无效")
	}
	for _, r := range number {
		if !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-' {
			return Verification{}, domain.NewRuleError("invalid_permit_number", "许可编号只能包含大写字母、数字和连字符")
		}
	}
	if len(requestedAsset) > 0 && strings.TrimSpace(requestedAsset[0]) != "" && !domain.ValidAssetCode(domain.NormalizeAssetCode(requestedAsset[0])) {
		return Verification{}, domain.NewRuleError("invalid_asset_code", "待核对设备编号格式无效")
	}
	c, err := s.repo.FindPermit(ctx, number)
	if err != nil {
		if err == domain.ErrNotFound {
			return Verification{Valid: false, CheckedAt: now, Status: "not_found", Message: "许可不存在", Failures: []string{"许可编号核对失败"}}, nil
		}
		return Verification{}, err
	}
	if c.Permit == nil || c.Freeze == nil {
		return Verification{CheckedAt: now, Message: "许可材料不完整", Failures: []string{"任务关联或冻结材料缺失"}}, nil
	}
	digest, err := domain.SnapshotDigest(c.Freeze.Snapshot)
	if err != nil {
		return Verification{}, err
	}
	scope := make([]string, 0, len(c.Freeze.Snapshot.Assets))
	for _, asset := range c.Freeze.Snapshot.Assets {
		scope = append(scope, domain.NormalizeAssetCode(asset.AssetCode))
	}
	sort.Strings(scope)
	permitScope := make([]string, len(c.Permit.ScopeAssetCodes))
	for i, code := range c.Permit.ScopeAssetCodes {
		permitScope[i] = domain.NormalizeAssetCode(code)
	}
	sort.Strings(permitScope)
	checks := map[string]bool{
		"permitNumber": c.Permit.PermitNumber == number,
		"campaignLink": c.Permit.CampaignID == c.ID && c.Freeze.Snapshot.CampaignID == c.ID,
		"frozenDigest": digest == c.Freeze.Digest && digest == c.Permit.FrozenDigest,
		"scope":        strings.Join(scope, "\x00") == strings.Join(permitScope, "\x00"),
	}
	remaining := int(math.Ceil(c.Permit.ValidUntil.Sub(now).Hours() / 24))
	status := "valid"
	if !c.Permit.ValidUntil.After(now) {
		status = "expired"
	} else if remaining <= 30 {
		status = "expiring_soon"
	}
	failures := []string{}
	labels := map[string]string{"permitNumber": "许可编号核对失败", "campaignLink": "任务关联核对失败", "frozenDigest": "冻结摘要核对失败", "scope": "许可范围清单核对失败"}
	for _, key := range []string{"permitNumber", "campaignLink", "frozenDigest", "scope"} {
		if !checks[key] {
			failures = append(failures, labels[key])
		}
	}
	if status == "expired" {
		failures = append(failures, "许可已过有效期")
	}
	var normalizedAsset string
	var inScope *bool
	if len(requestedAsset) > 0 && strings.TrimSpace(requestedAsset[0]) != "" {
		normalizedAsset = domain.NormalizeAssetCode(requestedAsset[0])
		matched := false
		for _, code := range permitScope {
			if code == normalizedAsset {
				matched = true
				break
			}
		}
		inScope = &matched
	}
	valid := len(failures) == 0
	message := "许可验真通过"
	if !valid {
		message = "许可验真失败：" + strings.Join(failures, "；")
	}
	return Verification{Valid: valid, Permit: c.Permit, TheatreName: c.TheatreName, InspectionYear: c.InspectionYear, RecalculatedDigest: digest, Message: message, Status: status, RemainingDays: remaining, AssetCode: normalizedAsset, AssetInScope: inScope, CheckedAt: now, Checks: checks, Failures: failures}, nil
}
