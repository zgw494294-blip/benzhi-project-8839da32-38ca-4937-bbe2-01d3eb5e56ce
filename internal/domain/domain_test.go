package domain

import (
	"errors"
	"testing"
	"time"
)

func TestCampaignStateMachineAndScopeLock(t *testing.T) {
	now := time.Date(2026, 8, 27, 1, 2, 3, 0, time.UTC)
	c, err := NewCampaign("CAM-1", "测试剧场", 2026, "检验员", now)
	if err != nil {
		t.Fatal(err)
	}
	asset := RiggingAsset{ID: "A1", AssetCode: "DG-1", AssetType: AssetPoweredBar, RatedLoadKg: 500, DriveType: "电动"}
	if err := c.AddAsset(asset); err != nil {
		t.Fatal(err)
	}
	if err := c.Transition(StatusExecuting); err != nil {
		t.Fatal(err)
	}
	if err := c.AddAsset(RiggingAsset{ID: "A2", AssetCode: "DG-2", AssetType: AssetWinch, RatedLoadKg: 500}); err == nil {
		t.Fatal("执行后应锁定设备范围")
	}
	if err := c.Transition(StatusApproved); err == nil {
		t.Fatal("不应从执行状态越级批准")
	}
	if err := c.Transition(StatusReviewPending); err != nil {
		t.Fatal(err)
	}
	if err := c.Transition(StatusApproved); err != nil {
		t.Fatal(err)
	}
	if err := c.Transition(StatusFrozen); err != nil {
		t.Fatal(err)
	}
	if err := c.EnsureMutable(); !errors.Is(err, ErrEvidenceLocked) {
		t.Fatalf("冻结后应不可变，得到 %v", err)
	}
}

func TestSnapshotDigestIgnoresCollectionOrder(t *testing.T) {
	at := time.Date(2026, 8, 27, 2, 0, 0, 0, time.UTC)
	base := FrozenSnapshot{CampaignID: "C", TheatreName: "剧场", Year: 2026, FrozenAt: at, Assets: []RiggingAsset{{ID: "2", AssetCode: "B", AssetType: AssetWinch, RatedLoadKg: 2, SafetyDevices: []string{"制动", "限位"}}, {ID: "1", AssetCode: "A", AssetType: AssetPoweredBar, RatedLoadKg: 1}}, Plan: InspectionPlan{Revision: 1, Checkpoints: []Checkpoint{{Code: "z", Name: "Z", AssetTypes: []AssetType{AssetWinch, AssetPoweredBar}, Unit: "mm", Comparison: CompareMaximum, Threshold: 1}, {Code: "a", Name: "A", AssetTypes: []AssetType{AssetPoweredBar}, Unit: "bool", Comparison: CompareBoolean, Threshold: 1}}}}
	other := base
	other.Assets = []RiggingAsset{base.Assets[1], base.Assets[0]}
	other.Assets[1].SafetyDevices = []string{"限位", "制动"}
	other.Plan.Checkpoints = []Checkpoint{base.Plan.Checkpoints[1], base.Plan.Checkpoints[0]}
	other.Plan.Checkpoints[1].AssetTypes = []AssetType{AssetPoweredBar, AssetWinch}
	a, err := SnapshotDigest(base)
	if err != nil {
		t.Fatal(err)
	}
	b, err := SnapshotDigest(other)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("规范摘要应一致: %s != %s", a, b)
	}
}
