package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"llmsnare/internal/benchmark"

	"github.com/google/uuid"
)

func TestBackfillRunIDsAddsRunIDToLegacyWAL(t *testing.T) {
	store := New(t.TempDir())
	legacy := benchmark.Result{
		Timestamp:       time.Unix(1, 0).UTC(),
		FinishedAt:      time.Unix(2, 0).UTC(),
		CaseID:          "sample_case",
		Profile:         "demo",
		Provider:        "openai",
		Model:           "gpt-4o",
		Success:         true,
		TotalScore:      10,
		RawScore:        10,
		MaxScore:        10,
		NormalizedScore: 100,
	}
	writeLegacyWAL(t, store, "demo", legacy)

	changed, err := store.BackfillRunIDs()
	if err != nil {
		t.Fatalf("BackfillRunIDs returned error: %v", err)
	}
	if changed != 1 {
		t.Fatalf("changed = %d, want 1", changed)
	}

	loaded, err := store.LoadProfile("demo", 0, TimelineFilter{})
	if err != nil {
		t.Fatalf("LoadProfile returned error: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("len(loaded) = %d, want 1", len(loaded))
	}
	if loaded[0].RunID == "" {
		t.Fatal("run_id = empty, want backfilled ID")
	}
	parsed, err := uuid.Parse(loaded[0].RunID)
	if err != nil {
		t.Fatalf("parse run_id: %v", err)
	}
	if got := parsed.Version(); got != 7 {
		t.Fatalf("run_id version = %d, want 7", got)
	}

	changed, err = store.BackfillRunIDs()
	if err != nil {
		t.Fatalf("second BackfillRunIDs returned error: %v", err)
	}
	if changed != 0 {
		t.Fatalf("second changed = %d, want 0", changed)
	}
}

func TestRebuildProjectionUsesSQLiteAfterReady(t *testing.T) {
	store := New(t.TempDir())
	results := []benchmark.Result{
		{
			RunID:             uuid.Must(uuid.NewV7()).String(),
			Timestamp:         time.Unix(1, 0).UTC(),
			FinishedAt:        time.Unix(2, 0).UTC(),
			CaseID:            "sample_case",
			Profile:           "alpha",
			Provider:          "openai",
			Model:             "gpt-4o",
			ModelVendor:       "openai",
			InferenceProvider: "openai",
			Success:           true,
			TotalScore:        10,
			RawScore:          10,
			MaxScore:          10,
			NormalizedScore:   100,
		},
		{
			RunID:             uuid.Must(uuid.NewV7()).String(),
			Timestamp:         time.Unix(3, 0).UTC(),
			FinishedAt:        time.Unix(4, 0).UTC(),
			CaseID:            "sample_case",
			Profile:           "beta",
			Provider:          "openai",
			Model:             "gpt-4o-mini",
			ModelVendor:       "openai",
			InferenceProvider: "cloudflare",
			Success:           true,
			TotalScore:        9,
			RawScore:          9,
			MaxScore:          10,
			NormalizedScore:   90,
		},
	}
	for i := range results {
		if err := store.Append(&results[i]); err != nil {
			t.Fatalf("Append returned error: %v", err)
		}
	}

	stats, err := store.RebuildProjection()
	if err != nil {
		t.Fatalf("RebuildProjection returned error: %v", err)
	}
	if stats.Rows != 2 {
		t.Fatalf("stats.Rows = %d, want 2", stats.Rows)
	}

	if err := os.Remove(filepath.Join(store.dir, "alpha.jsonl")); err != nil {
		t.Fatalf("remove alpha WAL: %v", err)
	}
	if err := os.Remove(filepath.Join(store.dir, "beta.jsonl")); err != nil {
		t.Fatalf("remove beta WAL: %v", err)
	}

	loaded, err := store.LoadAll(0, TimelineFilter{InferenceProvider: "cloudflare"})
	if err != nil {
		t.Fatalf("LoadAll returned error: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("len(loaded) = %d, want 1", len(loaded))
	}
	beta, ok := loaded["beta"]
	if !ok || len(beta) != 1 {
		t.Fatalf("loaded[beta] = %#v, want single entry", loaded["beta"])
	}
	if got := beta[0].RunID; got != results[1].RunID {
		t.Fatalf("run_id = %q, want %q", got, results[1].RunID)
	}

	filtered, err := store.LoadProfile("beta", 0, TimelineFilter{
		Model:  "gpt-4o-mini",
		CaseID: "sample_case",
	})
	if err != nil {
		t.Fatalf("LoadProfile returned error: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("len(filtered) = %d, want 1", len(filtered))
	}
	if got := filtered[0].RunID; got != results[1].RunID {
		t.Fatalf("filtered run_id = %q, want %q", got, results[1].RunID)
	}
}

func TestReplaceFileOverwritesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")

	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatalf("write dst: %v", err)
	}

	if err := replaceFile(src, dst); err != nil {
		t.Fatalf("replaceFile returned error: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if got := string(data); got != "new" {
		t.Fatalf("dst contents = %q, want %q", got, "new")
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("src still exists, stat err = %v", err)
	}
}

func writeLegacyWAL(t *testing.T, store *Store, profile string, result benchmark.Result) {
	t.Helper()
	if err := store.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir returned error: %v", err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal legacy WAL: %v", err)
	}
	path := filepath.Join(store.dir, profile+".jsonl")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write legacy WAL: %v", err)
	}
}
