package domain

import "time"

type DefectStatus string
type DefectSeverity string

const (
	DefectOpen         DefectStatus   = "open"
	DefectRemediated   DefectStatus   = "remediated"
	DefectRetestFailed DefectStatus   = "retest_failed"
	DefectClosed       DefectStatus   = "closed"
	SeverityMajor      DefectSeverity = "major"
	SeverityCritical   DefectSeverity = "critical"
)

type RemedyRound struct {
	ID         string    `json:"id"`
	Round      int       `json:"round"`
	Remedy     string    `json:"remedy"`
	Owner      string    `json:"owner"`
	RecordedAt time.Time `json:"recordedAt"`
}

type RetestRound struct {
	Round            int       `json:"round"`
	RevisionID       string    `json:"revisionId"`
	Passed           bool      `json:"passed"`
	Evaluation       string    `json:"evaluation"`
	AttemptedAt      time.Time `json:"attemptedAt"`
	SourceRevisionID string    `json:"sourceRevisionId"`
}

type DefectCase struct {
	ID               string         `json:"id"`
	CampaignID       string         `json:"campaignId"`
	AssetID          string         `json:"assetId"`
	CheckpointCode   string         `json:"checkpointCode"`
	Severity         DefectSeverity `json:"severity"`
	Reason           string         `json:"reason"`
	Status           DefectStatus   `json:"status"`
	Remedy           string         `json:"remedy,omitempty"`
	Owner            string         `json:"owner,omitempty"`
	SourceRevisionID string         `json:"sourceRevisionId"`
	RetestRevisionID string         `json:"retestRevisionId,omitempty"`
	ClosedAt         *time.Time     `json:"closedAt,omitempty"`
	RemedyRounds     []RemedyRound  `json:"remedyRounds,omitempty"`
	RetestRounds     []RetestRound  `json:"retestRounds,omitempty"`
}

func (d *DefectCase) RecordRemedy(remedy, owner string) error {
	if d.Status == DefectClosed {
		return NewRuleError("defect_closed", "已关闭缺陷不能重复整改")
	}
	if remedy == "" || owner == "" {
		return NewRuleError("remedy_required", "整改措施和责任人不能为空")
	}
	d.Remedy, d.Owner, d.Status = remedy, owner, DefectRemediated
	return nil
}

func (d *DefectCase) RecordRemedyRound(id, remedy, owner string, at time.Time) error {
	if err := d.RecordRemedy(remedy, owner); err != nil {
		return err
	}
	d.RemedyRounds = append(d.RemedyRounds, RemedyRound{ID: id, Round: len(d.RemedyRounds) + 1, Remedy: remedy, Owner: owner, RecordedAt: at})
	return nil
}

func (d *DefectCase) RecordRetest(revision MeasurementRevision, passed bool, at time.Time) error {
	if d.Status != DefectRemediated {
		return NewRuleError("remedy_first", "每次复验前必须登记新一轮整改措施")
	}
	d.RetestRounds = append(d.RetestRounds, RetestRound{Round: len(d.RetestRounds) + 1, RevisionID: revision.ID, Passed: passed, Evaluation: revision.Evaluation, AttemptedAt: revision.MeasuredAt, SourceRevisionID: revision.SupersedesID})
	if !passed {
		d.Status = DefectRetestFailed
		d.RetestRevisionID = ""
		d.ClosedAt = nil
		return nil
	}
	return d.Close(revision.ID, at)
}

func (d *DefectCase) Close(retestID string, at time.Time) error {
	if d.Status != DefectRemediated {
		return NewRuleError("remedy_first", "必须先登记整改措施")
	}
	if retestID == "" {
		return NewRuleError("retest_required", "必须关联复验修订")
	}
	d.Status, d.RetestRevisionID, d.ClosedAt = DefectClosed, retestID, &at
	return nil
}
