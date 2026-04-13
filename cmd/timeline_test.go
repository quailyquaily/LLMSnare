package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"llmsnare/internal/benchmark"
	"llmsnare/internal/storage"

	"github.com/google/uuid"
)

func TestTimelineStatusCommandShowsSQLiteBackend(t *testing.T) {
	oldConfigPath := configPath
	root := t.TempDir()
	configPath = filepath.Join(root, "config.yaml")
	t.Cleanup(func() {
		configPath = oldConfigPath
	})

	timelineDir := filepath.Join(root, "timeline")
	if err := os.WriteFile(configPath, []byte(`version: 1
storage:
  timeline_dir: "`+timelineDir+`"
profiles:
  demo:
    provider: openai
    model: "gpt-4o"
    api_key: "test-key"
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	store := storage.New(timelineDir)
	result := benchmark.Result{
		RunID:             uuid.Must(uuid.NewV7()).String(),
		Timestamp:         time.Unix(1, 0).UTC(),
		FinishedAt:        time.Unix(2, 0).UTC(),
		CaseID:            "sample_case",
		Profile:           "demo",
		Provider:          "openai",
		Model:             "gpt-4o",
		ModelVendor:       "openai",
		InferenceProvider: "openai",
		Success:           true,
		TotalScore:        10,
		RawScore:          10,
		MaxScore:          10,
		NormalizedScore:   100,
	}
	if err := store.Append(&result); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}
	if _, err := store.RebuildProjection(); err != nil {
		t.Fatalf("RebuildProjection returned error: %v", err)
	}

	var out bytes.Buffer
	cmd := newTimelineStatusCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"read_backend: sqlite",
		"wal_profiles: 1",
		"wal_rows: 1",
		"sqlite_exists: true",
		"sqlite_ready: true",
		"sqlite_dirty: false",
		"sqlite_rows: 1",
		"sqlite_schema_version: 2",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status output missing %q:\n%s", want, got)
		}
	}
}
