package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"
)

func CanonicalSnapshot(snapshot FrozenSnapshot) ([]byte, error) {
	// 冻结时间是事务元数据，不参与候选内容摘要，确保预览与落库摘要一致。
	snapshot.FrozenAt = time.Time{}
	for i := range snapshot.Assets {
		snapshot.Assets[i].SafetyDevices = append([]string(nil), snapshot.Assets[i].SafetyDevices...)
		sort.Strings(snapshot.Assets[i].SafetyDevices)
	}
	sort.Slice(snapshot.Assets, func(i, j int) bool { return snapshot.Assets[i].AssetCode < snapshot.Assets[j].AssetCode })
	for i := range snapshot.Plan.Checkpoints {
		snapshot.Plan.Checkpoints[i].AssetTypes = append([]AssetType(nil), snapshot.Plan.Checkpoints[i].AssetTypes...)
		sort.Slice(snapshot.Plan.Checkpoints[i].AssetTypes, func(a, b int) bool {
			return snapshot.Plan.Checkpoints[i].AssetTypes[a] < snapshot.Plan.Checkpoints[i].AssetTypes[b]
		})
	}
	sort.Slice(snapshot.Plan.Checkpoints, func(i, j int) bool { return snapshot.Plan.Checkpoints[i].Code < snapshot.Plan.Checkpoints[j].Code })
	sort.Slice(snapshot.Measurements, func(i, j int) bool {
		a, b := snapshot.Measurements[i], snapshot.Measurements[j]
		if a.AssetID != b.AssetID {
			return a.AssetID < b.AssetID
		}
		if a.CheckpointCode != b.CheckpointCode {
			return a.CheckpointCode < b.CheckpointCode
		}
		return a.Revision < b.Revision
	})
	sort.Slice(snapshot.Defects, func(i, j int) bool { return snapshot.Defects[i].ID < snapshot.Defects[j].ID })
	return json.Marshal(snapshot)
}

func SnapshotDigest(snapshot FrozenSnapshot) (string, error) {
	b, err := CanonicalSnapshot(snapshot)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
