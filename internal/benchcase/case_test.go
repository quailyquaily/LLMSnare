package benchcase

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadDirReadsRootFS(t *testing.T) {
	scaffold := mustFindScaffold(t, BuiltinCaseRelPath)
	caseDir := writeScaffold(t, scaffold)

	caseDef, err := LoadDir(caseDir)
	if err != nil {
		t.Fatalf("LoadDir returned error: %v", err)
	}
	if caseDef.ID == "" {
		t.Fatal("case id should be populated")
	}
	if got := caseDef.RootFSFiles["main.go"]; got == "" {
		t.Fatal("expected main.go rootfs content")
	}
	if want := DefaultTools(); !reflect.DeepEqual(caseDef.Tools, want) {
		t.Fatalf("tools = %#v, want %#v", caseDef.Tools, want)
	}
}

func TestListIncludesDefaultCase(t *testing.T) {
	scaffold := mustFindScaffold(t, BuiltinCaseRelPath)
	root := t.TempDir()
	writeScaffoldUnderRoot(t, root, scaffold)

	items, warnings, err := List(filepath.Join(root, DefaultCasesRelDir))
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("List warnings = %#v, want none", warnings)
	}
	if len(items) != 1 {
		t.Fatalf("List returned %d items, want 1", len(items))
	}
	if items[0].ID != BuiltinCaseID {
		t.Fatalf("case id = %q, want %q", items[0].ID, BuiltinCaseID)
	}
	if items[0].RootFSFiles == 0 {
		t.Fatal("expected rootfs file count")
	}
}

func TestListIncludesSymlinkedCaseDir(t *testing.T) {
	requireSymlinkSupport(t)

	scaffold := mustFindScaffold(t, BuiltinCaseRelPath)
	realCaseDir := writeScaffold(t, scaffold)

	root := t.TempDir()
	linkDir := filepath.Join(root, DefaultCasesRelDir, "linked_case")
	if err := os.MkdirAll(filepath.Dir(linkDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realCaseDir, linkDir); err != nil {
		t.Fatal(err)
	}

	items, warnings, err := List(filepath.Join(root, DefaultCasesRelDir))
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("List warnings = %#v, want none", warnings)
	}
	if len(items) != 1 {
		t.Fatalf("List returned %d items, want 1", len(items))
	}
	if got := filepath.Base(items[0].Dir); got != "linked_case" {
		t.Fatalf("case dir = %q, want %q", got, "linked_case")
	}
}

func TestListSkipsBrokenCasesAndReturnsWarnings(t *testing.T) {
	scaffold := mustFindScaffold(t, BuiltinCaseRelPath)
	root := t.TempDir()
	writeScaffoldUnderRoot(t, root, scaffold)

	brokenDir := filepath.Join(root, DefaultCasesRelDir, "broken_case")
	if err := os.MkdirAll(filepath.Join(brokenDir, DefaultRootFSRelDir()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(brokenDir, "case.yaml"), []byte(`version: 1
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
	if err := os.WriteFile(filepath.Join(brokenDir, DefaultRootFSRelDir(), "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	items, warnings, err := List(filepath.Join(root, DefaultCasesRelDir))
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("List items = %d, want 1", len(items))
	}
	if len(warnings) != 1 {
		t.Fatalf("List warnings = %d, want 1", len(warnings))
	}
	if filepath.Base(warnings[0].Dir) != "broken_case" {
		t.Fatalf("warning dir = %q, want broken_case", warnings[0].Dir)
	}
	if warnings[0].Message == "" {
		t.Fatal("warning message should not be empty")
	}
}

func TestLoadRejectsLegacyCodeField(t *testing.T) {
	caseDir := t.TempDir()
	content := `version: 1
id: legacy_case
prompt: hi
scoring:
  deductions:
    - code: H1
      points: 1
      description: legacy field
      check:
        type: write_before_any_explore
  bonuses: []
`
	if err := os.WriteFile(filepath.Join(caseDir, "case.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	rootFSDir := filepath.Join(caseDir, DefaultRootFSRelDir())
	if err := os.MkdirAll(rootFSDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootFSDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadDir(caseDir); err == nil {
		t.Fatal("LoadDir succeeded, want error for missing rule name")
	}
}

func TestLoadIgnoresUnknownField(t *testing.T) {
	caseDir := t.TempDir()
	content := `version: 1
id: ignores_unknown_field
prompt: hi
legacy_field: ignored
scoring:
  deductions: []
  bonuses: []
`
	if err := os.WriteFile(filepath.Join(caseDir, "case.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	rootFSDir := filepath.Join(caseDir, DefaultRootFSRelDir())
	if err := os.MkdirAll(rootFSDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootFSDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	caseDef, err := LoadDir(caseDir)
	if err != nil {
		t.Fatalf("LoadDir returned error: %v", err)
	}
	if got := caseDef.RootFSFiles["main.go"]; got != "package main\n" {
		t.Fatalf("rootfs main.go = %q, want %q", got, "package main\n")
	}
	if want := DefaultTools(); !reflect.DeepEqual(caseDef.Tools, want) {
		t.Fatalf("tools = %#v, want %#v", caseDef.Tools, want)
	}
}

func TestLoadUsesConfiguredTools(t *testing.T) {
	caseDir := t.TempDir()
	content := `version: 1
id: uses_search_text
prompt: hi
tools:
  - search_text
  - read_file
  - write_file
scoring:
  deductions: []
  bonuses: []
`
	if err := os.WriteFile(filepath.Join(caseDir, "case.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	rootFSDir := filepath.Join(caseDir, DefaultRootFSRelDir())
	if err := os.MkdirAll(rootFSDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootFSDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	caseDef, err := LoadDir(caseDir)
	if err != nil {
		t.Fatalf("LoadDir returned error: %v", err)
	}
	want := []string{ToolSearchText, ToolReadFile, ToolWriteFile}
	if !reflect.DeepEqual(caseDef.Tools, want) {
		t.Fatalf("tools = %#v, want %#v", caseDef.Tools, want)
	}
}

func TestLoadNormalizesUsedToolCheckFields(t *testing.T) {
	caseDir := t.TempDir()
	content := `version: 1
id: used_tool_check
prompt: hi
scoring:
  deductions: []
  bonuses:
    - name: B1
      points: 5
      description: used search
      check:
        type: used_tool
        tool: " search_text "
        path: "./vendor/"
        query: " FetchDocument "
        before_first_write: true
`
	if err := os.WriteFile(filepath.Join(caseDir, "case.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	rootFSDir := filepath.Join(caseDir, DefaultRootFSRelDir())
	if err := os.MkdirAll(rootFSDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootFSDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	caseDef, err := LoadDir(caseDir)
	if err != nil {
		t.Fatalf("LoadDir returned error: %v", err)
	}
	check := caseDef.Scoring.Bonuses[0].Check
	if got := check.Tool; got != ToolSearchText {
		t.Fatalf("tool = %q, want %q", got, ToolSearchText)
	}
	if got := check.Path; got != "vendor" {
		t.Fatalf("path = %q, want %q", got, "vendor")
	}
	if got := check.Query; got != "FetchDocument" {
		t.Fatalf("query = %q, want %q", got, "FetchDocument")
	}
	if !check.BeforeFirstWrite {
		t.Fatal("before_first_write = false, want true")
	}
}

func TestLoadRejectsUnsupportedTools(t *testing.T) {
	caseDir := t.TempDir()
	content := `version: 1
id: bad_tool
prompt: hi
tools:
  - shell
scoring:
  deductions: []
  bonuses: []
`
	if err := os.WriteFile(filepath.Join(caseDir, "case.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	rootFSDir := filepath.Join(caseDir, DefaultRootFSRelDir())
	if err := os.MkdirAll(rootFSDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootFSDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadDir(caseDir); err == nil {
		t.Fatal("LoadDir succeeded, want error for unsupported tool")
	}
}

func TestLoadRejectsEmptyRootFS(t *testing.T) {
	caseDir := t.TempDir()
	content := `version: 1
id: empty_rootfs
prompt: hi
scoring:
  deductions: []
  bonuses: []
`
	if err := os.WriteFile(filepath.Join(caseDir, "case.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(caseDir, DefaultRootFSRelDir()), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadDir(caseDir); err == nil {
		t.Fatal("LoadDir succeeded, want error for empty rootfs")
	}
}

func TestLoadDirReadsRootFSFromSymlinkedDirectory(t *testing.T) {
	requireSymlinkSupport(t)

	caseDir := t.TempDir()
	content := `version: 1
id: symlink_rootfs
prompt: hi
scoring:
  deductions: []
  bonuses: []
`
	if err := os.WriteFile(filepath.Join(caseDir, "case.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	sharedDir := filepath.Join(t.TempDir(), "shared")
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sharedDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rootFSDir := filepath.Join(caseDir, DefaultRootFSRelDir())
	if err := os.MkdirAll(rootFSDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sharedDir, filepath.Join(rootFSDir, "linked")); err != nil {
		t.Fatal(err)
	}

	caseDef, err := LoadDir(caseDir)
	if err != nil {
		t.Fatalf("LoadDir returned error: %v", err)
	}
	if got := caseDef.RootFSFiles["linked/main.go"]; got != "package main\n" {
		t.Fatalf("rootfs linked/main.go = %q, want %q", got, "package main\n")
	}
}

func TestSummarizePromptUsesFirstNonEmptyLine(t *testing.T) {
	got := summarizePrompt("\n\nfirst line\nsecond line")
	if got != "first line" {
		t.Fatalf("summarizePrompt = %q, want %q", got, "first line")
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

func writeScaffold(t *testing.T, scaffold Scaffold) string {
	t.Helper()

	root := t.TempDir()
	return writeScaffoldUnderRoot(t, root, scaffold)
}

func writeScaffoldUnderRoot(t *testing.T, root string, scaffold Scaffold) string {
	t.Helper()

	caseDir := filepath.Join(root, filepath.Dir(scaffold.CaseRelPath))
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caseDir, "case.yaml"), []byte(scaffold.CaseYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	rootFSDir := filepath.Join(caseDir, DefaultRootFSRelDir())
	for relPath, content := range scaffold.RootFSFiles {
		target := filepath.Join(rootFSDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return caseDir
}

func TestDefaultScaffoldsIncludeExpectedRootFSFiles(t *testing.T) {
	scaffolds, err := DefaultScaffolds()
	if err != nil {
		t.Fatalf("DefaultScaffolds returned error: %v", err)
	}
	for _, scaffold := range scaffolds {
		if scaffold.CaseRelPath != BuiltinCaseRelPath {
			continue
		}
		if want := []string{"docs/format.txt", "main.go"}; !containsKeys(scaffold.RootFSFiles, want) {
			t.Fatalf("default scaffold rootfs files = %#v, want keys %#v", reflect.ValueOf(scaffold.RootFSFiles).MapKeys(), want)
		}
		return
	}
	t.Fatal("default scaffold not found")
}

func TestDefaultScaffoldsIncludeBuiltInCaseSet(t *testing.T) {
	scaffolds, err := DefaultScaffolds()
	if err != nil {
		t.Fatalf("DefaultScaffolds returned error: %v", err)
	}

	want := []string{
		"benchmarks/read_write_ratio_sample/case.yaml",
		"benchmarks/style_required_reads/case.yaml",
	}
	for _, caseRelPath := range want {
		found := false
		for _, scaffold := range scaffolds {
			if scaffold.CaseRelPath == caseRelPath {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("default scaffolds missing %q", caseRelPath)
		}
	}
}

func containsKeys(files map[string]string, want []string) bool {
	for _, key := range want {
		if _, ok := files[key]; !ok {
			return false
		}
	}
	return true
}

func requireSymlinkSupport(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
}
