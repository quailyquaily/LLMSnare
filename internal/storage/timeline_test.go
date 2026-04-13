package storage

import (
	"testing"
	"time"

	"llmsnare/internal/benchmark"

	"github.com/google/uuid"
)

func TestAppendAndLoadProfile(t *testing.T) {
	store := New(t.TempDir())
	result := benchmark.Result{
		Timestamp:         time.Unix(1, 0).UTC(),
		FinishedAt:        time.Unix(2, 0).UTC(),
		Profile:           "demo",
		Provider:          "openai",
		Model:             "gpt-4o",
		ModelVendor:       "openai",
		InferenceProvider: "cloudflare",
		Success:           true,
		TotalScore:        110,
		RawScore:          110,
		MaxScore:          125,
		NormalizedScore:   88,
	}

	if err := store.Append(&result); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}
	if result.RunID == "" {
		t.Fatal("run_id = empty, want generated ID")
	}
	generatedID, err := uuid.Parse(result.RunID)
	if err != nil {
		t.Fatalf("parse generated run_id: %v", err)
	}
	if got := generatedID.Version(); got != 7 {
		t.Fatalf("run_id version = %d, want 7", got)
	}

	loaded, err := store.LoadProfile("demo", 0, TimelineFilter{})
	if err != nil {
		t.Fatalf("LoadProfile returned error: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("len(loaded) = %d, want 1", len(loaded))
	}
	if loaded[0].TotalScore != 110 {
		t.Fatalf("loaded score = %d, want 110", loaded[0].TotalScore)
	}
	if loaded[0].NormalizedScore != 88 {
		t.Fatalf("loaded normalized score = %v, want 88", loaded[0].NormalizedScore)
	}
	if loaded[0].ModelVendor != "openai" {
		t.Fatalf("loaded model_vendor = %q, want %q", loaded[0].ModelVendor, "openai")
	}
	if loaded[0].InferenceProvider != "cloudflare" {
		t.Fatalf("loaded inference_provider = %q, want %q", loaded[0].InferenceProvider, "cloudflare")
	}
	if loaded[0].RunID != result.RunID {
		t.Fatalf("loaded run_id = %q, want %q", loaded[0].RunID, result.RunID)
	}
}
