package application

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"stage-rigging-safety-release/internal/domain"
	"stage-rigging-safety-release/internal/storage"
)

func testService(t *testing.T) (*Service, context.Context, time.Time) {
	t.Helper()
	repo, err := storage.Open(filepath.Join(t.TempDir(), "application.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	at := time.Date(2026, 8, 27, 2, 0, 0, 0, time.UTC)
	svc := New(repo)
	svc.now = func() time.Time { return at }
	return svc, context.Background(), at
}

func testMeta(version int, key string, role ...string) Metadata {
	r := "inspector"
	actor := "测试检验员"
	if len(role) > 0 {
		r = role[0]
		actor = "测试复核负责人"
	}
	return Metadata{ExpectedVersion: version, IdempotencyKey: key, Actor: actor, Role: r}
}

func createTestCampaign(t *testing.T, svc *Service, ctx context.Context, id string) *domain.InspectionCampaign {
	t.Helper()
	c, err := svc.CreateCampaign(ctx, CreateCampaignCommand{ID: id, TheatreName: "扩展验收剧场", InspectionYear: 2026, LeadInspector: "测试检验员", IdempotencyKey: id + "-create", Actor: "测试检验员", Role: "inspector"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func asset(id, code string, kind domain.AssetType) domain.RiggingAsset {
	devices := []string{"上限位", "下限位"}
	if kind == domain.AssetPoweredBar || kind == domain.AssetWinch {
		devices = append(devices, "制动器")
	}
	return domain.RiggingAsset{ID: id, AssetCode: code, AssetType: kind, RatedLoadKg: 500, DriveType: "电动", SafetyDevices: devices, CommissionedOn: "2020-01-01"}
}

func TestBatchScopePlanAndMeasurementAreAtomicAndIdempotent(t *testing.T) {
	svc, ctx, at := testService(t)
	c := createTestCampaign(t, svc, ctx, "CAM-BATCH")
	assets := []domain.RiggingAsset{asset("A1", " dg-01 ", domain.AssetPoweredBar), asset("A2", "pg-01", domain.AssetCounterweightBar), asset("A3", "jy-01", domain.AssetWinch)}
	added, err := svc.AddAssets(ctx, c.ID, AddAssetsCommand{Metadata: testMeta(c.Version, "assets-1"), Assets: assets})
	if err != nil {
		t.Fatal(err)
	}
	if added.LatestVersion != 2 || added.AddedCount != 3 || added.CountsByType[domain.AssetWinch] != 1 || added.Campaign.Assets[0].AssetCode != "DG-01" {
		t.Fatalf("批量设备汇总不正确: %+v", added)
	}
	replayed, err := svc.AddAssets(ctx, c.ID, AddAssetsCommand{Metadata: testMeta(1, "assets-1"), Assets: assets})
	if err != nil || replayed.LatestVersion != 2 || len(replayed.Campaign.Assets) != 3 {
		t.Fatalf("设备批次幂等重放失败: %+v %v", replayed, err)
	}
	changedReplay := append([]domain.RiggingAsset(nil), assets...)
	changedReplay[0].RatedLoadKg = 800
	if _, err = svc.AddAssets(ctx, c.ID, AddAssetsCommand{Metadata: testMeta(1, "assets-1"), Assets: changedReplay}); err == nil {
		t.Fatal("同一幂等键不得接受不同批次内容")
	}
	invalid := []domain.RiggingAsset{asset("A4", "DG-01", domain.AssetWinch), asset("A5", "NEW-01", domain.AssetPoweredBar)}
	invalid[1].SafetyDevices = []string{"上限位"}
	if _, err = svc.AddAssets(ctx, c.ID, AddAssetsCommand{Metadata: testMeta(2, "assets-invalid"), Assets: invalid}); err == nil {
		t.Fatal("含重复编号和缺失安全装置的批次应整体失败")
	}
	view, _ := svc.GetCampaign(ctx, c.ID)
	if view.Campaign.Version != 2 || len(view.Campaign.Assets) != 3 {
		t.Fatalf("失败批次不应改变范围: %+v", view.Campaign)
	}
	preflight, err := svc.PreflightPlan(ctx, c.ID, PlanPreflightCommand{ExpectedVersion: 2, Actor: "测试检验员", Role: "inspector", Checkpoints: DefaultCheckpoints()})
	if err != nil || !preflight.Valid || len(preflight.Matrix) != 3 {
		t.Fatalf("方案预检失败: %+v %v", preflight, err)
	}
	missingBrake := []domain.Checkpoint{}
	for _, cp := range DefaultCheckpoints() {
		if cp.Code != "brake_distance" {
			missingBrake = append(missingBrake, cp)
		}
	}
	invalidPlan, err := svc.PreflightPlan(ctx, c.ID, PlanPreflightCommand{ExpectedVersion: 2, Actor: "测试检验员", Role: "inspector", Checkpoints: missingBrake})
	if err != nil || invalidPlan.Valid {
		t.Fatalf("遗漏卷扬机制动距离检查应在预检中形成缺口: %+v %v", invalidPlan, err)
	}
	c, err = svc.ConfirmPlan(ctx, c.ID, ConfirmPlanCommand{Metadata: testMeta(2, "plan-1"), Checkpoints: DefaultCheckpoints(), PreviewDigest: preflight.Digest})
	if err != nil || c.Version != 3 || c.Plans[0].ContentDigest != preflight.Digest {
		t.Fatalf("方案锁定失败: %+v %v", c, err)
	}
	calibrated := at.Add(-30 * 24 * time.Hour)
	validUntil := at.Add(180 * 24 * time.Hour)
	inputs := []MeasurementInput{}
	for _, cp := range DefaultCheckpoints() {
		if !cp.AppliesTo(domain.AssetPoweredBar) {
			continue
		}
		value := cp.Threshold
		if cp.Code == "brake_distance" {
			value = 90
		}
		inputs = append(inputs, MeasurementInput{CheckpointCode: cp.Code, Value: value, Unit: cp.Unit, InstrumentCode: " meter 01 ", InstrumentCalibratedOn: calibrated, InstrumentValidUntil: validUntil, MeasuredAt: at, Observation: "批量实测"})
	}
	measured, err := svc.SubmitMeasurements(ctx, c.ID, SubmitMeasurementCommand{Metadata: testMeta(c.Version, "measurements-1"), AssetID: "A1", Measurements: inputs})
	if err != nil {
		t.Fatal(err)
	}
	if measured.LatestVersion != 4 || len(measured.Items) != 5 || measured.CompletenessBefore != 0 || measured.CompletenessAfter != 100 || len(measured.Campaign.Defects) != 1 {
		t.Fatalf("批量实测响应不正确: %+v", measured)
	}
	ids := make([]string, len(measured.Items))
	for i := range measured.Items {
		ids[i] = measured.Items[i].RevisionID
	}
	replayedMeasurement, err := svc.SubmitMeasurements(ctx, c.ID, SubmitMeasurementCommand{Metadata: testMeta(3, "measurements-1"), AssetID: "A1", Measurements: inputs})
	if err != nil || replayedMeasurement.LatestVersion != 4 || len(replayedMeasurement.Items) != 5 {
		t.Fatalf("实测批次幂等重放失败: %+v %v", replayedMeasurement, err)
	}
	for i := range ids {
		if replayedMeasurement.Items[i].RevisionID != ids[i] {
			t.Fatalf("幂等重放生成了不同修订: %v / %v", ids, replayedMeasurement.Items)
		}
	}
	bad := []MeasurementInput{{CheckpointCode: "upper_limit", Value: 1, Unit: "mm", InstrumentCode: "METER-02", InstrumentCalibratedOn: calibrated, InstrumentValidUntil: validUntil, MeasuredAt: at}}
	if _, err = svc.SubmitMeasurements(ctx, c.ID, SubmitMeasurementCommand{Metadata: testMeta(4, "measurements-invalid"), AssetID: "A2", Measurements: bad}); err == nil {
		t.Fatal("错误单位应使整批实测失败")
	}
	after, _ := svc.GetCampaign(ctx, c.ID)
	if after.Campaign.Version != 4 || len(after.Campaign.Measurements) != 5 || len(after.Campaign.Defects) != 1 {
		t.Fatalf("失败实测批次改变了聚合: %+v", after.Campaign)
	}
	conflictingCalibration := []MeasurementInput{{CheckpointCode: "upper_limit", Value: 1, Unit: "bool", InstrumentCode: "METER01", InstrumentCalibratedOn: calibrated, InstrumentValidUntil: validUntil.Add(-24 * time.Hour), MeasuredAt: at}}
	if _, err = svc.SubmitMeasurements(ctx, c.ID, SubmitMeasurementCommand{Metadata: testMeta(4, "instrument-conflict"), AssetID: "A2", Measurements: conflictingCalibration}); err == nil {
		t.Fatal("同一仪器相同校准日期的不同有效期应被拒绝")
	}
	overlongCalibration := []MeasurementInput{{CheckpointCode: "upper_limit", Value: 1, Unit: "bool", InstrumentCode: "METER03", InstrumentCalibratedOn: at.Add(-10 * 24 * time.Hour), InstrumentValidUntil: at.Add(400 * 24 * time.Hour), MeasuredAt: at}}
	if _, err = svc.SubmitMeasurements(ctx, c.ID, SubmitMeasurementCommand{Metadata: testMeta(4, "instrument-overlong"), AssetID: "A2", Measurements: overlongCalibration}); err == nil {
		t.Fatal("超过检查点上限的校准周期应被拒绝")
	}
}

func TestFailedRetestKeepsContinuousRounds(t *testing.T) {
	svc, ctx, at := testService(t)
	c := createTestCampaign(t, svc, ctx, "CAM-RETEST")
	added, err := svc.AddAssets(ctx, c.ID, AddAssetsCommand{Metadata: testMeta(c.Version, "assets"), Assets: []domain.RiggingAsset{asset("A1", "JY-01", domain.AssetWinch)}})
	if err != nil {
		t.Fatal(err)
	}
	preflight, _ := svc.PreflightPlan(ctx, c.ID, PlanPreflightCommand{ExpectedVersion: added.LatestVersion, Actor: "测试检验员", Role: "inspector", Checkpoints: DefaultCheckpoints()})
	c, err = svc.ConfirmPlan(ctx, c.ID, ConfirmPlanCommand{Metadata: testMeta(added.LatestVersion, "plan"), Checkpoints: DefaultCheckpoints(), PreviewDigest: preflight.Digest})
	if err != nil {
		t.Fatal(err)
	}
	calibrated, valid := at.Add(-10*24*time.Hour), at.Add(100*24*time.Hour)
	result, err := svc.SubmitMeasurements(ctx, c.ID, SubmitMeasurementCommand{Metadata: testMeta(c.Version, "fail"), AssetID: "A1", Measurements: []MeasurementInput{{CheckpointCode: "brake_distance", Value: 100, Unit: "mm", InstrumentCode: "M-1", InstrumentCalibratedOn: calibrated, InstrumentValidUntil: valid, MeasuredAt: at}}})
	if err != nil {
		t.Fatal(err)
	}
	defectID := result.Campaign.Defects[0].ID
	c, err = svc.RecordRemedy(ctx, c.ID, RemedyCommand{Metadata: testMeta(result.LatestVersion, "remedy-1"), DefectID: defectID, Remedy: "调整制动器", Owner: "机械班组"})
	if err != nil {
		t.Fatal(err)
	}
	c, err = svc.SubmitRetest(ctx, c.ID, RetestCommand{Metadata: testMeta(c.Version, "retest-1"), DefectID: defectID, Value: 90, Unit: "mm", InstrumentCode: "M-1", InstrumentCalibratedOn: calibrated, InstrumentValidUntil: valid, MeasuredAt: at.Add(time.Minute), Observation: "首次仍超限"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Defects[0].Status != domain.DefectRetestFailed || len(c.Defects[0].RetestRounds) != 1 || c.Defects[0].ClosedAt != nil {
		t.Fatalf("失败复验未正确留痕: %+v", c.Defects[0])
	}
	if _, err = svc.SubmitRetest(ctx, c.ID, RetestCommand{Metadata: testMeta(c.Version, "retest-without-remedy"), DefectID: defectID, Value: 70, Unit: "mm", InstrumentCode: "M-1", InstrumentCalibratedOn: calibrated, InstrumentValidUntil: valid, MeasuredAt: at.Add(2 * time.Minute)}); err == nil {
		t.Fatal("失败复验后未整改不应允许连续复验")
	}
	c, err = svc.RecordRemedy(ctx, c.ID, RemedyCommand{Metadata: testMeta(c.Version, "remedy-2"), DefectID: defectID, Remedy: "更换制动组件", Owner: "机械班组"})
	if err != nil {
		t.Fatal(err)
	}
	c, err = svc.SubmitRetest(ctx, c.ID, RetestCommand{Metadata: testMeta(c.Version, "retest-2"), DefectID: defectID, Value: 70, Unit: "mm", InstrumentCode: "M-1", InstrumentCalibratedOn: calibrated, InstrumentValidUntil: valid, MeasuredAt: at.Add(3 * time.Minute), Observation: "二次复验合格"})
	if err != nil {
		t.Fatal(err)
	}
	defect := c.Defects[0]
	if defect.Status != domain.DefectClosed || len(defect.RemedyRounds) != 2 || len(defect.RetestRounds) != 2 || defect.RetestRounds[0].Passed || !defect.RetestRounds[1].Passed {
		t.Fatalf("多轮整改复验链不正确: %+v", defect)
	}
	latest, _ := c.LatestMeasurement("A1", "brake_distance")
	if latest.Revision != 3 || latest.SupersedesID != c.Measurements[1].ID {
		t.Fatalf("复验 supersedes 链不连续: %+v", c.Measurements)
	}
}

func TestReviewFreezeAndPermitVerification(t *testing.T) {
	svc, ctx, at := testService(t)
	c := createTestCampaign(t, svc, ctx, "CAM-FREEZE")
	added, err := svc.AddAssets(ctx, c.ID, AddAssetsCommand{Metadata: testMeta(c.Version, "assets"), Assets: []domain.RiggingAsset{asset("A1", "DG-01", domain.AssetCounterweightBar)}})
	if err != nil {
		t.Fatal(err)
	}
	preflight, _ := svc.PreflightPlan(ctx, c.ID, PlanPreflightCommand{ExpectedVersion: added.LatestVersion, Actor: "测试检验员", Role: "inspector", Checkpoints: DefaultCheckpoints()})
	c, err = svc.ConfirmPlan(ctx, c.ID, ConfirmPlanCommand{Metadata: testMeta(added.LatestVersion, "plan"), Checkpoints: DefaultCheckpoints(), PreviewDigest: preflight.Digest})
	if err != nil {
		t.Fatal(err)
	}
	inputs := []MeasurementInput{}
	for _, cp := range DefaultCheckpoints() {
		if cp.AppliesTo(domain.AssetCounterweightBar) {
			inputs = append(inputs, MeasurementInput{CheckpointCode: cp.Code, Value: cp.Threshold, Unit: cp.Unit, InstrumentCode: "M-OK", InstrumentCalibratedOn: at.Add(-20 * 24 * time.Hour), InstrumentValidUntil: at.Add(200 * 24 * time.Hour), MeasuredAt: at})
		}
	}
	measured, err := svc.SubmitMeasurements(ctx, c.ID, SubmitMeasurementCommand{Metadata: testMeta(c.Version, "measure"), AssetID: "A1", Measurements: inputs})
	if err != nil {
		t.Fatal(err)
	}
	c, err = svc.SubmitForReview(ctx, c.ID, ReviewSubmitCommand{Metadata: testMeta(measured.LatestVersion, "review-1")})
	if err != nil {
		t.Fatal(err)
	}
	c, err = svc.DecideReview(ctx, c.ID, ReviewDecisionCommand{Metadata: testMeta(c.Version, "return-1", "reviewer"), Decision: "return", Reason: "补充上限位观察", Items: []ReviewReturnItemInput{{Category: "observation", Reason: "补充动作过程说明", AssetID: "A1", CheckpointCode: "upper_limit"}}})
	if err != nil {
		t.Fatal(err)
	}
	item := c.Review.Items[0]
	updated, err := svc.SubmitMeasurements(ctx, c.ID, SubmitMeasurementCommand{Metadata: testMeta(c.Version, "supplement"), AssetID: "A1", Measurements: []MeasurementInput{{CheckpointCode: "upper_limit", Value: 1, Unit: "bool", InstrumentCode: "M-OK", InstrumentCalibratedOn: at.Add(-20 * 24 * time.Hour), InstrumentValidUntil: at.Add(200 * 24 * time.Hour), MeasuredAt: at.Add(time.Minute), Observation: "补充上限位完整动作过程"}}})
	if err != nil {
		t.Fatal(err)
	}
	newRevision := updated.Items[0].RevisionID
	c, err = svc.SubmitForReview(ctx, c.ID, ReviewSubmitCommand{Metadata: testMeta(updated.LatestVersion, "review-2"), Resolutions: []ReturnResolution{{ItemID: item.ID, HandlingNote: "已补充动作观察", EvidenceRevisionIDs: []string{newRevision}}}})
	if err != nil {
		t.Fatal(err)
	}
	c, err = svc.DecideReview(ctx, c.ID, ReviewDecisionCommand{Metadata: testMeta(c.Version, "approve", "reviewer"), Decision: "approve", Reason: "补证已核对"})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := svc.PreviewFreeze(ctx, c.ID)
	if err != nil || !preview.CanFreeze || preview.MaterialCounts["reviewRounds"] != 2 {
		t.Fatalf("冻结预览不正确: %+v %v", preview, err)
	}
	if _, err = svc.Freeze(ctx, c.ID, FreezeCommand{Metadata: testMeta(c.Version, "freeze-drift", "reviewer"), CandidateDigest: "changed"}); err == nil {
		t.Fatal("候选摘要不一致应拒绝冻结")
	}
	c, err = svc.Freeze(ctx, c.ID, FreezeCommand{Metadata: testMeta(c.Version, "freeze-ok", "reviewer"), CandidateDigest: preview.CandidateDigest})
	if err != nil || c.Freeze.Digest != preview.CandidateDigest || c.Freeze.MaterialCounts["finalMeasurements"] != preview.MaterialCounts["finalMeasurements"] {
		t.Fatalf("冻结结果与预览不一致: %+v %v", c.Freeze, err)
	}
	c, err = svc.IssuePermit(ctx, c.ID, IssuePermitCommand{Metadata: testMeta(c.Version, "permit", "reviewer"), ValidUntil: at.Add(365 * 24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	eventsBefore, _ := svc.Timeline(ctx, c.ID)
	verified, err := svc.VerifyPermit(ctx, c.Permit.PermitNumber, " dg-01 ")
	if err != nil || !verified.Valid || verified.AssetInScope == nil || !*verified.AssetInScope || verified.Status != "valid" || verified.RemainingDays <= 0 {
		t.Fatalf("范围内许可验真失败: %+v %v", verified, err)
	}
	notInScope, err := svc.VerifyPermit(ctx, c.Permit.PermitNumber, "DG-010")
	if err != nil || !notInScope.Valid || notInScope.AssetInScope == nil || *notInScope.AssetInScope {
		t.Fatalf("相似设备编号不应被部分匹配: %+v %v", notInScope, err)
	}
	eventsAfter, _ := svc.Timeline(ctx, c.ID)
	if len(eventsAfter) != len(eventsBefore) {
		t.Fatal("许可验真不应改变审计时间线")
	}
	svc.now = func() time.Time { return at.Add(366 * 24 * time.Hour) }
	expired, err := svc.VerifyPermit(ctx, c.Permit.PermitNumber)
	if err != nil || expired.Valid || expired.Status != "expired" || expired.RemainingDays > 0 || expired.Permit == nil {
		t.Fatalf("过期许可仍应可查询但整体无效: %+v %v", expired, err)
	}
}
