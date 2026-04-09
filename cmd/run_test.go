package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"llmsnare/internal/benchmark"

	"github.com/spf13/cobra"
)

func TestRenderTextResultUsesReadableSections(t *testing.T) {
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)

	readWriteRatio := 2.5
	coverage := 1.0
	renderTextResult(cmd, benchmark.Result{
		Timestamp:       time.Unix(0, 0).UTC(),
		FinishedAt:      time.Unix(2, 250_000_000).UTC(),
		CaseID:          "read_write_ratio_sample",
		Profile:         "demo_profile",
		Driver:          "openai",
		Model:           "gpt-4o",
		Endpoint:        "https://api.openai.com/v1",
		Success:         true,
		TotalScore:      110,
		RawScore:        110,
		MaxScore:        125,
		NormalizedScore: 88,
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
		"Endpoint",
		"https://api.openai.com/v1",
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
		{Kind: benchmark.ProgressRunStarted, Profile: "demo_profile", CaseID: "case_v1"},
		{Kind: benchmark.ProgressRoundStarted, Profile: "demo_profile", Round: 1},
		{Kind: benchmark.ProgressToolBatch, Profile: "demo_profile", Round: 1, ToolCalls: 2},
		{Kind: benchmark.ProgressToolExecuted, Profile: "demo_profile", Round: 1, Tool: "read_file", ToolPath: "main.go"},
		{Kind: benchmark.ProgressRunFinished, Profile: "demo_profile", Success: true, RawScore: 105, MaxScore: 125, NormalizedScore: 84, Elapsed: 1500 * time.Millisecond},
	}

	for _, event := range events {
		renderRunProgress(&out, 1, 2, event)
	}

	got := out.String()
	for _, want := range []string{
		"[1/2] started, profile=demo_profile, case=case_v1",
		"[1/2] round 001: received 2 tool calls",
		"[1/2] round 001: read_file main.go",
		"[1/2] finished, status=PASS, elapsed=1.5s, score=84.00%",
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
	if reporter := runProgressReporter(cmd, true, 1, 1); reporter != nil {
		t.Fatal("expected no progress reporter in JSON mode")
	}
	if reporter := runProgressReporter(cmd, false, 1, 1); reporter == nil {
		t.Fatal("expected progress reporter in text mode")
	}
}
