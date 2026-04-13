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
	"sync"
	"time"

	"llmsnare/internal/benchmark"
	"llmsnare/internal/storage"
)

const maxTimelineEntries = 1024

//go:embed openapi.yaml
var openAPISpec []byte

type Server struct {
	store *storage.Store
	cache *responseCache
}

type timelineEntry struct {
	Timestamp         time.Time                   `json:"timestamp"`
	FinishedAt        time.Time                   `json:"finished_at"`
	CaseID            string                      `json:"case_id"`
	Profile           string                      `json:"profile"`
	Provider          string                      `json:"provider"`
	Model             string                      `json:"model"`
	ModelVendor       string                      `json:"model_vendor"`
	InferenceProvider string                      `json:"inference_provider"`
	Success           bool                        `json:"success"`
	TotalScore        int                         `json:"total_score"`
	RawScore          int                         `json:"raw_score"`
	MaxScore          int                         `json:"max_score"`
	NormalizedScore   float64                     `json:"normalized_score"`
	Metrics           benchmark.Metrics           `json:"metrics"`
	Deductions        []benchmark.ScoreAdjustment `json:"deductions,omitempty"`
	Bonuses           []timelineBonus             `json:"bonuses,omitempty"`
}

type timelineBonus struct {
	Name        string `json:"name"`
	Points      int    `json:"points"`
	Description string `json:"description"`
}

type responseCache struct {
	mu      sync.RWMutex
	entries map[string]cachedResponse
}

type cachedResponse struct {
	version string
	body    []byte
}

func NewServer(store *storage.Store) *Server {
	return &Server{
		store: store,
		cache: &responseCache{entries: make(map[string]cachedResponse)},
	}
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
	mux.HandleFunc("/v1/timelines", s.handleTimelines)
	mux.HandleFunc("/v1/timelines/", s.handleTimelineProfile)
	return withCORS(mux)
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

	version, err := s.store.AllVersion()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	body, err := s.cachedJSON(cacheKeyAllTimelines(limit), version, func() (any, error) {
		data, err := s.store.LoadAll(limit)
		if err != nil {
			return nil, err
		}
		return map[string]any{"profiles": projectTimelineGroups(data)}, nil
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSONBytes(w, http.StatusOK, body)
}

func (s *Server) handleTimelineProfile(w http.ResponseWriter, r *http.Request) {
	limit, err := parseLimit(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	profile := strings.TrimPrefix(r.URL.Path, "/v1/timelines/")
	if profile == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not found"})
		return
	}

	version, err := s.store.ProfileVersion(profile)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	body, err := s.cachedJSON(cacheKeyTimelineProfile(profile, limit), version, func() (any, error) {
		entries, err := s.store.LoadProfile(profile, limit)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"profile": profile,
			"entries": projectTimelineEntries(entries),
		}, nil
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSONBytes(w, http.StatusOK, body)
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
		Timestamp:         entry.Timestamp,
		FinishedAt:        entry.FinishedAt,
		CaseID:            entry.CaseID,
		Profile:           entry.Profile,
		Provider:          entry.Provider,
		Model:             entry.Model,
		ModelVendor:       entry.ModelVendor,
		InferenceProvider: entry.InferenceProvider,
		Success:           entry.Success,
		TotalScore:        entry.TotalScore,
		RawScore:          entry.RawScore,
		MaxScore:          entry.MaxScore,
		NormalizedScore:   entry.NormalizedScore,
		Metrics:           entry.Metrics,
		Deductions:        entry.Deductions,
		Bonuses:           projectTimelineBonuses(entry.Bonuses),
	}
}

func projectTimelineBonuses(items []benchmark.ScoreAdjustment) []timelineBonus {
	projected := make([]timelineBonus, 0, len(items))
	for _, item := range items {
		projected = append(projected, timelineBonus{
			Name:        item.Name,
			Points:      item.Points,
			Description: item.Description,
		})
	}
	return projected
}

func parseLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return maxTimelineEntries, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 0 {
		return 0, fmt.Errorf("limit must be a non-negative integer")
	}
	if limit == 0 || limit > maxTimelineEntries {
		return maxTimelineEntries, nil
	}
	return limit, nil
}

func cacheKeyAllTimelines(limit int) string {
	return "all:" + strconv.Itoa(limit)
}

func cacheKeyTimelineProfile(profile string, limit int) string {
	return "profile:" + profile + ":" + strconv.Itoa(limit)
}

func (s *Server) cachedJSON(key, version string, build func() (any, error)) ([]byte, error) {
	if s.cache != nil {
		if body, ok := s.cache.get(key, version); ok {
			return body, nil
		}
	}

	payload, err := build()
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if s.cache != nil {
		s.cache.set(key, version, body)
	}
	return body, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	writeJSONBytes(w, status, body)
}

func writeJSONBytes(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if len(body) == 0 {
		body = []byte("null")
	}
	_, _ = w.Write(append(append([]byte(nil), body...), '\n'))
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (c *responseCache) get(key, version string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[key]
	if !ok || entry.version != version {
		return nil, false
	}
	return append([]byte(nil), entry.body...), true
}

func (c *responseCache) set(key, version string, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cachedResponse{
		version: version,
		body:    append([]byte(nil), body...),
	}
}
