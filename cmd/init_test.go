package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"llmsnare/internal/benchcase"
	"llmsnare/internal/config"
)

func TestInitCommandSkipsExistingFilesWithoutForce(t *testing.T) {
	oldConfigPath := configPath
	root := t.TempDir()
	configPath = filepath.Join(root, "config.yaml")
	t.Cleanup(func() {
		configPath = oldConfigPath
	})

	scaffold := mustFindBuiltinScaffold(t, benchcase.BuiltinCaseRelPath)
	keys := sortedRootFSKeys(scaffold.RootFSFiles)
	if len(keys) < 2 {
		t.Fatalf("builtin scaffold rootfs files = %d, want at least 2", len(keys))
	}

	const (
		existingConfig = "existing config\n"
		existingCase   = "existing case\n"
		existingRootFS = "existing rootfs\n"
	)
	if err := os.WriteFile(configPath, []byte(existingConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	casePath := filepath.Join(root, filepath.FromSlash(scaffold.CaseRelPath))
	if err := os.MkdirAll(filepath.Dir(casePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(casePath, []byte(existingCase), 0o644); err != nil {
		t.Fatal(err)
	}

	existingRootFSPath := filepath.Join(filepath.Dir(casePath), benchcase.DefaultRootFSRelDir(), filepath.FromSlash(keys[0]))
	if err := os.MkdirAll(filepath.Dir(existingRootFSPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(existingRootFSPath, []byte(existingRootFS), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newInitCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if got := readFileForTest(t, configPath); got != existingConfig {
		t.Fatalf("config content = %q, want %q", got, existingConfig)
	}
	if got := readFileForTest(t, casePath); got != existingCase {
		t.Fatalf("case content = %q, want %q", got, existingCase)
	}
	if got := readFileForTest(t, existingRootFSPath); got != existingRootFS {
		t.Fatalf("rootfs content = %q, want %q", got, existingRootFS)
	}

	missingRootFSPath := filepath.Join(filepath.Dir(casePath), benchcase.DefaultRootFSRelDir(), filepath.FromSlash(keys[1]))
	if got := readFileForTest(t, missingRootFSPath); got != scaffold.RootFSFiles[keys[1]] {
		t.Fatalf("missing rootfs content = %q, want %q", got, scaffold.RootFSFiles[keys[1]])
	}

	output := out.String()
	for _, want := range []string{
		"skipped " + configPath,
		"skipped " + casePath,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestInitCommandForceOverwritesExistingFiles(t *testing.T) {
	oldConfigPath := configPath
	root := t.TempDir()
	configPath = filepath.Join(root, "config.yaml")
	t.Cleanup(func() {
		configPath = oldConfigPath
	})

	scaffold := mustFindBuiltinScaffold(t, benchcase.BuiltinCaseRelPath)
	keys := sortedRootFSKeys(scaffold.RootFSFiles)
	if len(keys) == 0 {
		t.Fatal("builtin scaffold should contain rootfs files")
	}

	if err := os.WriteFile(configPath, []byte("old config\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	casePath := filepath.Join(root, filepath.FromSlash(scaffold.CaseRelPath))
	if err := os.MkdirAll(filepath.Dir(casePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(casePath, []byte("old case\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rootFSPath := filepath.Join(filepath.Dir(casePath), benchcase.DefaultRootFSRelDir(), filepath.FromSlash(keys[0]))
	if err := os.MkdirAll(filepath.Dir(rootFSPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rootFSPath, []byte("old rootfs\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newInitCommand()
	cmd.SetArgs([]string{"--force"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if got := readFileForTest(t, configPath); got != config.TemplateYAML() {
		t.Fatalf("config content = %q, want template", got)
	}
	if got := readFileForTest(t, casePath); got != scaffold.CaseYAML {
		t.Fatalf("case content = %q, want scaffold", got)
	}
	if got := readFileForTest(t, rootFSPath); got != scaffold.RootFSFiles[keys[0]] {
		t.Fatalf("rootfs content = %q, want scaffold", got)
	}

	output := out.String()
	for _, want := range []string{
		"wrote " + configPath,
		"wrote " + casePath,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestWriteDefaultCaseScaffoldsSkipsExistingRootFSFilesWithoutForce(t *testing.T) {
	root := t.TempDir()
	scaffold := benchcase.Scaffold{
		CaseRelPath: "benchmarks/demo/case.yaml",
		CaseYAML: `version: 1
id: demo_case
prompt: hi
scoring:
  deductions: []
  bonuses: []
`,
		RootFSFiles: map[string]string{
			"main.go":       "new main\n",
			"docs/style.md": "new style\n",
		},
	}

	casePath := filepath.Join(root, filepath.FromSlash(scaffold.CaseRelPath))
	if err := os.MkdirAll(filepath.Dir(casePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(casePath, []byte("existing case\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	existingRootFSPath := filepath.Join(filepath.Dir(casePath), benchcase.DefaultRootFSRelDir(), "main.go")
	if err := os.MkdirAll(filepath.Dir(existingRootFSPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(existingRootFSPath, []byte("existing main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	results, err := writeDefaultCaseScaffolds(root, []benchcase.Scaffold{scaffold}, false)
	if err != nil {
		t.Fatalf("writeDefaultCaseScaffolds returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].Path != casePath {
		t.Fatalf("result path = %q, want %q", results[0].Path, casePath)
	}
	if results[0].Written {
		t.Fatal("case.yaml should be skipped when it already exists")
	}

	if got := readFileForTest(t, casePath); got != "existing case\n" {
		t.Fatalf("case content = %q, want existing case", got)
	}
	if got := readFileForTest(t, existingRootFSPath); got != "existing main\n" {
		t.Fatalf("existing rootfs content = %q, want existing main", got)
	}

	missingRootFSPath := filepath.Join(filepath.Dir(casePath), benchcase.DefaultRootFSRelDir(), "docs/style.md")
	if got := readFileForTest(t, missingRootFSPath); got != scaffold.RootFSFiles["docs/style.md"] {
		t.Fatalf("missing rootfs content = %q, want scaffold content", got)
	}
}

func mustFindBuiltinScaffold(t *testing.T, caseRelPath string) benchcase.Scaffold {
	t.Helper()

	scaffolds, err := benchcase.DefaultScaffolds()
	if err != nil {
		t.Fatalf("DefaultScaffolds returned error: %v", err)
	}
	for _, scaffold := range scaffolds {
		if scaffold.CaseRelPath == caseRelPath {
			return scaffold
		}
	}
	t.Fatalf("scaffold %q not found", caseRelPath)
	return benchcase.Scaffold{}
}

func sortedRootFSKeys(files map[string]string) []string {
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func readFileForTest(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
