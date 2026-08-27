package inspection

import (
	"fmt"
	"time"

	"stage-rigging-safety-release/internal/domain"
)

func DefectFor(campaignID string, measurement domain.MeasurementRevision, evaluation Evaluation) *domain.DefectCase {
	if evaluation.Passed {
		return nil
	}
	return &domain.DefectCase{
		ID: fmt.Sprintf("DEF-%s", measurement.ID), CampaignID: campaignID, AssetID: measurement.AssetID,
		CheckpointCode: measurement.CheckpointCode, Severity: evaluation.Severity, Reason: evaluation.Reason,
		Status: domain.DefectOpen, SourceRevisionID: measurement.ID,
	}
}

func ValidateRetest(defect domain.DefectCase, original, latest, retest domain.MeasurementRevision, evaluated Evaluation, now time.Time) error {
	if defect.Status != domain.DefectRemediated {
		return domain.NewRuleError("remedy_first", "复验前必须登记整改措施")
	}
	if retest.AssetID != defect.AssetID || retest.CheckpointCode != defect.CheckpointCode {
		return domain.NewRuleError("retest_mismatch", "复验必须对应原设备和检查点")
	}
	if original.ID != defect.SourceRevisionID {
		return domain.NewRuleError("source_mismatch", "缺陷原始证据链不一致")
	}
	if retest.Revision != latest.Revision+1 || retest.SupersedesID != latest.ID {
		return domain.NewRuleError("retest_revision", "复验必须沿最新证据建立连续更新修订")
	}
	if retest.MeasuredAt.After(now.Add(5 * time.Minute)) {
		return domain.NewRuleError("retest_time", "复验时间无效")
	}
	return nil
}
