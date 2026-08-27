package httpui

import (
	"embed"
	"io/fs"
	"net/http"
	"stage-rigging-safety-release/internal/application"
)

//go:embed static/*
var staticFiles embed.FS

type Server struct {
	app *application.Service
	mux *http.ServeMux
}

func New(app *application.Service) *Server {
	s := &Server{app: app, mux: http.NewServeMux()}
	s.routes()
	return s
}
func (s *Server) Handler() http.Handler { return securityHeaders(s.mux) }

func (s *Server) routes() {
	assets, _ := fs.Sub(staticFiles, "static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(assets))))
	s.mux.HandleFunc("GET /", s.WorkbenchHandler)
	s.mux.HandleFunc("GET /healthz", s.HealthHandler)
	s.mux.HandleFunc("GET /api/v1/campaigns", s.ListCampaignsHandler)
	s.mux.HandleFunc("POST /api/v1/campaigns", s.CreateCampaignHandler)
	s.mux.HandleFunc("GET /api/v1/campaigns/{id}", s.GetCampaignHandler)
	s.mux.HandleFunc("POST /api/v1/campaigns/{id}/assets", s.AddAssetHandler)
	s.mux.HandleFunc("GET /api/v1/campaigns/{id}/assets/{assetID}/checklist", s.DeviceChecklistHandler)
	s.mux.HandleFunc("POST /api/v1/campaigns/{id}/plans/preflight", s.PreflightPlanHandler)
	s.mux.HandleFunc("POST /api/v1/campaigns/{id}/plans/confirm", s.ConfirmPlanHandler)
	s.mux.HandleFunc("POST /api/v1/campaigns/{id}/measurements", s.SubmitMeasurementHandler)
	s.mux.HandleFunc("POST /api/v1/campaigns/{id}/defects/{defectID}/remedy", s.RecordRemedyHandler)
	s.mux.HandleFunc("POST /api/v1/campaigns/{id}/defects/{defectID}/retest", s.SubmitRetestHandler)
	s.mux.HandleFunc("POST /api/v1/campaigns/{id}/review/submit", s.SubmitReviewHandler)
	s.mux.HandleFunc("POST /api/v1/campaigns/{id}/review/decision", s.ReviewDecisionHandler)
	s.mux.HandleFunc("POST /api/v1/campaigns/{id}/freeze", s.FreezeHandler)
	s.mux.HandleFunc("GET /api/v1/campaigns/{id}/freeze/preview", s.FreezePreviewHandler)
	s.mux.HandleFunc("POST /api/v1/campaigns/{id}/permit", s.IssuePermitHandler)
	s.mux.HandleFunc("GET /api/v1/campaigns/{id}/timeline", s.TimelineHandler)
	s.mux.HandleFunc("GET /api/v1/permits/{number}/verify", s.VerifyPermitHandler)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}
