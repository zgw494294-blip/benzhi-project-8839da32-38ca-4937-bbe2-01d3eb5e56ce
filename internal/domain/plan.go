package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type Comparison string

const (
	CompareMaximum Comparison = "maximum"
	CompareMinimum Comparison = "minimum"
	CompareBoolean Comparison = "boolean"
)

type Checkpoint struct {
	Code             string      `json:"code"`
	Name             string      `json:"name"`
	AssetTypes       []AssetType `json:"assetTypes"`
	Unit             string      `json:"unit"`
	Comparison       Comparison  `json:"comparison"`
	Threshold        float64     `json:"threshold"`
	Critical         bool        `json:"critical"`
	InstrumentMaxAge int         `json:"instrumentMaxAgeDays"`
}

type InspectionPlan struct {
	ID            string       `json:"id"`
	CampaignID    string       `json:"campaignId"`
	Revision      int          `json:"revision"`
	Status        string       `json:"status"`
	Checkpoints   []Checkpoint `json:"checkpoints"`
	ConfirmedBy   string       `json:"confirmedBy,omitempty"`
	ConfirmedAt   *time.Time   `json:"confirmedAt,omitempty"`
	ContentDigest string       `json:"contentDigest"`
}

func (p InspectionPlan) Validate() error {
	if p.Revision < 1 || len(p.Checkpoints) == 0 {
		return NewRuleError("invalid_plan", "方案必须包含修订号和检查点")
	}
	seen := map[string]bool{}
	issues := []string{}
	for i, cp := range p.Checkpoints {
		if strings.TrimSpace(cp.Code) == "" || strings.TrimSpace(cp.Name) == "" || len(cp.AssetTypes) == 0 {
			issues = append(issues, fmt.Sprintf("第 %d 个检查点：代码、名称和适用设备不能为空", i+1))
		}
		code := strings.ToLower(strings.TrimSpace(cp.Code))
		if seen[code] {
			issues = append(issues, fmt.Sprintf("第 %d 个检查点：代码 %s 重复", i+1, code))
		}
		seen[code] = true
		if cp.Comparison != CompareMaximum && cp.Comparison != CompareMinimum && cp.Comparison != CompareBoolean {
			issues = append(issues, fmt.Sprintf("第 %d 个检查点：比较方式无效", i+1))
		}
		unit := strings.ToLower(strings.TrimSpace(cp.Unit))
		if cp.Comparison == CompareBoolean && (unit != "bool" || cp.Threshold != 1) {
			issues = append(issues, fmt.Sprintf("第 %d 个检查点：boolean 比较必须使用 bool 单位且阈值为 1", i+1))
		}
		if cp.Comparison != CompareBoolean && (unit == "" || unit == "bool") {
			issues = append(issues, fmt.Sprintf("第 %d 个检查点：数值比较必须使用非 bool 单位", i+1))
		}
		if math.IsNaN(cp.Threshold) || math.IsInf(cp.Threshold, 0) || (unit == "percent" && (cp.Threshold < 0 || cp.Threshold > 100)) || (cp.Comparison != CompareBoolean && cp.Threshold < 0) {
			issues = append(issues, fmt.Sprintf("第 %d 个检查点：阈值超出允许范围", i+1))
		}
		if cp.InstrumentMaxAge < 1 || cp.InstrumentMaxAge > 3650 {
			issues = append(issues, fmt.Sprintf("第 %d 个检查点：仪器最大校准周期必须为 1 至 3650 天", i+1))
		}
		assetKinds := map[AssetType]bool{}
		for _, kind := range cp.AssetTypes {
			if kind != AssetCounterweightBar && kind != AssetPoweredBar && kind != AssetWinch {
				issues = append(issues, fmt.Sprintf("第 %d 个检查点：包含无效设备类别 %s", i+1, kind))
			}
			if assetKinds[kind] {
				issues = append(issues, fmt.Sprintf("第 %d 个检查点：设备类别 %s 重复", i+1, kind))
			}
			assetKinds[kind] = true
		}
	}
	if len(issues) > 0 {
		return NewRuleError("plan_invalid", "检验方案校验失败", issues...)
	}
	return nil
}

func NormalizeCheckpoints(values []Checkpoint) []Checkpoint {
	result := append([]Checkpoint(nil), values...)
	for i := range result {
		result[i].Code = strings.ToLower(strings.TrimSpace(result[i].Code))
		result[i].Name = strings.TrimSpace(result[i].Name)
		result[i].Unit = strings.ToLower(strings.TrimSpace(result[i].Unit))
		result[i].AssetTypes = append([]AssetType(nil), result[i].AssetTypes...)
		sort.Slice(result[i].AssetTypes, func(a, b int) bool { return result[i].AssetTypes[a] < result[i].AssetTypes[b] })
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Code < result[j].Code })
	return result
}

func PlanCandidateDigest(assets []RiggingAsset, checkpoints []Checkpoint) string {
	type scopeItem struct {
		Code string    `json:"code"`
		Type AssetType `json:"type"`
	}
	scope := make([]scopeItem, 0, len(assets))
	for _, asset := range assets {
		scope = append(scope, scopeItem{Code: NormalizeAssetCode(asset.AssetCode), Type: asset.AssetType})
	}
	sort.Slice(scope, func(i, j int) bool { return scope[i].Code < scope[j].Code })
	payload := struct {
		Scope       []scopeItem  `json:"scope"`
		Checkpoints []Checkpoint `json:"checkpoints"`
	}{scope, NormalizeCheckpoints(checkpoints)}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (p InspectionPlan) Digest() string {
	cp := NormalizeCheckpoints(p.Checkpoints)
	b, _ := json.Marshal(cp)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func (p InspectionPlan) Checkpoint(code string) (Checkpoint, bool) {
	for _, cp := range p.Checkpoints {
		if cp.Code == code {
			return cp, true
		}
	}
	return Checkpoint{}, false
}

func (cp Checkpoint) AppliesTo(kind AssetType) bool {
	for _, candidate := range cp.AssetTypes {
		if candidate == kind {
			return true
		}
	}
	return false
}
