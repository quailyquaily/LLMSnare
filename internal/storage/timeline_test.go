package storage

import (
	"testing"
	"time"

	"llmsnare/internal/benchmark"
)

func TestAppendAndLoadProfile(t *testing.T) {
	store := New(t.TempDir())
	result := benchmark.Result{
		Timestamp:       time.Unix(1, 0).UTC(),
		FinishedAt:      time.Unix(2, 0).UTC(),
		Profile:         "demo",
		Driver:          "openai",
		Model:           "gpt-4o",
		Success:         true,
		TotalScore:      110,
		RawScore:        110,
		MaxScore:        125,
		NormalizedScore: 88,
	}

	if err := store.Append(result); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	loaded, err := store.LoadProfile("demo", 0)
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
}
