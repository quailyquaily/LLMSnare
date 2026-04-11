package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"llmsnare/internal/benchmark"
	"llmsnare/internal/storage"
)

func TestTimelineProfileOmitsHeavyFields(t *testing.T) {
	store := storage.New(t.TempDir())
	result := benchmark.Result{
		Timestamp:       time.Unix(1, 0).UTC(),
		FinishedAt:      time.Unix(2, 0).UTC(),
		CaseID:          "sample_case",
		Profile:         "demo",
		Provider:        "openai",
		Model:           "gpt-4o",
		Endpoint:        "https://api.openai.com/v1",
		Success:         false,
		Error:           `gemini provider model "gemini-3.1-pro-preview": upstream detail`,
		TotalScore:      90,
		RawScore:        90,
		MaxScore:        100,
		NormalizedScore: 90,
		Deductions: []benchmark.ScoreAdjustment{
			{Name: "S1", Points: -10, Description: "missing read"},
		},
		Bonuses: []benchmark.ScoreAdjustment{
			{Name: "B1", Points: 10, Description: "looks correct"},
		},
		ToolCalls: []benchmark.ToolCallLog{
			{
				Sequence:  1,
				Timestamp: time.Unix(1, 0).UTC(),
				Tool:      "read_file",
				Input:     map[string]any{"path": "main.go"},
				Result:    "package main",
				IsError:   false,
			},
		},
		FinalWrites:   map[string]string{"main.go": "package main"},
		FinalResponse: "done",
	}
	if err := store.Append(result); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/timelines/demo", nil)
	rec := httptest.NewRecorder()
	NewServer(store).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	entries, ok := payload["entries"].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("entries = %#v, want single entry", payload["entries"])
	}

	entry, ok := entries[0].(map[string]any)
	if !ok {
		t.Fatalf("entry = %#v, want object", entries[0])
	}
	for _, forbidden := range []string{"endpoint", "error", "final_writes", "final_response"} {
		if _, ok := entry[forbidden]; ok {
			t.Fatalf("entry unexpectedly contains %q", forbidden)
		}
	}

	bonuses, ok := entry["bonuses"].([]any)
	if !ok || len(bonuses) != 1 {
		t.Fatalf("bonuses = %#v, want single bonus", entry["bonuses"])
	}
	bonus, ok := bonuses[0].(map[string]any)
	if !ok {
		t.Fatalf("bonus = %#v, want object", bonuses[0])
	}
	if got := bonus["description"]; got != "looks correct" {
		t.Fatalf("bonus description = %#v, want %q", got, "looks correct")
	}

	if _, ok := entry["tool_calls"]; ok {
		t.Fatal("entry unexpectedly contains tool_calls")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
}

func TestRoutesHandlesCORSPreflight(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/v1/timelines/demo", nil)
	rec := httptest.NewRecorder()

	NewServer(storage.New(t.TempDir())).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, OPTIONS" {
		t.Fatalf("Access-Control-Allow-Methods = %q, want %q", got, "GET, OPTIONS")
	}
}

func TestTimelineProfileDefaultsToMax1024Entries(t *testing.T) {
	store := storage.New(t.TempDir())
	for i := 0; i < maxTimelineEntries+1; i++ {
		result := benchmark.Result{
			Timestamp:       time.Unix(int64(i+1), 0).UTC(),
			FinishedAt:      time.Unix(int64(i+1), 0).UTC(),
			CaseID:          "sample_case",
			Profile:         "demo",
			Provider:        "openai",
			Model:           "gpt-4o",
			Success:         true,
			TotalScore:      i,
			RawScore:        i,
			MaxScore:        maxTimelineEntries + 1,
			NormalizedScore: float64(i),
		}
		if err := store.Append(result); err != nil {
			t.Fatalf("Append returned error: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/timelines/demo", nil)
	rec := httptest.NewRecorder()
	NewServer(store).routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	entries, ok := payload["entries"].([]any)
	if !ok {
		t.Fatalf("entries = %#v, want array", payload["entries"])
	}
	if len(entries) != maxTimelineEntries {
		t.Fatalf("entries len = %d, want %d", len(entries), maxTimelineEntries)
	}
	first, ok := entries[0].(map[string]any)
	if !ok {
		t.Fatalf("first entry = %#v, want object", entries[0])
	}
	if got := first["raw_score"]; got != float64(1) {
		t.Fatalf("first raw_score = %#v, want %v", got, float64(1))
	}
}

func TestTimelineProfileCapsExplicitLimitAt1024(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/timelines/demo?limit=5000", nil)
	limit, err := parseLimit(req)
	if err != nil {
		t.Fatalf("parseLimit returned error: %v", err)
	}
	if limit != maxTimelineEntries {
		t.Fatalf("limit = %d, want %d", limit, maxTimelineEntries)
	}
}

func TestParseLimitRejectsNegativeValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/timelines/demo?limit=-"+strconv.Itoa(1), nil)
	if _, err := parseLimit(req); err == nil {
		t.Fatal("expected parseLimit to reject negative limit")
	}
}
