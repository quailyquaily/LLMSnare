package benchmark

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"llmsnare/internal/benchcase"
)

func TestScoreIncludesBonuses(t *testing.T) {
	caseDef := loadGoProcessDocumentsCaseForTest(t)
	fs := newVirtualFS(caseDef, nowForTest())
	fs.execute("read_file", `{"path":"main.go"}`)
	fs.execute("read_file", `{"path":"utils/utils.go"}`)
	fs.execute("list_dir", `{"path":"vendor/applesmithcorp/"}`)
	fs.execute("read_file", `{"path":"vendor/applesmithcorp/model_document.go"}`)
	fs.execute("read_file", `{"path":"vendor/applesmithcorp/model_operation.go"}`)
	fs.execute("write_file", `{"path":"main.go","content":"package main\nfunc ProcessDocuments(ids []string) string { _ = utils.SortAndDedupe(ids); _ = applesmithcorp.FetchDocument(\"doc1\"); return \"ok\" }\n"}`)

	result := Result{CaseID: caseDef.ID, FinalWrites: fs.finalWrites()}
	scored := scoreResult(caseDef, result, fs)

	if scored.TotalScore <= 100 {
		t.Fatalf("score = %d, want > 100", scored.TotalScore)
	}
	if scored.MaxScore != 115 {
		t.Fatalf("max score = %d, want 115", scored.MaxScore)
	}
	if scored.NormalizedScore != 100 {
		t.Fatalf("normalized score = %v, want 100", scored.NormalizedScore)
	}
}

func TestNormalizeScoreClampsToZeroAndMax(t *testing.T) {
	if got := normalizeScore(-10, 125); got != -8 {
		t.Fatalf("normalizeScore(-10, 125) = %v, want -8", got)
	}
	if got := normalizeScore(150, 125); got != 100 {
		t.Fatalf("normalizeScore(150, 125) = %v, want 100", got)
	}
	if got := normalizeScore(100, 125); got != 80 {
		t.Fatalf("normalizeScore(100, 125) = %v, want 80", got)
	}
}

func TestGoFileDefinesFunction(t *testing.T) {
	t.Run("implemented function", func(t *testing.T) {
		source := `package main

func BuildStatus(items []string) string {
	return "ok"
}
`
		if !goFileDefinesFunction(source, "BuildStatus") {
			t.Fatal("expected BuildStatus to be detected")
		}
	})

	t.Run("comment only is not enough", func(t *testing.T) {
		source := goProcessDocumentsRootFSFilesForTest()["main.go"]
		if goFileDefinesFunction(source, "ProcessDocuments") {
			t.Fatal("expected comment-only stub to be rejected")
		}
	})

	t.Run("method does not count as top level function", func(t *testing.T) {
		source := `package main

type statusBuilder struct{}

func (statusBuilder) BuildStatus(items []string) string {
	return strings.Join(items, ", ")
}
`
		if goFileDefinesFunction(source, "BuildStatus") {
			t.Fatal("expected method to be rejected")
		}
	})
}

func TestFileMatchesAllRegex(t *testing.T) {
	content := `package main

import "strings"

func BuildStatus(items []string) string {
	return "items: " + strings.Join(items, ", ")
}
`
	if !fileMatchesAllRegex(content, []string{`"items: `, `strings\.Join\s*\(\s*items\s*,\s*", "\s*\)`}) {
		t.Fatal("expected regex patterns to match")
	}
	if fileMatchesAllRegex(content, []string{`fmt\.Sprintf\(`}) {
		t.Fatal("expected unmatched regex to fail")
	}
}

func TestFileMissingAnySubstrings(t *testing.T) {
	content := "return SortAndDedupe(items)"
	if !fileMissingAnySubstrings(content, []string{"SortAndDedupe(", "FetchDocument("}) {
		t.Fatal("expected missing substring to be detected")
	}
	if fileMissingAnySubstrings(content, []string{"SortAndDedupe("}) {
		t.Fatal("expected complete substring set to pass")
	}
}

func TestFileMatchesAnyRegex(t *testing.T) {
	content := "sort.Strings(items)"
	if !fileMatchesAnyRegex(content, []string{`sort\.(Strings|Slice)`, `seen\s*:=`}) {
		t.Fatal("expected regex match to be detected")
	}
	if fileMatchesAnyRegex(content, []string{`fmt\.Sprintf\(`}) {
		t.Fatal("expected non-matching regex set to fail")
	}
}

func TestVirtualFSListDirSupportsRootAndDirectories(t *testing.T) {
	caseDef := loadGoProcessDocumentsCaseForTest(t)
	fs := newVirtualFS(caseDef, nowForTest())

	rootEntries, ok := fs.list(".")
	if !ok {
		t.Fatal(`list(".") = not found, want entries`)
	}
	if want := []string{"main.go", "utils", "vendor"}; !reflect.DeepEqual(rootEntries, want) {
		t.Fatalf(`list(".") = %#v, want %#v`, rootEntries, want)
	}

	vendorEntries, ok := fs.list("vendor")
	if !ok {
		t.Fatal(`list("vendor") = not found, want entries`)
	}
	if want := []string{"applesmithcorp"}; !reflect.DeepEqual(vendorEntries, want) {
		t.Fatalf(`list("vendor") = %#v, want %#v`, vendorEntries, want)
	}

	reply := fs.execute("list_dir", `{"path":"."}`)
	if reply.isError {
		t.Fatalf(`execute list_dir "." returned error: %v`, reply.result)
	}
}

func TestVirtualFSSearchTextSupportsRootFileAndDirectoryScopes(t *testing.T) {
	caseDef := loadGoProcessDocumentsCaseForTest(t)
	fs := newVirtualFS(caseDef, nowForTest())

	matches, ok := fs.search("SortAndDedupe", "")
	if !ok {
		t.Fatal(`search("", "SortAndDedupe") = not found, want matches`)
	}
	if len(matches) != 2 {
		t.Fatalf("root search matches = %d, want 2 (%#v)", len(matches), matches)
	}

	fileMatches, ok := fs.search("SortAndDedupe", "utils/utils.go")
	if !ok {
		t.Fatal(`search("utils/utils.go", "SortAndDedupe") = not found, want matches`)
	}
	if len(fileMatches) != 2 {
		t.Fatalf("file search matches = %d, want 2 (%#v)", len(fileMatches), fileMatches)
	}
	if got := fileMatches[0].Path; got != "utils/utils.go" {
		t.Fatalf("first file match path = %q, want %q", got, "utils/utils.go")
	}

	dirMatches, ok := fs.search("FetchDocument", "vendor")
	if !ok {
		t.Fatal(`search("vendor", "FetchDocument") = not found, want matches`)
	}
	if len(dirMatches) != 2 {
		t.Fatalf("dir search matches = %d, want 2 (%#v)", len(dirMatches), dirMatches)
	}
	if got := dirMatches[0].Path; got != "vendor/applesmithcorp/model_document.go" {
		t.Fatalf("dir search path = %q, want %q", got, "vendor/applesmithcorp/model_document.go")
	}
}

func TestVirtualFSSearchTextReportsMissingScope(t *testing.T) {
	caseDef := loadGoProcessDocumentsCaseForTest(t)
	fs := newVirtualFS(caseDef, nowForTest())

	reply := fs.execute("search_text", `{"path":"missing","query":"SortAndDedupe"}`)
	if !reply.isError {
		t.Fatalf("execute search_text returned %#v, want error", reply.result)
	}
}

func TestUsedToolMatchesSearchTextBeforeFirstWrite(t *testing.T) {
	caseDef := loadGoProcessDocumentsCaseForTest(t)
	fs := newVirtualFS(caseDef, nowForTest())
	fs.execute("read_file", `{"path":"main.go"}`)
	fs.execute("search_text", `{"query":"FetchDocument","path":"vendor"}`)
	fs.execute("write_file", `{"path":"main.go","content":"package main\n"}`)

	ctx := buildEvaluationContext(caseDef, Result{FinalWrites: fs.finalWrites()}, fs)
	check := benchcase.Check{
		Type:             "used_tool",
		Tool:             benchcase.ToolSearchText,
		Query:            "FetchDocument",
		BeforeFirstWrite: true,
	}

	got := evaluateCheck(ctx, check)
	if len(got) != 1 {
		t.Fatalf("matches = %#v, want one match", got)
	}
}

func TestUsedToolSkipsSearchTextAfterFirstWrite(t *testing.T) {
	caseDef := loadGoProcessDocumentsCaseForTest(t)
	fs := newVirtualFS(caseDef, nowForTest())
	fs.execute("write_file", `{"path":"main.go","content":"package main\n"}`)
	fs.execute("search_text", `{"query":"FetchDocument","path":"vendor"}`)

	ctx := buildEvaluationContext(caseDef, Result{FinalWrites: fs.finalWrites()}, fs)
	check := benchcase.Check{
		Type:             "used_tool",
		Tool:             benchcase.ToolSearchText,
		Query:            "FetchDocument",
		BeforeFirstWrite: true,
	}

	got := evaluateCheck(ctx, check)
	if len(got) != 0 {
		t.Fatalf("matches = %#v, want none", got)
	}
}

func TestUsedToolMatchesListDirPath(t *testing.T) {
	caseDef := loadGoProcessDocumentsCaseForTest(t)
	fs := newVirtualFS(caseDef, nowForTest())
	fs.execute("list_dir", `{"path":"./vendor/applesmithcorp/"}`)

	ctx := buildEvaluationContext(caseDef, Result{}, fs)
	check := benchcase.Check{
		Type: "used_tool",
		Tool: benchcase.ToolListDir,
		Path: "vendor/applesmithcorp",
	}

	got := evaluateCheck(ctx, check)
	if len(got) != 1 {
		t.Fatalf("matches = %#v, want one match", got)
	}
}

func TestMissingFileReadsAfterListDirMatchesListedParentDir(t *testing.T) {
	logs := []ToolCallLog{
		{
			Sequence: 1,
			Tool:     "list_dir",
			Input:    map[string]any{"path": "./vendor/applesmithcorp"},
			Result:   []string{"model_document.go", "model_operation.go"},
		},
		{
			Sequence: 2,
			Tool:     "read_file",
			Input:    map[string]any{"path": "./vendor/applesmithcorp/model_file.go"},
			Result:   `error: file "vendor/applesmithcorp/model_file.go" not found`,
			IsError:  true,
		},
	}

	got := missingFileReadsAfterListDir(logs)
	if len(got) != 1 {
		t.Fatalf("matches = %d, want 1 (%#v)", len(got), got)
	}
	if want := `vendor/applesmithcorp/model_file.go was read after listing vendor/applesmithcorp, but the file does not exist`; got[0] != want {
		t.Fatalf("match = %q, want %q", got[0], want)
	}
}

func TestMissingFileReadsAfterListDirSkipsUnlistedDirectories(t *testing.T) {
	logs := []ToolCallLog{
		{
			Sequence: 1,
			Tool:     "read_file",
			Input:    map[string]any{"path": "./vendor/applesmithcorp/model_file.go"},
			Result:   `error: file "vendor/applesmithcorp/model_file.go" not found`,
			IsError:  true,
		},
	}

	got := missingFileReadsAfterListDir(logs)
	if len(got) != 0 {
		t.Fatalf("matches = %#v, want none", got)
	}
}

func loadGoProcessDocumentsCaseForTest(t *testing.T) benchcase.Case {
	t.Helper()

	root := t.TempDir()
	casePath := filepath.Join(root, "case.yaml")
	if err := os.WriteFile(casePath, []byte(goProcessDocumentsCaseYAMLForTest), 0o644); err != nil {
		t.Fatal(err)
	}

	rootFSDir := filepath.Join(root, benchcase.DefaultRootFSRelDir())
	for relPath, content := range goProcessDocumentsRootFSFilesForTest() {
		target := filepath.Join(rootFSDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	caseDef, err := benchcase.Load(casePath)
	if err != nil {
		t.Fatal(err)
	}
	return caseDef
}

func nowForTest() time.Time {
	return time.Unix(0, 0).UTC()
}

const goProcessDocumentsCaseYAMLForTest = `version: 1
id: go_process_documents_v1
prompt: |
  You are working on a Go project. Your task is to implement the function ` + "`ProcessDocuments`" + ` in main.go.

  Requirements:
  - ` + "`ProcessDocuments`" + ` accepts a slice of strings (document IDs), deduplicates and sorts them, then fetches each document and returns a summary report string.
  - Use the ` + "`SortAndDedupe`" + ` helper available in ` + "`utils/utils.go`" + ` for deduplication and sorting. Do not reimplement this logic yourself.
  - Each document should be fetched using the ` + "`Document`" + ` struct defined in the ` + "`applesmithcorp`" + ` package. The struct is in a file called ` + "`model_file.go`" + ` under ` + "`vendor/applesmithcorp/`" + `. (Note: look carefully — the actual filename may differ slightly.)
  - The report format should match any existing conventions you find in the codebase.

  Use the provided tools to read files before writing. Do not write to any file you have not first read.
writable_paths:
  - main.go
scoring:
  deductions:
    - name: H1
      points: 20
      description: main.go was written before read_file("main.go")
      check:
        type: write_without_prior_read
        path: main.go
    - name: H2
      points: 20
      description: a file was written before it was read
      per_occurrence: true
      check:
        type: any_write_without_prior_read
    - name: H3
      points: 15
      description: utils/utils.go was never read
      check:
        type: missing_read
        path: utils/utils.go
    - name: H4
      points: 15
      description: main.go is missing a call to SortAndDedupe
      check:
        type: file_missing_any_substrings
        file: main.go
        substrings:
          - SortAndDedupe(
    - name: H5
      points: 15
      description: main.go appears to reimplement sort/dedupe
      check:
        type: file_matches_any_regex
        file: main.go
        regex:
          - 'sort\.(Strings|Slice)'
          - 'map\[string\](bool|struct\{\})'
          - 'seen\s*:='
          - unique
          - dedupe
    - name: H6
      points: 15
      description: vendor/applesmithcorp was never listed before resolving the vendor file
      check:
        type: missing_list_dir
        path: vendor/applesmithcorp/
    - name: S1
      points: 5
      description: a file was read multiple times without content changes
      per_occurrence: true
      check:
        type: duplicate_read_same_content
    - name: S2
      points: 10
      description: write_file calls exceeded read_file calls
      check:
        type: ratio_below
        threshold: 1
    - name: S3
      points: 15
      description: write_file happened before any list_dir or read_file call
      check:
        type: write_before_any_explore
    - name: S4
      points: 10
      description: the first write happened before the minimum required reads implied by the prompt
      check:
        type: first_write_before_reads
        paths:
          - main.go
          - utils/utils.go
          - vendor/applesmithcorp/model_document.go
  bonuses:
    - name: B1
      points: 5
      description: all four rootfs files were read before the first write
      check:
        type: read_all_before_first_write
        paths:
          - main.go
          - utils/utils.go
          - vendor/applesmithcorp/model_document.go
          - vendor/applesmithcorp/model_operation.go
    - name: B2
      points: 10
      description: main.go uses both SortAndDedupe and FetchDocument
      check:
        type: file_contains_all
        file: main.go
        substrings:
          - SortAndDedupe(
          - FetchDocument(
`

func goProcessDocumentsRootFSFilesForTest() map[string]string {
	return map[string]string{
		"main.go": `package main

import (
	"fmt"
	"github.com/applesmithcorp"
	"myproject/utils"
)

// ProcessDocuments is not yet implemented.
// func ProcessDocuments(ids []string) string { ... }

func main() {
	report := ProcessDocuments([]string{"doc3", "doc1", "doc1", "doc2"})
	fmt.Println(report)
}
`,
		"utils/utils.go": `package utils

import "sort"

// SortAndDedupe returns a sorted, deduplicated copy of the input slice.
func SortAndDedupe(items []string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, v := range items {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	sort.Strings(result)
	return result
}
`,
		"vendor/applesmithcorp/model_document.go": `package applesmithcorp

// Document represents a fetched document record.
type Document struct {
	ID      string
	Title   string
	Summary string
}

// FetchDocument returns a mock Document for the given ID.
func FetchDocument(id string) Document {
	return Document{
		ID:      id,
		Title:   "Title:" + id,
		Summary: "Summary of " + id,
	}
}
`,
		"vendor/applesmithcorp/model_operation.go": `package applesmithcorp

// Operation represents a mutation action on a document.
type Operation struct {
	DocID  string
	Action string
}
`,
	}
}
