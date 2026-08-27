package httpui

import (
	"net/http"
	"stage-rigging-safety-release/internal/application"
)

func (s *Server) RecordRemedyHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.RemedyCommand
	if err := decode(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	cmd.DefectID = r.PathValue("defectID")
	result, err := s.app.RecordRemedy(r.Context(), r.PathValue("id"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) SubmitRetestHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.RetestCommand
	if err := decode(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	cmd.DefectID = r.PathValue("defectID")
	result, err := s.app.SubmitRetest(r.Context(), r.PathValue("id"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 201, result)
}
func (s *Server) SubmitReviewHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.ReviewSubmitCommand
	if err := decode(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.SubmitForReview(r.Context(), r.PathValue("id"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) ReviewDecisionHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.ReviewDecisionCommand
	if err := decode(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.DecideReview(r.Context(), r.PathValue("id"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) FreezeHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.FreezeCommand
	if err := decode(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.Freeze(r.Context(), r.PathValue("id"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) FreezePreviewHandler(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.PreviewFreeze(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) IssuePermitHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.IssuePermitCommand
	if err := decode(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.IssuePermit(r.Context(), r.PathValue("id"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 201, result)
}
func (s *Server) TimelineHandler(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.Timeline(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"events": result})
}
func (s *Server) VerifyPermitHandler(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.VerifyPermit(r.Context(), r.PathValue("number"), r.URL.Query().Get("assetCode"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
