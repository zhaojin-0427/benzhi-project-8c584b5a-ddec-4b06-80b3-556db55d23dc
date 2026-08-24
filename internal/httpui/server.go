package httpui

import (
	"embed"
	"io/fs"
	"net/http"
	"time"

	"manuscript-conservation-gate/internal/application"
)

//go:embed assets/*
var assets embed.FS

type Server struct {
	service *application.Service
	mux     *http.ServeMux
	started time.Time
}

func New(service *application.Service) http.Handler {
	s := &Server{service: service, mux: http.NewServeMux(), started: time.Now().UTC()}
	s.routes()
	return s.security(s.mux)
}

func (s *Server) routes() {
	static, _ := fs.Sub(assets, "assets")
	s.mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(static))))
	s.mux.HandleFunc("GET /", s.Workbench)
	s.mux.HandleFunc("GET /healthz", s.Health)
	s.mux.HandleFunc("GET /api/cases", s.ListCases)
	s.mux.HandleFunc("POST /api/cases", s.CreateCase)
	s.mux.HandleFunc("GET /api/cases/{caseID}", s.GetCase)
	s.mux.HandleFunc("POST /api/cases/{caseID}/damages", s.AddDamage)
	s.mux.HandleFunc("POST /api/cases/{caseID}/plans", s.CreatePlan)
	s.mux.HandleFunc("POST /api/cases/{caseID}/submit", s.SubmitPlan)
	s.mux.HandleFunc("POST /api/cases/{caseID}/assessments", s.AssessCompatibility)
	s.mux.HandleFunc("POST /api/cases/{caseID}/samples", s.RecordSample)
	s.mux.HandleFunc("POST /api/cases/{caseID}/reviews", s.Review)
	s.mux.HandleFunc("POST /api/cases/{caseID}/release", s.Release)
	s.mux.HandleFunc("GET /api/cases/{caseID}/credential/verify", s.VerifyCredential)
}

func (s *Server) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
