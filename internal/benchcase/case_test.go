package benchcase

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsFixtureDirectory(t *testing.T) {
	scaffold := mustFindScaffold(t, DefaultCaseRelPath)
	root := t.TempDir()
	casePath := filepath.Join(root, "case.yaml")
	if err := os.WriteFile(casePath, []byte(scaffold.CaseYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	fixtureRoot := filepath.Join(root, DefaultFixtureRelDir())
	for relPath, content := range scaffold.FixtureFiles {
		target := filepath.Join(fixtureRoot, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	caseDef, err := Load(casePath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if caseDef.ID == "" {
		t.Fatal("case id should be populated")
	}
	if got := caseDef.FixtureFiles["main.go"]; got == "" {
		t.Fatal("expected main.go fixture content")
	}
}

func TestDefaultScaffoldsIncludeDefaultCase(t *testing.T) {
	scaffolds, err := DefaultScaffolds()
	if err != nil {
		t.Fatalf("DefaultScaffolds returned error: %v", err)
	}
	found := false
	for _, scaffold := range scaffolds {
		if scaffold.CaseRelPath == DefaultCaseRelPath {
			found = true
			if scaffold.FixtureFiles["docs/format.txt"] == "" {
				t.Fatal("expected default case fixture file")
			}
		}
	}
	if !found {
		t.Fatal("default scaffold not found")
	}
}

func TestLoadWithFixtureDirOverride(t *testing.T) {
	scaffold := mustFindScaffold(t, DefaultCaseRelPath)
	root := t.TempDir()
	casePath := filepath.Join(root, "case.yaml")
	if err := os.WriteFile(casePath, []byte(scaffold.CaseYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	overrideRoot := filepath.Join(root, "override-fixture")
	target := filepath.Join(overrideRoot, "main.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	caseDef, err := LoadWithFixtureDir(casePath, overrideRoot)
	if err != nil {
		t.Fatalf("LoadWithFixtureDir returned error: %v", err)
	}
	if got := caseDef.FixtureFiles["main.go"]; got != "package main\n" {
		t.Fatalf("fixture override not applied, got %q", got)
	}
}

func TestLoadRejectsLegacyCodeField(t *testing.T) {
	root := t.TempDir()
	casePath := filepath.Join(root, "case.yaml")
	content := `version: 1
id: legacy_case
prompt: hi
fixture_dir: fixture
scoring:
  deductions:
    - code: H1
      points: 1
      description: legacy field
      check:
        type: write_before_any_explore
  bonuses: []
metrics: {}
`
	if err := os.WriteFile(casePath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	fixtureRoot := filepath.Join(root, DefaultFixtureRelDir())
	if err := os.MkdirAll(fixtureRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureRoot, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(casePath); err == nil {
		t.Fatal("Load succeeded, want error for missing rule name")
	}
}

func mustFindScaffold(t *testing.T, caseRelPath string) Scaffold {
	t.Helper()

	scaffolds, err := DefaultScaffolds()
	if err != nil {
		t.Fatalf("DefaultScaffolds returned error: %v", err)
	}
	for _, scaffold := range scaffolds {
		if scaffold.CaseRelPath == caseRelPath {
			return scaffold
		}
	}
	t.Fatalf("scaffold %q not found", caseRelPath)
	return Scaffold{}
}
