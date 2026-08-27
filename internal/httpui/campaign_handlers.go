package httpui

import (
	"net/http"
	"stage-rigging-safety-release/internal/application"
	"stage-rigging-safety-release/internal/domain"
)

func (s *Server) WorkbenchHandler(w http.ResponseWriter, r *http.Request) {
	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "工作台资源不可用", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}
func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
func (s *Server) ListCampaignsHandler(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.ListCampaigns(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"campaigns": items})
}
func (s *Server) CreateCampaignHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.CreateCampaignCommand
	if err := decode(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.CreateCampaign(r.Context(), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 201, result)
}
func (s *Server) GetCampaignHandler(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.GetCampaign(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) AddAssetHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		application.Metadata
		Asset  *domain.RiggingAsset  `json:"asset,omitempty"`
		Assets []domain.RiggingAsset `json:"assets,omitempty"`
	}
	if err := decode(w, r, &request); err != nil {
		writeError(w, err)
		return
	}
	assets := request.Assets
	if request.Asset != nil {
		assets = append(assets, *request.Asset)
	}
	result, err := s.app.AddAssets(r.Context(), r.PathValue("id"), application.AddAssetsCommand{Metadata: request.Metadata, Assets: assets})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) PreflightPlanHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.PlanPreflightCommand
	if err := decode(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.PreflightPlan(r.Context(), r.PathValue("id"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) ConfirmPlanHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.ConfirmPlanCommand
	if err := decode(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.ConfirmPlan(r.Context(), r.PathValue("id"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) SubmitMeasurementHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.SubmitMeasurementCommand
	if err := decode(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.SubmitMeasurements(r.Context(), r.PathValue("id"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 201, result)
}

func (s *Server) DeviceChecklistHandler(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.DeviceChecklist(r.Context(), r.PathValue("id"), r.PathValue("assetID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
