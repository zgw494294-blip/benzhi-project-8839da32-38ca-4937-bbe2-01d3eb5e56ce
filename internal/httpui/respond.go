package httpui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"stage-rigging-safety-release/internal/domain"
)

const maxBody = 1 << 20

func decode(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("解析 JSON 请求: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return domain.NewRuleError("single_json_required", "请求体必须只包含一个 JSON 对象")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := "internal_error"
	message := "服务器处理请求失败"
	details := []string(nil)
	var rule *domain.RuleError
	switch {
	case errors.As(err, &rule):
		status = http.StatusUnprocessableEntity
		code = rule.Code
		message = rule.Message
		details = rule.Details
		if rule.Code == "plan_digest_mismatch" || rule.Code == "freeze_digest_drift" {
			status = http.StatusConflict
		}
	case errors.Is(err, domain.ErrNotFound):
		status = http.StatusNotFound
		code = "not_found"
		message = err.Error()
	case errors.Is(err, domain.ErrConflict):
		status = http.StatusConflict
		code = "version_conflict"
		message = err.Error()
	case errors.Is(err, domain.ErrIdempotency):
		status = http.StatusConflict
		code = "idempotency_conflict"
		message = err.Error()
	case errors.Is(err, domain.ErrForbidden):
		status = http.StatusForbidden
		code = "forbidden"
		message = err.Error()
	case errors.Is(err, domain.ErrEvidenceLocked):
		status = http.StatusLocked
		code = "evidence_locked"
		message = err.Error()
	case errors.Is(err, domain.ErrInvalidState):
		status = http.StatusConflict
		code = "invalid_state"
		message = err.Error()
	case errors.Is(err, domain.ErrValidation):
		status = http.StatusUnprocessableEntity
		code = "validation_failed"
		message = err.Error()
	}
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "details": details}})
}
