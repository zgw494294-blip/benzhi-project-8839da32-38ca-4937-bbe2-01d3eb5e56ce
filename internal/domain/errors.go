package domain

import "errors"

var (
	ErrNotFound       = errors.New("记录不存在")
	ErrConflict       = errors.New("版本冲突，请刷新后重试")
	ErrValidation     = errors.New("输入不符合业务规则")
	ErrInvalidState   = errors.New("当前状态不允许此操作")
	ErrEvidenceLocked = errors.New("证据包已冻结，禁止继续改写")
	ErrIdempotency    = errors.New("幂等键已用于其他操作")
	ErrForbidden      = errors.New("当前角色无权执行此操作")
)

type RuleError struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

func (e *RuleError) Error() string { return e.Message }

func NewRuleError(code, message string, details ...string) error {
	return &RuleError{Code: code, Message: message, Details: details}
}
