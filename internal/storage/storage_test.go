package storage

import (
	"context"
	"errors"
	"path/filepath"
	"stage-rigging-safety-release/internal/domain"
	"testing"
	"time"
)

func TestSQLitePersistenceVersionAndIdempotency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inspection.db")
	ctx := context.Background()
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	c, err := domain.NewCampaign("C1", "持久化剧场", 2026, "检验员", now)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Create(ctx, c, "create-1", "检验员", "inspector"); err != nil {
		t.Fatal(err)
	}
	mutate := func(c *domain.InspectionCampaign) (any, error) {
		c.TheatreName = "更新后的剧场"
		return map[string]string{"changed": "name"}, nil
	}
	updated, replayed, err := store.Mutate(ctx, "C1", 1, "update-1", "campaign.updated", "检验员", "inspector", mutate)
	if err != nil || replayed || updated.Version != 2 {
		t.Fatalf("首次更新失败: replay=%v version=%v err=%v", replayed, updated.Version, err)
	}
	replayedCampaign, replayed, err := store.Mutate(ctx, "C1", 1, "update-1", "campaign.updated", "检验员", "inspector", func(*domain.InspectionCampaign) (any, error) {
		t.Fatal("幂等重放不应再次调用变更函数")
		return nil, nil
	})
	if err != nil || !replayed || replayedCampaign.Version != 2 {
		t.Fatalf("幂等重放失败: %+v %v %v", replayedCampaign, replayed, err)
	}
	_, _, err = store.Mutate(ctx, "C1", 1, "update-2", "campaign.updated", "检验员", "inspector", mutate)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("陈旧版本应冲突: %v", err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	loaded, err := store.Get(ctx, "C1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.TheatreName != "更新后的剧场" || loaded.Version != 2 {
		t.Fatalf("重启后数据不一致: %+v", loaded)
	}
	events, err := store.Timeline(ctx, "C1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Action != "campaign.updated" {
		t.Fatalf("审计时间线不完整: %+v", events)
	}
}
