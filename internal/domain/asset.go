package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type AssetType string

const (
	AssetCounterweightBar AssetType = "counterweight_bar"
	AssetPoweredBar       AssetType = "powered_bar"
	AssetWinch            AssetType = "winch"
)

type RiggingAsset struct {
	ID             string    `json:"id"`
	CampaignID     string    `json:"campaignId"`
	AssetCode      string    `json:"assetCode"`
	AssetType      AssetType `json:"assetType"`
	RatedLoadKg    float64   `json:"ratedLoadKg"`
	DriveType      string    `json:"driveType"`
	SafetyDevices  []string  `json:"safetyDevices"`
	CommissionedOn string    `json:"commissionedOn"`
}

func NormalizeAssetCode(value string) string {
	return strings.ToUpper(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value)))
}

func ValidAssetCode(value string) bool {
	if value == "" || utf8.RuneCountInString(value) > 64 {
		return false
	}
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("-_./", r) {
			return false
		}
	}
	return true
}

func NormalizeSafetyDevices(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		name := strings.Join(strings.Fields(value), "")
		if name == "上下限位" {
			for _, limit := range []string{"上限位", "下限位"} {
				if !seen[limit] {
					seen[limit] = true
					result = append(result, limit)
				}
			}
			continue
		}
		if name != "" && !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result
}

func (a *RiggingAsset) Normalize() {
	a.AssetCode = NormalizeAssetCode(a.AssetCode)
	a.DriveType = strings.TrimSpace(a.DriveType)
	a.SafetyDevices = NormalizeSafetyDevices(a.SafetyDevices)
}

func (a RiggingAsset) Validate() error {
	if !ValidAssetCode(NormalizeAssetCode(a.AssetCode)) || strings.TrimSpace(a.ID) == "" {
		return NewRuleError("invalid_asset", "设备编号和 ID 不能为空")
	}
	if a.AssetType != AssetCounterweightBar && a.AssetType != AssetPoweredBar && a.AssetType != AssetWinch {
		return NewRuleError("invalid_asset_type", "不支持的设备类别", string(a.AssetType))
	}
	if a.RatedLoadKg <= 0 {
		return NewRuleError("invalid_rated_load", "设备额定载荷必须大于零")
	}
	if a.RatedLoadKg > 1000000 {
		return NewRuleError("invalid_rated_load", "设备额定载荷超出允许范围")
	}
	if a.CommissionedOn != "" {
		commissioned, err := time.Parse("2006-01-02", a.CommissionedOn)
		if err != nil {
			return NewRuleError("invalid_commissioned_on", "投用日期必须为 YYYY-MM-DD")
		}
		if commissioned.After(time.Now().UTC().Add(24 * time.Hour)) {
			return NewRuleError("invalid_commissioned_on", "投用日期不得晚于当前日期")
		}
	}
	return nil
}

func (a RiggingAsset) ValidateScopeRow() error {
	if err := a.Validate(); err != nil {
		return err
	}
	if a.CommissionedOn == "" {
		return NewRuleError("commissioned_on_required", "必须登记投用日期")
	}
	if strings.TrimSpace(a.DriveType) == "" {
		return NewRuleError("drive_type_required", "必须登记驱动方式")
	}
	required := []string{"上限位", "下限位"}
	if a.AssetType == AssetPoweredBar || a.AssetType == AssetWinch {
		required = append(required, "制动器")
	}
	has := map[string]bool{}
	for _, item := range a.SafetyDevices {
		has[item] = true
	}
	for _, item := range required {
		if !has[item] {
			return NewRuleError("safety_device_required", "缺少必需安全装置", item)
		}
	}
	return nil
}

func (c *InspectionCampaign) AddAssets(assets []RiggingAsset) error {
	if err := c.EnsureMutable(); err != nil {
		return err
	}
	if c.Status != StatusDraft {
		return NewRuleError("scope_locked", "方案执行后设备范围已锁定")
	}
	if len(assets) == 0 {
		return NewRuleError("assets_required", "批次至少包含一台设备")
	}
	existing := map[string]bool{}
	ids := map[string]bool{}
	for _, current := range c.Assets {
		existing[NormalizeAssetCode(current.AssetCode)] = true
		ids[current.ID] = true
	}
	batch := map[string]int{}
	prepared := make([]RiggingAsset, len(assets))
	issues := []string{}
	for i, asset := range assets {
		asset.Normalize()
		if err := asset.ValidateScopeRow(); err != nil {
			issues = append(issues, fmt.Sprintf("第 %d 行：%s", i+1, err.Error()))
		}
		if existing[asset.AssetCode] {
			issues = append(issues, fmt.Sprintf("第 %d 行：设备编号 %s 已在任务范围中", i+1, asset.AssetCode))
		}
		if prior, ok := batch[asset.AssetCode]; ok {
			issues = append(issues, fmt.Sprintf("第 %d 行：设备编号 %s 与第 %d 行重复", i+1, asset.AssetCode, prior))
		} else if asset.AssetCode != "" {
			batch[asset.AssetCode] = i + 1
		}
		if ids[asset.ID] {
			issues = append(issues, fmt.Sprintf("第 %d 行：设备 ID 重复", i+1))
		}
		ids[asset.ID] = true
		asset.CampaignID = c.ID
		prepared[i] = asset
	}
	if len(issues) > 0 {
		return NewRuleError("asset_batch_invalid", "设备批次校验失败", issues...)
	}
	c.Assets = append(c.Assets, prepared...)
	return nil
}

func (c *InspectionCampaign) AddAsset(asset RiggingAsset) error {
	if err := c.EnsureMutable(); err != nil {
		return err
	}
	if c.Status != StatusDraft {
		return NewRuleError("scope_locked", "方案执行后设备范围已锁定")
	}
	if err := asset.Validate(); err != nil {
		return err
	}
	for _, current := range c.Assets {
		if current.ID == asset.ID || current.AssetCode == asset.AssetCode {
			return NewRuleError("duplicate_asset", "设备 ID 或编号重复", asset.AssetCode)
		}
	}
	asset.CampaignID = c.ID
	c.Assets = append(c.Assets, asset)
	return nil
}
