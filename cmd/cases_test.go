package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"llmsnare/internal/benchcase"
)

func TestResolveCaseDirUsesDefaultCasesRoot(t *testing.T) {
	oldConfigPath := configPath
	root := t.TempDir()
	configPath = filepath.Join(root, "config.yaml")
	t.Cleanup(func() {
		configPath = oldConfigPath
	})

	caseDir := filepath.Join(root, benchcase.DefaultCasesRelDir, benchcase.BuiltinCaseID)
	if err := os.MkdirAll(filepath.Join(caseDir, benchcase.DefaultRootFSRelDir()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caseDir, "case.yaml"), []byte(`version: 1
id: demo_case
prompt: hi
scoring:
  deductions: []
  bonuses: []
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caseDir, benchcase.DefaultRootFSRelDir(), "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveCaseDir("demo_case")
	if err != nil {
		t.Fatalf("resolveCaseDir returned error: %v", err)
	}
	want := filepath.Join(filepath.Dir(configPath), benchcase.DefaultCasesRelDir, "demo_case")
	if got != want {
		t.Fatalf("resolveCaseDir = %q, want %q", got, want)
	}
}

func TestResolveCaseDirWithoutCaseRejectsWhenNoCasesExist(t *testing.T) {
	oldConfigPath := configPath
	configPath = filepath.Join(t.TempDir(), "config.yaml")
	t.Cleanup(func() {
		configPath = oldConfigPath
	})

	_, err := resolveCaseDir("")
	if err == nil || !strings.Contains(err.Error(), "run `llmsnare init`") {
		t.Fatalf("resolveCaseDir error = %v, want init hint", err)
	}
}

func TestResolveCaseDirWithoutCaseRejectsWhenCasesExist(t *testing.T) {
	oldConfigPath := configPath
	root := t.TempDir()
	configPath = filepath.Join(root, "config.yaml")
	t.Cleanup(func() {
		configPath = oldConfigPath
	})

	caseDir := filepath.Join(root, benchcase.DefaultCasesRelDir, "demo_case")
	if err := os.MkdirAll(filepath.Join(caseDir, benchcase.DefaultRootFSRelDir()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caseDir, "case.yaml"), []byte(`version: 1
id: demo_case
prompt: hi
scoring:
  deductions: []
  bonuses: []
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caseDir, benchcase.DefaultRootFSRelDir(), "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := resolveCaseDir("")
	if err == nil || !strings.Contains(err.Error(), "run `llmsnare cases`") {
		t.Fatalf("resolveCaseDir error = %v, want cases hint", err)
	}
}

func TestResolveCaseDirWithoutCaseRejectsWhenOnlyBrokenCasesExist(t *testing.T) {
	oldConfigPath := configPath
	root := t.TempDir()
	configPath = filepath.Join(root, "config.yaml")
	t.Cleanup(func() {
		configPath = oldConfigPath
	})

	caseDir := filepath.Join(root, benchcase.DefaultCasesRelDir, "broken_case")
	if err := os.MkdirAll(filepath.Join(caseDir, benchcase.DefaultRootFSRelDir()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caseDir, "case.yaml"), []byte(`version: 1
id: broken_case
prompt: hi
scoring:
  deductions:
    - points: 1
      description: broken
      check:
        type: write_before_any_explore
  bonuses: []
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caseDir, benchcase.DefaultRootFSRelDir(), "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := resolveCaseDir("")
	if err == nil || !strings.Contains(err.Error(), "run `llmsnare cases`") {
		t.Fatalf("resolveCaseDir error = %v, want cases hint", err)
	}
}

func TestResolveCaseDirRejectsCaseYAMLPath(t *testing.T) {
	oldConfigPath := configPath
	configPath = filepath.Join(t.TempDir(), "config.yaml")
	t.Cleanup(func() {
		configPath = oldConfigPath
	})

	_, err := resolveCaseDir("./benchmarks/demo/case.yaml")
	if err == nil {
		t.Fatal("resolveCaseDir succeeded, want error")
	}
}

func TestRenderCaseListShowsCoreFields(t *testing.T) {
	var out bytes.Buffer
	renderCaseList(&out, []benchcase.Summary{
		{
			ID:            benchcase.BuiltinCaseID,
			Dir:           filepath.Join("benchmarks", benchcase.BuiltinCaseID),
			PromptSummary: "BuildStatus in main.go",
			WritablePaths: 1,
			RootFSFiles:   2,
		},
	}, "benchmarks")

	got := out.String()
	for _, want := range []string{
		"Available Cases",
		"- " + benchcase.BuiltinCaseID,
		"dir: read_write_ratio_sample",
		"rootfs files: 2",
		"writable paths: 1",
		"prompt: BuildStatus in main.go",
		benchcase.BuiltinCaseID,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("case list missing %q:\n%s", want, got)
		}
	}
}

func TestRenderCaseWarningsShowsWarningLines(t *testing.T) {
	var out bytes.Buffer
	renderCaseWarnings(&out, []benchcase.ListWarning{
		{
			Dir:     filepath.Join("benchmarks", "broken_case"),
			Message: "every scoring rule must set name",
		},
	}, "benchmarks")

	got := out.String()
	for _, want := range []string{
		"warning:",
		"broken_case",
		"every scoring rule must set name",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("warning output missing %q:\n%s", want, got)
		}
	}
}
