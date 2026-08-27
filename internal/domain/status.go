package domain

type CampaignStatus string

const (
	StatusDraft         CampaignStatus = "draft"
	StatusExecuting     CampaignStatus = "executing"
	StatusReviewPending CampaignStatus = "review_pending"
	StatusApproved      CampaignStatus = "approved"
	StatusFrozen        CampaignStatus = "frozen"
	StatusLicensed      CampaignStatus = "licensed"
)

var transitions = map[CampaignStatus]map[CampaignStatus]bool{
	StatusDraft:         {StatusExecuting: true},
	StatusExecuting:     {StatusReviewPending: true},
	StatusReviewPending: {StatusExecuting: true, StatusApproved: true},
	StatusApproved:      {StatusFrozen: true},
	StatusFrozen:        {StatusLicensed: true},
}

func CanTransition(from, to CampaignStatus) bool { return transitions[from][to] }

func (c *InspectionCampaign) Transition(to CampaignStatus) error {
	if !CanTransition(c.Status, to) {
		return NewRuleError("invalid_transition", "不允许的任务状态迁移", string(c.Status)+" -> "+string(to))
	}
	c.Status = to
	return nil
}

func (c *InspectionCampaign) EnsureMutable() error {
	if c.Status == StatusFrozen || c.Status == StatusLicensed {
		return ErrEvidenceLocked
	}
	return nil
}
