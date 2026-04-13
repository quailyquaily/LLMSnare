package cmd

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"llmsnare/internal/benchmark"
	"llmsnare/internal/storage"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func TestRenderTextResultUsesReadableSections(t *testing.T) {
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)

	readWriteRatio := 2.5
	coverage := 1.0
	renderTextResult(cmd, benchmark.Result{
		Timestamp:         time.Unix(0, 0).UTC(),
		FinishedAt:        time.Unix(2, 250_000_000).UTC(),
		CaseID:            "read_write_ratio_sample",
		Profile:           "demo_profile",
		Provider:          "openai",
		Model:             "gpt-4o",
		ModelVendor:       "openai",
		InferenceProvider: "groq",
		Endpoint:          "https://api.openai.com/v1",
		Success:           true,
		TotalScore:        110,
		RawScore:          110,
		MaxScore:          125,
		NormalizedScore:   88,
		Metrics: benchmark.Metrics{
			ReadFileCalls:        2,
			WriteFileCalls:       1,
			ListDirCalls:         1,
			ReadWriteRatio:       &readWriteRatio,
			PreWriteReadCoverage: &coverage,
		},
		Deductions: []benchmark.ScoreAdjustment{
			{Name: "S1", Points: -5, Description: "duplicate read"},
		},
		Bonuses: []benchmark.ScoreAdjustment{
			{Name: "B1", Points: 10, Description: "recovered vendor"},
		},
		ToolCalls: []benchmark.ToolCallLog{
			{Tool: "read_file"},
			{Tool: "write_file"},
		},
	})

	got := out.String()
	for _, want := range []string{
		"Profile: demo_profile",
		"Summary",
		"Metrics",
		"Deductions",
		"Bonuses",
		"Tool calls",
		"Provider",
		"Model vendor",
		"Inference provider",
		"Endpoint",
		"https://api.openai.com/v1",
		"openai",
		"groq",
		"88.00%",
		"110/125",
		"2.25s",
		"read/write ratio",
		"S1",
		"-5",
		"B1",
		"+10",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestRenderRunProgressShowsRoundAndCompletion(t *testing.T) {
	var out bytes.Buffer
	events := []benchmark.ProgressEvent{
		{Kind: benchmark.ProgressRunStarted, Profile: "demo_profile", ModelVendor: "google", InferenceProvider: "groq", CaseID: "case_v1"},
		{Kind: benchmark.ProgressRoundStarted, Profile: "demo_profile", ModelVendor: "google", InferenceProvider: "groq", Round: 1},
		{Kind: benchmark.ProgressToolBatch, Profile: "demo_profile", ModelVendor: "google", InferenceProvider: "groq", Round: 1, ToolCalls: 2},
		{Kind: benchmark.ProgressToolExecuted, Profile: "demo_profile", ModelVendor: "google", InferenceProvider: "groq", Round: 1, Tool: "read_file", ToolPath: "main.go"},
		{Kind: benchmark.ProgressRunFinished, Profile: "demo_profile", ModelVendor: "google", InferenceProvider: "groq", Success: true, RawScore: 105, MaxScore: 125, NormalizedScore: 84, Elapsed: 1500 * time.Millisecond},
	}

	for _, event := range events {
		renderRunProgress(&out, 1, 2, event)
	}

	got := out.String()
	for _, want := range []string{
		`[1/2] started, profile="demo_profile", model_vendor="google", inference_provider="groq", case=case_v1`,
		`[1/2] round 001, profile="demo_profile", model_vendor="google", inference_provider="groq": received 2 tool calls`,
		`[1/2] round 001, profile="demo_profile", model_vendor="google", inference_provider="groq": read_file main.go`,
		`[1/2] finished, profile="demo_profile", model_vendor="google", inference_provider="groq", status=PASS, elapsed=1.5s, score=84.00%`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("progress output missing %q:\n%s", want, got)
		}
	}
}

func TestFormatRoundPadsToThreeDigits(t *testing.T) {
	if got := formatRound(1); got != "001" {
		t.Fatalf("formatRound(1) = %q, want %q", got, "001")
	}
	if got := formatRound(24); got != "024" {
		t.Fatalf("formatRound(24) = %q, want %q", got, "024")
	}
}

func TestFormatRawScoreShowsMaxWhenPresent(t *testing.T) {
	if got := formatRawScore(110, 125); got != "110/125" {
		t.Fatalf("formatRawScore(110, 125) = %q, want %q", got, "110/125")
	}
	if got := formatRawScore(0, 0); got != "0" {
		t.Fatalf("formatRawScore(0, 0) = %q, want %q", got, "0")
	}
}

func TestRunProgressReporterIsDisabledForJSON(t *testing.T) {
	cmd := &cobra.Command{}
	if reporter := runProgressReporter(cmd, true, 1, 1, nil); reporter != nil {
		t.Fatal("expected no progress reporter in JSON mode")
	}
	if reporter := runProgressReporter(cmd, false, 1, 1, nil); reporter == nil {
		t.Fatal("expected progress reporter in text mode")
	}
}

func TestProfileGroupUsesLeadingToken(t *testing.T) {
	cases := map[string]string{
		"openai_gpt4o":       "openai",
		"gemini-main":        "gemini",
		"claude.sonnet":      "claude",
		"singleprofile":      "singleprofile",
		"_leading_delimiter": "_leading_delimiter",
	}
	for input, want := range cases {
		if got := profileGroup(input); got != want {
			t.Fatalf("profileGroup(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestExecuteProfilesRejectsInvalidParallel(t *testing.T) {
	_, err := executeProfiles(context.Background(), []namedProfile{{Name: "openai_one"}}, 0, func(ctx context.Context, profileIndex, totalProfiles int, namedProfile namedProfile) (benchmark.Result, error) {
		return benchmark.Result{}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "parallel must be at least 1") {
		t.Fatalf("executeProfiles error = %v, want parallel validation error", err)
	}
}

func TestExecuteProfilesAvoidsConcurrentSamePrefix(t *testing.T) {
	profiles := []namedProfile{
		{Name: "alpha_one"},
		{Name: "alpha_two"},
		{Name: "beta_one"},
	}
	release := map[string]chan struct{}{
		"alpha_one": make(chan struct{}),
		"alpha_two": make(chan struct{}),
		"beta_one":  make(chan struct{}),
	}
	started := make(chan string, len(profiles))

	var mu sync.Mutex
	activeGroups := make(map[string]int)
	overlap := false

	type runResult struct {
		results []benchmark.Result
		err     error
	}
	done := make(chan runResult, 1)
	go func() {
		results, err := executeProfiles(context.Background(), profiles, 2, func(ctx context.Context, profileIndex, totalProfiles int, namedProfile namedProfile) (benchmark.Result, error) {
			group := profileGroup(namedProfile.Name)

			mu.Lock()
			activeGroups[group]++
			if activeGroups[group] > 1 {
				overlap = true
			}
			mu.Unlock()

			started <- namedProfile.Name
			<-release[namedProfile.Name]

			mu.Lock()
			activeGroups[group]--
			mu.Unlock()

			return benchmark.Result{Profile: namedProfile.Name, RawScore: profileIndex}, nil
		})
		done <- runResult{results: results, err: err}
	}()

	first := <-started
	second := <-started
	firstTwo := map[string]bool{first: true, second: true}
	if !firstTwo["alpha_one"] || !firstTwo["beta_one"] || firstTwo["alpha_two"] {
		t.Fatalf("first started profiles = %q, %q; want alpha_one and beta_one first", first, second)
	}
	if overlap {
		t.Fatal("same prefix profiles overlapped before any release")
	}

	close(release["alpha_one"])

	third := <-started
	if third != "alpha_two" {
		t.Fatalf("third started profile = %q, want alpha_two", third)
	}
	if overlap {
		t.Fatal("same prefix profiles overlapped after alpha_two started")
	}

	close(release["beta_one"])
	close(release["alpha_two"])

	run := <-done
	if run.err != nil {
		t.Fatalf("executeProfiles returned error: %v", run.err)
	}
	if overlap {
		t.Fatal("same prefix profiles overlapped")
	}
	if len(run.results) != len(profiles) {
		t.Fatalf("len(results) = %d, want %d", len(run.results), len(profiles))
	}
	for i, namedProfile := range profiles {
		if got := run.results[i].Profile; got != namedProfile.Name {
			t.Fatalf("results[%d].Profile = %q, want %q", i, got, namedProfile.Name)
		}
	}
}

func TestPersistResultsAppendsTimelineEntries(t *testing.T) {
	dir := t.TempDir()
	results := []benchmark.Result{
		{
			Timestamp:         time.Unix(1, 0).UTC(),
			FinishedAt:        time.Unix(2, 0).UTC(),
			Profile:           "demo",
			Provider:          "openai",
			Model:             "gpt-4o",
			ModelVendor:       "openai",
			InferenceProvider: "cloudflare",
			Endpoint:          "https://api.openai.com/v1",
			Success:           true,
			RawScore:          90,
			MaxScore:          100,
			NormalizedScore:   90,
		},
	}

	if err := persistResults(dir, results); err != nil {
		t.Fatalf("persistResults returned error: %v", err)
	}

	loaded, err := storage.New(dir).LoadProfile("demo", 0, storage.TimelineFilter{})
	if err != nil {
		t.Fatalf("LoadProfile returned error: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("len(loaded) = %d, want 1", len(loaded))
	}
	if got := loaded[0].NormalizedScore; got != 90 {
		t.Fatalf("normalized score = %v, want 90", got)
	}
	if got := loaded[0].ModelVendor; got != "openai" {
		t.Fatalf("model_vendor = %q, want %q", got, "openai")
	}
	if got := loaded[0].InferenceProvider; got != "cloudflare" {
		t.Fatalf("inference_provider = %q, want %q", got, "cloudflare")
	}
	if results[0].RunID == "" {
		t.Fatal("results[0].run_id = empty, want generated ID")
	}
	parsedID, err := uuid.Parse(results[0].RunID)
	if err != nil {
		t.Fatalf("parse run_id: %v", err)
	}
	if got := parsedID.Version(); got != 7 {
		t.Fatalf("run_id version = %d, want 7", got)
	}
	if got := loaded[0].RunID; got != results[0].RunID {
		t.Fatalf("loaded run_id = %q, want %q", got, results[0].RunID)
	}
}
