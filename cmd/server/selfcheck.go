package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"stage-rigging-safety-release/internal/application"
	"stage-rigging-safety-release/internal/domain"
	"stage-rigging-safety-release/internal/inspection"
)

type checkClient struct {
	base   string
	client *http.Client
	serial int
}

func (c *checkClient) key() string { c.serial++; return fmt.Sprintf("selfcheck-%02d", c.serial) }
func (c *checkClient) request(ctx context.Context, method, path string, body any, dst any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s 返回 %d: %s", method, path, resp.StatusCode, string(raw))
	}
	if dst != nil {
		return json.Unmarshal(raw, dst)
	}
	return nil
}
func inspector(version int, key string) application.Metadata {
	return application.Metadata{ExpectedVersion: version, IdempotencyKey: key, Actor: "自检检验员", Role: "inspector"}
}
func reviewer(version int, key string) application.Metadata {
	return application.Metadata{ExpectedVersion: version, IdempotencyKey: key, Actor: "自检复核负责人", Role: "reviewer"}
}

func selfcheck(ctx context.Context, baseURL string) error {
	c := &checkClient{base: baseURL, client: &http.Client{Timeout: 4 * time.Second}}
	var campaign domain.InspectionCampaign
	create := application.CreateCampaignCommand{ID: "CAM-SELFCHECK", TheatreName: "本智实验剧场", InspectionYear: time.Now().Year(), LeadInspector: "自检检验员", IdempotencyKey: c.key(), Actor: "自检检验员", Role: "inspector"}
	if err := c.request(ctx, "POST", "/api/v1/campaigns", create, &campaign); err != nil {
		return err
	}
	asset := domain.RiggingAsset{ID: "AST-SELFCHECK", AssetCode: "DG-01", AssetType: domain.AssetCounterweightBar, RatedLoadKg: 500, DriveType: "手动配重", SafetyDevices: []string{"上限位", "下限位"}, CommissionedOn: "2020-01-01"}
	var assetResult application.AssetBatchResult
	if err := c.request(ctx, "POST", "/api/v1/campaigns/"+campaign.ID+"/assets", application.AddAssetsCommand{Metadata: inspector(campaign.Version, c.key()), Assets: []domain.RiggingAsset{asset}}, &assetResult); err != nil {
		return err
	}
	campaign = *assetResult.Campaign
	var preflight inspection.PlanPreflight
	if err := c.request(ctx, "POST", "/api/v1/campaigns/"+campaign.ID+"/plans/preflight", application.PlanPreflightCommand{ExpectedVersion: campaign.Version, Actor: "自检检验员", Role: "inspector", Checkpoints: application.DefaultCheckpoints()}, &preflight); err != nil {
		return err
	}
	if err := c.request(ctx, "POST", "/api/v1/campaigns/"+campaign.ID+"/plans/confirm", application.ConfirmPlanCommand{Metadata: inspector(campaign.Version, c.key()), Checkpoints: application.DefaultCheckpoints(), PreviewDigest: preflight.Digest}, &campaign); err != nil {
		return err
	}
	points := []struct {
		code, unit string
		value      float64
	}{{"load_test", "kg", 500}, {"upper_limit", "bool", 1}, {"lower_limit", "bool", 1}, {"wire_wear", "percent", 6}}
	validUntil := time.Now().UTC().Add(180 * 24 * time.Hour)
	calibratedOn := time.Now().UTC().Add(-30 * 24 * time.Hour)
	for _, point := range points {
		cmd := application.SubmitMeasurementCommand{Metadata: inspector(campaign.Version, c.key()), AssetID: asset.ID, CheckpointCode: point.code, Value: point.value, Unit: point.unit, InstrumentCode: "SELF-METER-01", InstrumentCalibratedOn: calibratedOn, InstrumentValidUntil: validUntil, MeasuredAt: time.Now().UTC(), Observation: "selfcheck 原始实测"}
		var result application.MeasurementBatchResult
		if err := c.request(ctx, "POST", "/api/v1/campaigns/"+campaign.ID+"/measurements", cmd, &result); err != nil {
			return err
		}
		campaign = *result.Campaign
	}
	if len(campaign.Defects) != 1 {
		return fmt.Errorf("预期自动形成 1 项缺陷，实际 %d", len(campaign.Defects))
	}
	defect := campaign.Defects[0]
	remedy := application.RemedyCommand{Metadata: inspector(campaign.Version, c.key()), DefectID: defect.ID, Remedy: "更换磨损钢丝绳并重新张紧", Owner: "机械班组"}
	if err := c.request(ctx, "POST", fmt.Sprintf("/api/v1/campaigns/%s/defects/%s/remedy", campaign.ID, defect.ID), remedy, &campaign); err != nil {
		return err
	}
	retest := application.RetestCommand{Metadata: inspector(campaign.Version, c.key()), DefectID: defect.ID, Value: 2, Unit: "percent", InstrumentCode: "SELF-METER-01", InstrumentCalibratedOn: calibratedOn, InstrumentValidUntil: validUntil, MeasuredAt: time.Now().UTC(), Observation: "整改后复验合格"}
	if err := c.request(ctx, "POST", fmt.Sprintf("/api/v1/campaigns/%s/defects/%s/retest", campaign.ID, defect.ID), retest, &campaign); err != nil {
		return err
	}
	if err := c.request(ctx, "POST", "/api/v1/campaigns/"+campaign.ID+"/review/submit", application.ReviewSubmitCommand{Metadata: inspector(campaign.Version, c.key())}, &campaign); err != nil {
		return err
	}
	decision := application.ReviewDecisionCommand{Metadata: reviewer(campaign.Version, c.key()), Decision: "approve", Reason: "selfcheck 覆盖、证据和闭环检查通过"}
	if err := c.request(ctx, "POST", "/api/v1/campaigns/"+campaign.ID+"/review/decision", decision, &campaign); err != nil {
		return err
	}
	var freezePreview application.FreezePreview
	if err := c.request(ctx, "GET", "/api/v1/campaigns/"+campaign.ID+"/freeze/preview", nil, &freezePreview); err != nil {
		return err
	}
	if err := c.request(ctx, "POST", "/api/v1/campaigns/"+campaign.ID+"/freeze", application.FreezeCommand{Metadata: reviewer(campaign.Version, c.key()), CandidateDigest: freezePreview.CandidateDigest}, &campaign); err != nil {
		return err
	}
	issue := application.IssuePermitCommand{Metadata: reviewer(campaign.Version, c.key()), ValidUntil: time.Now().UTC().Add(365 * 24 * time.Hour)}
	if err := c.request(ctx, "POST", "/api/v1/campaigns/"+campaign.ID+"/permit", issue, &campaign); err != nil {
		return err
	}
	if campaign.Permit == nil {
		return fmt.Errorf("许可未生成")
	}
	var verification application.Verification
	if err := c.request(ctx, "GET", "/api/v1/permits/"+campaign.Permit.PermitNumber+"/verify", nil, &verification); err != nil {
		return err
	}
	if !verification.Valid {
		return fmt.Errorf("许可验真未通过: %s", verification.Message)
	}
	var timeline struct {
		Events []domain.AuditEvent `json:"events"`
	}
	if err := c.request(ctx, "GET", "/api/v1/campaigns/"+campaign.ID+"/timeline", nil, &timeline); err != nil {
		return err
	}
	if len(timeline.Events) < 12 {
		return fmt.Errorf("审计事件不足: %d", len(timeline.Events))
	}
	return nil
}
