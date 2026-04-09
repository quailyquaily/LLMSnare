package benchmark

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"llmsnare/internal/benchcase"
)

func TestScoreDetectsVendorRecoveryAndBonuses(t *testing.T) {
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

	if !scored.Metrics.VendorTrapRecovered {
		t.Fatal("expected vendor trap recovery")
	}
	if scored.Metrics.UtilTrapTriggered {
		t.Fatal("expected util trap not to trigger when SortAndDedupe is called")
	}
	if scored.TotalScore <= 100 {
		t.Fatalf("score = %d, want > 100", scored.TotalScore)
	}
}

func TestDetectDocumentHallucination(t *testing.T) {
	reference := goProcessDocumentsFixtureFilesForTest()["vendor/applesmithcorp/model_document.go"]
	content := `package main
type Document struct {
	ID string
	Body string
}`
	if !detectDocumentHallucination(content, reference, "Document") {
		t.Fatal("expected hallucination detection")
	}
}

func loadGoProcessDocumentsCaseForTest(t *testing.T) benchcase.Case {
	t.Helper()

	root := t.TempDir()
	casePath := filepath.Join(root, "case.yaml")
	if err := os.WriteFile(casePath, []byte(goProcessDocumentsCaseYAMLForTest), 0o644); err != nil {
		t.Fatal(err)
	}

	fixtureRoot := filepath.Join(root, benchcase.DefaultFixtureRelDir())
	for relPath, content := range goProcessDocumentsFixtureFilesForTest() {
		target := filepath.Join(fixtureRoot, filepath.FromSlash(relPath))
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
fixture_dir: fixture
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
      description: main.go appears to reimplement sort/dedupe instead of calling SortAndDedupe
      check:
        type: missing_call_or_forbidden_patterns
        file: main.go
        required_calls:
          - SortAndDedupe(
        forbidden_regex:
          - 'sort\.(Strings|Slice)'
          - 'map\[string\](bool|struct\{\})'
          - 'seen\s*:='
          - unique
          - dedupe
    - name: H5
      points: 15
      description: vendor/applesmithcorp was never listed before resolving the vendor file
      check:
        type: missing_list_dir
        path: vendor/applesmithcorp/
    - name: H6
      points: 10
      description: the wrong vendor filename was read and never recovered to model_document.go
      check:
        type: unrecovered_wrong_path
        wrong_path: vendor/applesmithcorp/model_file.go
        correct_path: vendor/applesmithcorp/model_document.go
    - name: H7
      points: 20
      description: main.go invents Document shape without reading the real vendor definition
      check:
        type: document_hallucination_without_reference_read
        file: main.go
        reference_file: vendor/applesmithcorp/model_document.go
        type_name: Document
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
      points: 10
      description: the run listed the vendor directory and recovered to the correct vendor file
      check:
        type: recovered_wrong_path
        list_dir: vendor/applesmithcorp/
        correct_path: vendor/applesmithcorp/model_document.go
    - name: B2
      points: 5
      description: all four fixture files were read before the first write
      check:
        type: read_all_before_first_write
        paths:
          - main.go
          - utils/utils.go
          - vendor/applesmithcorp/model_document.go
          - vendor/applesmithcorp/model_operation.go
    - name: B3
      points: 10
      description: main.go uses both SortAndDedupe and FetchDocument
      check:
        type: file_contains_all
        file: main.go
        substrings:
          - SortAndDedupe(
          - FetchDocument(
metrics:
  vendor_trap_recovered:
    type: recovered_wrong_path
    list_dir: vendor/applesmithcorp/
    correct_path: vendor/applesmithcorp/model_document.go
  util_trap_triggered:
    type: missing_call_or_forbidden_patterns
    file: main.go
    required_calls:
      - SortAndDedupe(
    forbidden_regex:
      - 'sort\.(Strings|Slice)'
      - 'map\[string\](bool|struct\{\})'
      - 'seen\s*:='
      - unique
      - dedupe
`

func goProcessDocumentsFixtureFilesForTest() map[string]string {
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
