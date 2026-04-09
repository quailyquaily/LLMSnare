package benchmark

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"llmsnare/internal/benchcase"
	"llmsnare/internal/config"

	"github.com/quailyquaily/uniai"
)

const maxRounds = 24

var (
	errToolArgs     = errors.New("invalid tool arguments")
	readToolSchema  = []byte(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)
	writeToolSchema = []byte(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`)
)

type ChatClient interface {
	Chat(ctx context.Context, opts ...uniai.ChatOption) (*uniai.ChatResult, error)
}

type Runner struct{}

func NewRunner() *Runner {
	return &Runner{}
}

type Result struct {
	Timestamp     time.Time         `json:"timestamp"`
	FinishedAt    time.Time         `json:"finished_at"`
	CaseID        string            `json:"case_id"`
	Profile       string            `json:"profile"`
	Driver        string            `json:"driver"`
	Model         string            `json:"model"`
	Success       bool              `json:"success"`
	Error         string            `json:"error,omitempty"`
	TotalScore    int               `json:"total_score"`
	Metrics       Metrics           `json:"metrics"`
	Deductions    []ScoreAdjustment `json:"deductions,omitempty"`
	Bonuses       []ScoreAdjustment `json:"bonuses,omitempty"`
	ToolCalls     []ToolCallLog     `json:"tool_calls"`
	FinalWrites   map[string]string `json:"final_writes,omitempty"`
	FinalResponse string            `json:"final_response,omitempty"`
}

type Metrics struct {
	ReadFileCalls        int      `json:"read_file_calls"`
	WriteFileCalls       int      `json:"write_file_calls"`
	ListDirCalls         int      `json:"list_dir_calls"`
	ReadWriteRatio       *float64 `json:"read_write_ratio"`
	PreWriteReadCoverage *float64 `json:"pre_write_read_coverage"`
	VendorTrapRecovered  bool     `json:"vendor_trap_recovered"`
	UtilTrapTriggered    bool     `json:"util_trap_triggered"`
}

type ScoreAdjustment struct {
	Name        string `json:"name"`
	Points      int    `json:"points"`
	Description string `json:"description"`
}

type ToolCallLog struct {
	Sequence  int            `json:"sequence"`
	Timestamp time.Time      `json:"timestamp"`
	Tool      string         `json:"tool"`
	Input     map[string]any `json:"input"`
	Result    any            `json:"result"`
	IsError   bool           `json:"is_error"`
}

type toolResponse struct {
	result      any
	modelOutput string
	isError     bool
}

func (r *Runner) Run(ctx context.Context, caseDef benchcase.Case, profileName string, profile config.Profile) (Result, error) {
	client, err := newClient(profile)
	if err != nil {
		result := failureResult(caseDef.ID, profileName, profile, err)
		return result, nil
	}
	ctx, cancel := context.WithTimeout(ctx, profile.Timeout)
	defer cancel()
	return r.RunWithClient(ctx, caseDef, profileName, profile, client)
}

func (r *Runner) RunWithClient(ctx context.Context, caseDef benchcase.Case, profileName string, profile config.Profile, client ChatClient) (Result, error) {
	startedAt := time.Now().UTC()
	fs := newVirtualFS(caseDef, startedAt)
	messages := []uniai.Message{uniai.User(caseDef.Prompt)}
	tools := buildTools()

	result := Result{
		Timestamp: startedAt,
		CaseID:    caseDef.ID,
		Profile:   profileName,
		Driver:    profile.Driver,
		Model:     profile.Model,
	}

	var finalText string
	var runErr error
	for round := 0; round < maxRounds; round++ {
		resp, err := client.Chat(ctx,
			uniai.WithProvider(profile.Driver),
			uniai.WithModel(profile.Model),
			uniai.WithReplaceMessages(messages...),
			uniai.WithTools(tools),
			uniai.WithToolChoice(uniai.ToolChoiceAuto()),
			uniai.WithToolsEmulationMode(uniai.ToolsEmulationFallback),
			uniai.WithTemperature(profile.Temperature),
			uniai.WithMaxTokens(profile.MaxOutputTokens),
		)
		if err != nil {
			runErr = fmt.Errorf("chat round %d: %w", round+1, err)
			break
		}

		if len(resp.ToolCalls) == 0 {
			finalText = resp.Text
			break
		}

		messages = append(messages, uniai.Message{
			Role:      uniai.RoleAssistant,
			Content:   resp.Text,
			ToolCalls: resp.ToolCalls,
		})

		for _, toolCall := range resp.ToolCalls {
			reply := fs.execute(toolCall.Function.Name, toolCall.Function.Arguments)
			messages = append(messages, uniai.ToolResult(toolCall.ID, reply.modelOutput))
		}
	}

	result.FinishedAt = time.Now().UTC()
	result.ToolCalls = append(result.ToolCalls, fs.logs...)
	result.FinalWrites = fs.finalWrites()
	result.FinalResponse = finalText

	scored := scoreResult(caseDef, result, fs)
	result.Metrics = scored.Metrics
	result.Deductions = scored.Deductions
	result.Bonuses = scored.Bonuses
	result.TotalScore = scored.TotalScore

	if runErr == nil && len(fs.logs) == 0 {
		runErr = fmt.Errorf("benchmark ended without any tool calls")
	}

	if runErr != nil {
		result.Success = false
		result.Error = runErr.Error()
		result.TotalScore = 0
		return result, nil
	}

	result.Success = true
	return result, nil
}

func failureResult(caseID, profileName string, profile config.Profile, err error) Result {
	now := time.Now().UTC()
	return Result{
		Timestamp:  now,
		FinishedAt: now,
		CaseID:     caseID,
		Profile:    profileName,
		Driver:     profile.Driver,
		Model:      profile.Model,
		Success:    false,
		Error:      err.Error(),
		TotalScore: 0,
	}
}

func buildTools() []uniai.Tool {
	return []uniai.Tool{
		uniai.FunctionTool("list_dir", "List files in a mock directory", readToolSchema),
		uniai.FunctionTool("read_file", "Read a mock file", readToolSchema),
		uniai.FunctionTool("write_file", "Write a mock file", writeToolSchema),
	}
}

func newClient(profile config.Profile) (ChatClient, error) {
	cfg := uniai.Config{Provider: profile.Driver}

	switch profile.Driver {
	case "openai":
		cfg.OpenAIAPIKey = profile.APIKey
		cfg.OpenAIAPIBase = profile.Endpoint
		cfg.OpenAIModel = profile.Model
	case "anthropic":
		cfg.AnthropicAPIKey = profile.APIKey
		cfg.AnthropicModel = profile.Model
	case "gemini":
		cfg.GeminiAPIKey = profile.APIKey
		cfg.GeminiAPIBase = profile.Endpoint
		cfg.GeminiModel = profile.Model
	default:
		return nil, fmt.Errorf("unsupported driver %q", profile.Driver)
	}

	return uniai.New(cfg), nil
}

type virtualFS struct {
	files map[string]string
	logs  []ToolCallLog
	seq   int
	now   time.Time
}

func newVirtualFS(caseDef benchcase.Case, start time.Time) *virtualFS {
	files := make(map[string]string, len(caseDef.FixtureFiles))
	for p, content := range caseDef.FixtureFiles {
		files[p] = content
	}
	return &virtualFS{files: files, now: start}
}

func (v *virtualFS) execute(name, argsJSON string) toolResponse {
	v.seq++
	v.now = v.now.Add(time.Millisecond)

	var input map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &input); err != nil {
		return v.record(name, nil, toolResponse{
			result:      errToolArgs.Error(),
			modelOutput: errToolArgs.Error(),
			isError:     true,
		})
	}

	switch name {
	case "list_dir":
		dir := normalizeDir(stringValue(input["path"]))
		entries, ok := v.list(dir)
		if !ok {
			msg := fmt.Sprintf("error: directory %q not found", dir)
			return v.record(name, input, toolResponse{result: msg, modelOutput: msg, isError: true})
		}
		payload, _ := json.Marshal(entries)
		return v.record(name, input, toolResponse{result: entries, modelOutput: string(payload)})
	case "read_file":
		filePath := normalizeFile(stringValue(input["path"]))
		content, ok := v.files[filePath]
		if !ok {
			msg := fmt.Sprintf("error: file %q not found", filePath)
			return v.record(name, input, toolResponse{result: msg, modelOutput: msg, isError: true})
		}
		return v.record(name, input, toolResponse{result: content, modelOutput: content})
	case "write_file":
		filePath := normalizeFile(stringValue(input["path"]))
		content := stringValue(input["content"])
		v.files[filePath] = content
		return v.record(name, input, toolResponse{result: "ok", modelOutput: "ok"})
	default:
		msg := fmt.Sprintf("error: unsupported tool %q", name)
		return v.record(name, input, toolResponse{result: msg, modelOutput: msg, isError: true})
	}
}

func (v *virtualFS) record(name string, input map[string]any, response toolResponse) toolResponse {
	v.logs = append(v.logs, ToolCallLog{
		Sequence:  v.seq,
		Timestamp: v.now,
		Tool:      name,
		Input:     input,
		Result:    response.result,
		IsError:   response.isError,
	})
	return response
}

func (v *virtualFS) list(dir string) ([]string, bool) {
	children := make([]string, 0)
	for filePath := range v.files {
		if path.Dir(filePath) == strings.TrimSuffix(dir, "/") {
			children = append(children, path.Base(filePath))
		}
	}
	if len(children) == 0 {
		return nil, false
	}
	sort.Strings(children)
	return children, true
}

func (v *virtualFS) finalWrites() map[string]string {
	if len(v.logs) == 0 {
		return nil
	}
	writes := map[string]string{}
	for _, log := range v.logs {
		if log.Tool != "write_file" || log.IsError {
			continue
		}
		filePath := normalizeFile(stringValue(log.Input["path"]))
		writes[filePath] = v.files[filePath]
	}
	if len(writes) == 0 {
		return nil
	}
	return writes
}

type scoredResult struct {
	Metrics    Metrics
	Deductions []ScoreAdjustment
	Bonuses    []ScoreAdjustment
	TotalScore int
}

type evaluationContext struct {
	caseDef                   benchcase.Case
	result                    Result
	fs                        *virtualFS
	firstSuccessfulReadByFile map[string]int
	firstWriteByFile          map[string]int
	successfulReads           map[string]bool
	metrics                   Metrics
}

func scoreResult(caseDef benchcase.Case, result Result, fs *virtualFS) scoredResult {
	ctx := buildEvaluationContext(caseDef, result, fs)

	deductions := applyRules(ctx, caseDef.Scoring.Deductions, -1)
	bonuses := applyRules(ctx, caseDef.Scoring.Bonuses, 1)

	total := 100
	for _, item := range bonuses {
		total += item.Points
	}
	for _, item := range deductions {
		total += item.Points
	}

	return scoredResult{
		Metrics:    ctx.metrics,
		Deductions: deductions,
		Bonuses:    bonuses,
		TotalScore: total,
	}
}

func buildEvaluationContext(caseDef benchcase.Case, result Result, fs *virtualFS) evaluationContext {
	firstReads := map[string]int{}
	firstWrites := map[string]int{}
	successfulReads := map[string]bool{}

	var (
		readCalls  int
		writeCalls int
		listCalls  int
	)

	writtenFiles := map[string]bool{}
	readBeforeWrite := map[string]bool{}

	for _, log := range fs.logs {
		switch log.Tool {
		case "read_file":
			readCalls++
			if log.IsError {
				continue
			}
			filePath := normalizeFile(stringValue(log.Input["path"]))
			successfulReads[filePath] = true
			if _, ok := firstReads[filePath]; !ok {
				firstReads[filePath] = log.Sequence
			}
		case "write_file":
			writeCalls++
			if log.IsError {
				continue
			}
			filePath := normalizeFile(stringValue(log.Input["path"]))
			writtenFiles[filePath] = true
			if _, ok := firstWrites[filePath]; !ok {
				firstWrites[filePath] = log.Sequence
			}
		case "list_dir":
			listCalls++
		}
	}

	for filePath := range writtenFiles {
		readSeq, readOK := firstReads[filePath]
		writeSeq, writeOK := firstWrites[filePath]
		if readOK && writeOK && readSeq < writeSeq {
			readBeforeWrite[filePath] = true
		}
	}

	var readWriteRatio *float64
	if writeCalls > 0 {
		value := float64(readCalls) / float64(writeCalls)
		readWriteRatio = &value
	}

	var coverage *float64
	if len(writtenFiles) > 0 {
		value := float64(len(readBeforeWrite)) / float64(len(writtenFiles))
		coverage = &value
	}

	ctx := evaluationContext{
		caseDef:                   caseDef,
		result:                    result,
		fs:                        fs,
		firstSuccessfulReadByFile: firstReads,
		firstWriteByFile:          firstWrites,
		successfulReads:           successfulReads,
		metrics: Metrics{
			ReadFileCalls:        readCalls,
			WriteFileCalls:       writeCalls,
			ListDirCalls:         listCalls,
			ReadWriteRatio:       readWriteRatio,
			PreWriteReadCoverage: coverage,
		},
	}

	if caseDef.Metrics.VendorTrapRecovered != nil {
		ctx.metrics.VendorTrapRecovered = len(evaluateCheck(ctx, *caseDef.Metrics.VendorTrapRecovered)) > 0
	}
	if caseDef.Metrics.UtilTrapTriggered != nil {
		ctx.metrics.UtilTrapTriggered = len(evaluateCheck(ctx, *caseDef.Metrics.UtilTrapTriggered)) > 0
	}

	return ctx
}

func applyRules(ctx evaluationContext, rules []benchcase.Rule, sign int) []ScoreAdjustment {
	items := make([]ScoreAdjustment, 0)
	for _, rule := range rules {
		matches := evaluateCheck(ctx, rule.Check)
		if len(matches) == 0 {
			continue
		}

		points := sign * rule.Points
		if rule.PerOccurrence {
			for _, match := range matches {
				desc := rule.Description
				if match != "" {
					desc = match
				}
				items = append(items, ScoreAdjustment{
					Name:        rule.Name,
					Points:      points,
					Description: desc,
				})
			}
			continue
		}

		desc := rule.Description
		if desc == "" {
			desc = matches[0]
		}
		items = append(items, ScoreAdjustment{
			Name:        rule.Name,
			Points:      points,
			Description: desc,
		})
	}
	return items
}

func evaluateCheck(ctx evaluationContext, check benchcase.Check) []string {
	switch check.Type {
	case "write_without_prior_read":
		if fileWasWrittenBeforeRead(ctx, check.Path) {
			return []string{fmt.Sprintf("%s was written before it was read", check.Path)}
		}
	case "any_write_without_prior_read":
		return filesWrittenBeforeRead(ctx)
	case "missing_read":
		if !ctx.successfulReads[check.Path] {
			return []string{fmt.Sprintf("%s was never read", check.Path)}
		}
	case "missing_list_dir":
		if !hasSuccessfulListDir(ctx.fs.logs, check.Path) {
			return []string{fmt.Sprintf("%s was never listed", check.Path)}
		}
	case "unrecovered_wrong_path":
		if attemptedPath(ctx.fs.logs, "read_file", check.WrongPath) && !ctx.successfulReads[check.CorrectPath] {
			return []string{fmt.Sprintf("%s was not recovered to %s", check.WrongPath, check.CorrectPath)}
		}
	case "duplicate_read_same_content":
		return duplicateReadMatches(ctx.fs.logs)
	case "ratio_below":
		if ctx.metrics.ReadWriteRatio != nil && *ctx.metrics.ReadWriteRatio < check.Threshold {
			return []string{fmt.Sprintf("read:write ratio %.2f is below %.2f", *ctx.metrics.ReadWriteRatio, check.Threshold)}
		}
	case "write_before_any_explore":
		if wroteBeforeAnyReadOrList(ctx.fs.logs) {
			return []string{"write_file happened before any list_dir or read_file"}
		}
	case "first_write_before_reads":
		if firstWriteBeforeRequiredReads(ctx, check.Paths) {
			return []string{"the first write happened before all required reads"}
		}
	case "recovered_wrong_path":
		if hasSuccessfulListDir(ctx.fs.logs, check.ListDir) && ctx.successfulReads[check.CorrectPath] {
			return []string{"vendor trap was recovered"}
		}
	case "read_all_before_first_write":
		if readAllBeforeFirstWrite(ctx.fs.logs, check.Paths) {
			return []string{"all required files were read before the first write"}
		}
	case "file_contains_all":
		if fileContainsAll(ctx.fileContent(check.File), check.Substrings) {
			return []string{"file contains all required substrings"}
		}
	case "missing_call_or_forbidden_patterns":
		if missingCallOrForbiddenPatterns(ctx.fileContent(check.File), check.RequiredCalls, check.ForbiddenRegex) {
			return []string{"required helper call is missing or forbidden patterns were found"}
		}
	case "document_hallucination_without_reference_read":
		typeName := check.TypeName
		if typeName == "" {
			typeName = "Document"
		}
		if !ctx.successfulReads[check.ReferenceFile] && detectDocumentHallucination(ctx.fileContent(check.File), ctx.caseDef.FixtureFiles[check.ReferenceFile], typeName) {
			return []string{fmt.Sprintf("%s appears to invent %s without reading %s", check.File, typeName, check.ReferenceFile)}
		}
	}
	return nil
}

func (ctx evaluationContext) fileContent(filePath string) string {
	filePath = normalizeFile(filePath)
	if content, ok := ctx.result.FinalWrites[filePath]; ok {
		return content
	}
	return ctx.fs.files[filePath]
}

func fileWasWrittenBeforeRead(ctx evaluationContext, filePath string) bool {
	filePath = normalizeFile(filePath)
	writeSeq, ok := ctx.firstWriteByFile[filePath]
	if !ok {
		return false
	}
	readSeq, ok := ctx.firstSuccessfulReadByFile[filePath]
	return !ok || readSeq > writeSeq
}

func filesWrittenBeforeRead(ctx evaluationContext) []string {
	files := make([]string, 0, len(ctx.firstWriteByFile))
	for filePath := range ctx.firstWriteByFile {
		files = append(files, filePath)
	}
	sort.Strings(files)

	matches := make([]string, 0)
	for _, filePath := range files {
		if fileWasWrittenBeforeRead(ctx, filePath) {
			matches = append(matches, fmt.Sprintf("%s was written before it was read", filePath))
		}
	}
	return matches
}

func duplicateReadMatches(logs []ToolCallLog) []string {
	type readKey struct {
		path    string
		content string
	}

	seen := map[readKey]bool{}
	matches := make([]string, 0)
	for _, log := range logs {
		if log.Tool != "read_file" || log.IsError {
			continue
		}
		filePath := normalizeFile(stringValue(log.Input["path"]))
		content, _ := log.Result.(string)
		key := readKey{path: filePath, content: content}
		if seen[key] {
			matches = append(matches, fmt.Sprintf("%s was read multiple times without content changes", filePath))
			continue
		}
		seen[key] = true
	}
	return matches
}

func wroteBeforeAnyReadOrList(logs []ToolCallLog) bool {
	for _, log := range logs {
		switch log.Tool {
		case "read_file", "list_dir":
			return false
		case "write_file":
			return true
		}
	}
	return false
}

func firstWriteBeforeRequiredReads(ctx evaluationContext, paths []string) bool {
	firstWrite := 0
	for _, seq := range ctx.firstWriteByFile {
		if firstWrite == 0 || seq < firstWrite {
			firstWrite = seq
		}
	}
	if firstWrite == 0 {
		return false
	}
	for _, requiredPath := range paths {
		readSeq, ok := ctx.firstSuccessfulReadByFile[normalizeFile(requiredPath)]
		if !ok || readSeq > firstWrite {
			return true
		}
	}
	return false
}

func readAllBeforeFirstWrite(logs []ToolCallLog, paths []string) bool {
	firstWrite := 0
	reads := map[string]bool{}
	for _, log := range logs {
		if log.Tool == "write_file" && !log.IsError {
			firstWrite = log.Sequence
			break
		}
		if log.Tool == "read_file" && !log.IsError {
			reads[normalizeFile(stringValue(log.Input["path"]))] = true
		}
	}
	if firstWrite == 0 {
		return false
	}
	for _, required := range paths {
		if !reads[normalizeFile(required)] {
			return false
		}
	}
	return true
}

func fileContainsAll(content string, substrings []string) bool {
	for _, item := range substrings {
		if !strings.Contains(content, item) {
			return false
		}
	}
	return len(substrings) > 0
}

func missingCallOrForbiddenPatterns(content string, requiredCalls, forbiddenRegex []string) bool {
	for _, call := range requiredCalls {
		if !strings.Contains(content, call) {
			return true
		}
	}
	for _, pattern := range forbiddenRegex {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		if re.MatchString(content) {
			return true
		}
	}
	return false
}

func detectDocumentHallucination(mainContent, referenceSource, typeName string) bool {
	if strings.Contains(mainContent, "type "+typeName+" struct") {
		return true
	}

	validFields, err := benchcase.ExtractStructFields(referenceSource, typeName)
	if err != nil {
		return false
	}

	file, err := parser.ParseFile(token.NewFileSet(), "main.go", mainContent, 0)
	if err != nil {
		return false
	}

	hallucinated := false
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.TypeSpec:
			if typed.Name.Name == typeName {
				hallucinated = true
				return false
			}
		case *ast.CompositeLit:
			if !isNamedTypeExpr(typed.Type, typeName) {
				return true
			}
			for _, elt := range typed.Elts {
				keyValue, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := keyValue.Key.(*ast.Ident)
				if !ok {
					continue
				}
				if _, ok := validFields[key.Name]; !ok {
					hallucinated = true
					return false
				}
			}
		}
		return true
	})
	return hallucinated
}

func isNamedTypeExpr(expr ast.Expr, typeName string) bool {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name == typeName
	case *ast.SelectorExpr:
		return typed.Sel.Name == typeName
	default:
		return false
	}
}

func hasSuccessfulListDir(logs []ToolCallLog, dir string) bool {
	target := normalizeDir(dir)
	for _, log := range logs {
		if log.Tool == "list_dir" && !log.IsError && normalizeDir(stringValue(log.Input["path"])) == target {
			return true
		}
	}
	return false
}

func attemptedPath(logs []ToolCallLog, tool, targetPath string) bool {
	targetPath = normalizeFile(targetPath)
	for _, log := range logs {
		if log.Tool == tool && normalizeFile(stringValue(log.Input["path"])) == targetPath {
			return true
		}
	}
	return false
}

func normalizeDir(value string) string {
	cleaned := path.Clean(strings.TrimSpace(value))
	if cleaned == "." || cleaned == "/" {
		return ""
	}
	if !strings.HasSuffix(cleaned, "/") {
		cleaned += "/"
	}
	return cleaned
}

func normalizeFile(value string) string {
	cleaned := path.Clean(strings.TrimSpace(value))
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
