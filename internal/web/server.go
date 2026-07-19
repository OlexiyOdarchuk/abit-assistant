// Package web is the HTTP presentation layer: a JSON API over the same
// internal/service use cases the Telegram bot uses, plus the embedded Svelte
// frontend. It owns no domain logic — handlers map requests to service calls
// and shape the results.
package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
)

// Deps bundles everything the server needs from the outside. cmd/web is the
// only place that should construct it.
type Deps struct {
	Program   *service.ProgramService
	Discover  *service.DiscoverService
	Simulate  *service.PrioritySimulator
	Applicant *service.ApplicantService
	Logger    *slog.Logger
}

// Server is the HTTP app: the JSON API plus the embedded SPA.
type Server struct {
	deps    Deps
	log     *slog.Logger
	mux     *http.ServeMux
	predict *service.PriorityPredictor
}

// New wires the routes. The returned Server is an http.Handler.
func New(deps Deps) *Server {
	log := deps.Logger
	if log == nil {
		log = slog.Default()
	}
	s := &Server{deps: deps, log: log.With("component", "web"), mux: http.NewServeMux()}
	s.predict = service.NewPriorityPredictor(deps.Program).WithLogger(s.log)

	s.mux.HandleFunc("GET /api/filters", s.handleFilters)
	s.mux.HandleFunc("POST /api/analyze", s.handleAnalyze)
	s.mux.HandleFunc("POST /api/discover", s.handleDiscover)
	s.mux.HandleFunc("POST /api/simulate", s.handleSimulate)
	s.mux.HandleFunc("POST /api/applicant", s.handleApplicant)
	s.mux.HandleFunc("POST /api/predict", s.handlePredict)
	s.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Everything else → the embedded SPA (with deep-link fallback).
	s.mux.Handle("/", staticHandler())
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

// --- request timeout helper ---

// apiTimeout caps a single API request. Discovery/simulation fan out to
// osvita + abit-poisk, so it matches the bot's search timeout.
const apiTimeout = 90 * time.Second

// --- JSON helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errResp struct {
	Error string `json:"error"`
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errResp{Error: msg})
}

// decodeBody reads a JSON request body into v, rejecting unknown fields and
// oversized payloads.
func decodeBody(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
