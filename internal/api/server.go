package api

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"llmsnare/internal/benchmark"
	"llmsnare/internal/storage"
)

//go:embed openapi.yaml
var openAPISpec []byte

type Server struct {
	store *storage.Store
}

type timelineEntry struct {
	Timestamp       time.Time                   `json:"timestamp"`
	FinishedAt      time.Time                   `json:"finished_at"`
	CaseID          string                      `json:"case_id"`
	Profile         string                      `json:"profile"`
	Provider        string                      `json:"provider"`
	Model           string                      `json:"model"`
	Success         bool                        `json:"success"`
	Error           string                      `json:"error,omitempty"`
	TotalScore      int                         `json:"total_score"`
	RawScore        int                         `json:"raw_score"`
	MaxScore        int                         `json:"max_score"`
	NormalizedScore float64                     `json:"normalized_score"`
	Metrics         benchmark.Metrics           `json:"metrics"`
	Deductions      []benchmark.ScoreAdjustment `json:"deductions,omitempty"`
	Bonuses         []timelineBonus             `json:"bonuses,omitempty"`
}

type timelineBonus struct {
	Name   string `json:"name"`
	Points int    `json:"points"`
}

func NewServer(store *storage.Store) *Server {
	return &Server{store: store}
}

func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	server := &http.Server{
		Addr:    addr,
		Handler: s.routes(),
	}

	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()

	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/openapi.yaml", s.handleOpenAPI)
	mux.HandleFunc("/api/v1/timelines", s.handleTimelines)
	mux.HandleFunc("/api/v1/timelines/", s.handleTimelineProfile)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(openAPISpec)
}

func (s *Server) handleTimelines(w http.ResponseWriter, r *http.Request) {
	limit, err := parseLimit(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	data, err := s.store.LoadAll(limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": projectTimelineGroups(data)})
}

func (s *Server) handleTimelineProfile(w http.ResponseWriter, r *http.Request) {
	limit, err := parseLimit(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	profile := strings.TrimPrefix(r.URL.Path, "/api/v1/timelines/")
	if profile == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not found"})
		return
	}

	entries, err := s.store.LoadProfile(profile, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"profile": profile,
		"entries": projectTimelineEntries(entries),
	})
}

func projectTimelineGroups(groups map[string][]benchmark.Result) map[string][]timelineEntry {
	projected := make(map[string][]timelineEntry, len(groups))
	for profile, entries := range groups {
		projected[profile] = projectTimelineEntries(entries)
	}
	return projected
}

func projectTimelineEntries(entries []benchmark.Result) []timelineEntry {
	projected := make([]timelineEntry, 0, len(entries))
	for _, entry := range entries {
		projected = append(projected, projectTimelineEntry(entry))
	}
	return projected
}

func projectTimelineEntry(entry benchmark.Result) timelineEntry {
	return timelineEntry{
		Timestamp:       entry.Timestamp,
		FinishedAt:      entry.FinishedAt,
		CaseID:          entry.CaseID,
		Profile:         entry.Profile,
		Provider:        entry.Provider,
		Model:           entry.Model,
		Success:         entry.Success,
		Error:           entry.Error,
		TotalScore:      entry.TotalScore,
		RawScore:        entry.RawScore,
		MaxScore:        entry.MaxScore,
		NormalizedScore: entry.NormalizedScore,
		Metrics:         entry.Metrics,
		Deductions:      entry.Deductions,
		Bonuses:         projectTimelineBonuses(entry.Bonuses),
	}
}

func projectTimelineBonuses(items []benchmark.ScoreAdjustment) []timelineBonus {
	projected := make([]timelineBonus, 0, len(items))
	for _, item := range items {
		projected = append(projected, timelineBonus{
			Name:   item.Name,
			Points: item.Points,
		})
	}
	return projected
}

func parseLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 0 {
		return 0, fmt.Errorf("limit must be a non-negative integer")
	}
	return limit, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
