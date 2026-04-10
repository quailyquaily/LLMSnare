package benchmark

import (
	"context"
	"encoding/base64"
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

type ProgressEventKind string

const (
	ProgressRunStarted   ProgressEventKind = "run_started"
	ProgressRoundStarted ProgressEventKind = "round_started"
	ProgressToolBatch    ProgressEventKind = "tool_batch"
	ProgressToolExecuted ProgressEventKind = "tool_executed"
	ProgressRunFinished  ProgressEventKind = "run_finished"
)

type ProgressEvent struct {
	Kind            ProgressEventKind
	CaseID          string
	Profile         string
	Round           int
	MaxRounds       int
	Tool            string
	ToolPath        string
	ToolCalls       int
	Elapsed         time.Duration
	RawScore        int
	MaxScore        int
	NormalizedScore float64
	Success         bool
	Error           string
}

type ProgressReporter func(ProgressEvent)

type RunnerOption func(*Runner)

type Runner struct {
	progress ProgressReporter
}

func NewRunner(opts ...RunnerOption) *Runner {
	runner := &Runner{}
	for _, opt := range opts {
		if opt != nil {
			opt(runner)
		}
	}
	return runner
}

func WithProgressReporter(reporter ProgressReporter) RunnerOption {
	return func(r *Runner) {
		r.progress = reporter
	}
}

func (r *Runner) report(event ProgressEvent) {
	if r.progress != nil {
		r.progress(event)
	}
}

type Result struct {
	Timestamp       time.Time         `json:"timestamp"`
	FinishedAt      time.Time         `json:"finished_at"`
	CaseID          string            `json:"case_id"`
	Profile         string            `json:"profile"`
	Provider        string            `json:"provider"`
	Model           string            `json:"model"`
	Endpoint        string            `json:"endpoint"`
	Success         bool              `json:"success"`
	Error           string            `json:"error,omitempty"`
	TotalScore      int               `json:"total_score"`
	RawScore        int               `json:"raw_score"`
	MaxScore        int               `json:"max_score"`
	NormalizedScore float64           `json:"normalized_score"`
	Metrics         Metrics           `json:"metrics"`
	Deductions      []ScoreAdjustment `json:"deductions,omitempty"`
	Bonuses         []ScoreAdjustment `json:"bonuses,omitempty"`
	ToolCalls       []ToolCallLog     `json:"tool_calls"`
	FinalWrites     map[string]string `json:"final_writes,omitempty"`
	FinalResponse   string            `json:"final_response,omitempty"`
}

type Metrics struct {
	ReadFileCalls        int      `json:"read_file_calls"`
	WriteFileCalls       int      `json:"write_file_calls"`
	ListDirCalls         int      `json:"list_dir_calls"`
	ReadWriteRatio       *float64 `json:"read_write_ratio"`
	PreWriteReadCoverage *float64 `json:"pre_write_read_coverage"`
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
		r.report(ProgressEvent{
			Kind:    ProgressRunFinished,
			CaseID:  caseDef.ID,
			Profile: profileName,
			Error:   err.Error(),
		})
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
		Provider:  profile.Provider,
		Model:     profile.Model,
		Endpoint:  profile.Endpoint,
	}

	r.report(ProgressEvent{
		Kind:      ProgressRunStarted,
		CaseID:    caseDef.ID,
		Profile:   profileName,
		MaxRounds: maxRounds,
	})

	var finalText string
	var runErr error
	for round := 0; round < maxRounds; round++ {
		roundNumber := round + 1
		r.report(ProgressEvent{
			Kind:      ProgressRoundStarted,
			CaseID:    caseDef.ID,
			Profile:   profileName,
			Round:     roundNumber,
			MaxRounds: maxRounds,
		})

		resp, err := client.Chat(ctx,
			uniai.WithProvider(profile.Provider),
			uniai.WithModel(profile.Model),
			uniai.WithReplaceMessages(messages...),
			uniai.WithTools(tools),
			uniai.WithToolChoice(uniai.ToolChoiceAuto()),
			uniai.WithToolsEmulationMode(uniai.ToolsEmulationOff),
			uniai.WithTemperature(profile.Temperature),
			uniai.WithMaxTokens(profile.MaxOutputTokens),
		)
		if err != nil {
			runErr = fmt.Errorf("chat round %d: %w", roundNumber, err)
			break
		}

		if len(resp.ToolCalls) == 0 {
			finalText = resp.Text
			break
		}

		r.report(ProgressEvent{
			Kind:      ProgressToolBatch,
			CaseID:    caseDef.ID,
			Profile:   profileName,
			Round:     roundNumber,
			MaxRounds: maxRounds,
			ToolCalls: len(resp.ToolCalls),
		})

		toolCalls := replayToolCallsForProvider(profile.Provider, resp.ToolCalls)
		messages = append(messages, assistantToolReplayMessage(resp.Text, toolCalls))

		for _, toolCall := range toolCalls {
			reply := fs.execute(toolCall.Function.Name, toolCall.Function.Arguments)
			toolResult, err := toolResultMessageForProvider(profile.Provider, toolCall, reply)
			if err != nil {
				runErr = fmt.Errorf("tool result round %d: %w", roundNumber, err)
				break
			}
			messages = append(messages, toolResult)

			if len(fs.logs) == 0 {
				continue
			}
			last := fs.logs[len(fs.logs)-1]
			event := ProgressEvent{
				Kind:      ProgressToolExecuted,
				CaseID:    caseDef.ID,
				Profile:   profileName,
				Round:     roundNumber,
				MaxRounds: maxRounds,
				Tool:      last.Tool,
				ToolPath:  stringValue(last.Input["path"]),
			}
			if last.IsError {
				event.Error = fmt.Sprint(last.Result)
			}
			r.report(event)
		}
		if runErr != nil {
			break
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
	result.RawScore = scored.TotalScore
	result.MaxScore = scored.MaxScore
	result.NormalizedScore = scored.NormalizedScore

	if runErr == nil && len(fs.logs) == 0 {
		runErr = fmt.Errorf("benchmark ended without any tool calls")
	}

	if runErr != nil {
		result.Success = false
		result.Error = runErr.Error()
		result.TotalScore = 0
		result.RawScore = 0
		result.NormalizedScore = 0
		r.report(ProgressEvent{
			Kind:            ProgressRunFinished,
			CaseID:          caseDef.ID,
			Profile:         profileName,
			Elapsed:         result.FinishedAt.Sub(startedAt),
			RawScore:        result.RawScore,
			MaxScore:        result.MaxScore,
			NormalizedScore: result.NormalizedScore,
			Error:           result.Error,
		})
		return result, nil
	}

	result.Success = isPassingScore(result.TotalScore)
	r.report(ProgressEvent{
		Kind:            ProgressRunFinished,
		CaseID:          caseDef.ID,
		Profile:         profileName,
		Elapsed:         result.FinishedAt.Sub(startedAt),
		RawScore:        result.RawScore,
		MaxScore:        result.MaxScore,
		NormalizedScore: result.NormalizedScore,
		Success:         result.Success,
	})
	return result, nil
}

func cloneToolCalls(toolCalls []uniai.ToolCall) []uniai.ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	out := make([]uniai.ToolCall, len(toolCalls))
	copy(out, toolCalls)
	return out
}

func assistantToolReplayMessage(text string, toolCalls []uniai.ToolCall) uniai.Message {
	msg := uniai.AssistantToolCalls(toolCalls...)
	msg.Content = text
	return msg
}

func replayToolCallsForProvider(provider string, toolCalls []uniai.ToolCall) []uniai.ToolCall {
	calls := cloneToolCalls(toolCalls)
	if !strings.EqualFold(strings.TrimSpace(provider), "gemini") {
		return calls
	}

	// Gemini can return a signature only on the first tool call in a parallel batch,
	// but the current uniai replay validation still expects every replayed call to carry one.
	lastSignature := ""
	for i := range calls {
		signature := strings.TrimSpace(calls[i].ThoughtSignature)
		if signature == "" {
			signature = decodeGeminiThoughtSignatureFromToolCallID(calls[i].ID)
		}
		if signature == "" {
			signature = lastSignature
		}
		if signature == "" {
			continue
		}
		calls[i].ThoughtSignature = signature
		lastSignature = signature
	}
	return calls
}

func toolResultMessageForProvider(provider string, toolCall uniai.ToolCall, reply toolResponse) (uniai.Message, error) {
	if !strings.EqualFold(strings.TrimSpace(provider), "gemini") {
		return uniai.ToolResult(toolCall.ID, reply.modelOutput), nil
	}
	return uniai.ToolResultValue(toolCall.ID, geminiToolResultValue(toolCall.Function.Name, reply))
}

func geminiToolResultValue(toolName string, reply toolResponse) any {
	if reply.isError {
		return map[string]any{"error": fmt.Sprint(reply.result)}
	}
	switch toolName {
	case "list_dir":
		return map[string]any{"entries": reply.result}
	case "read_file":
		return map[string]any{"content": fmt.Sprint(reply.result)}
	case "write_file":
		return map[string]any{"status": fmt.Sprint(reply.result)}
	default:
		return reply.result
	}
}

func decodeGeminiThoughtSignatureFromToolCallID(callID string) string {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return ""
	}
	idx := strings.LastIndex(callID, "|ts:")
	if idx <= 0 || idx+4 >= len(callID) {
		return ""
	}
	encoded := callID[idx+4:]
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return ""
	}
	return string(decoded)
}

func failureResult(caseID, profileName string, profile config.Profile, err error) Result {
	now := time.Now().UTC()
	return Result{
		Timestamp:       now,
		FinishedAt:      now,
		CaseID:          caseID,
		Profile:         profileName,
		Provider:        profile.Provider,
		Model:           profile.Model,
		Endpoint:        profile.Endpoint,
		Success:         false,
		Error:           err.Error(),
		TotalScore:      0,
		RawScore:        0,
		MaxScore:        0,
		NormalizedScore: 0,
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
	cfg := uniai.Config{Provider: profile.Provider}

	switch profile.Provider {
	case "openai":
		cfg.OpenAIAPIKey = profile.APIKey
		cfg.OpenAIAPIBase = profile.Endpoint
		cfg.OpenAIModel = profile.Model
	case "openai_resp":
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
	case "cloudflare":
		cfg.CloudflareAccountID = profile.AccountID
		cfg.CloudflareAPIToken = profile.APIToken
		cfg.CloudflareAPIBase = profile.Endpoint
	default:
		return nil, fmt.Errorf("unsupported provider %q", profile.Provider)
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
	files := make(map[string]string, len(caseDef.RootFSFiles))
	for p, content := range caseDef.RootFSFiles {
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
	target := normalizeDir(dir)
	prefix := ""
	if target != "." {
		prefix = target + "/"
	}

	children := make(map[string]struct{})
	for filePath := range v.files {
		filePath = normalizeFile(filePath)
		if filePath == "" {
			continue
		}

		rest := ""
		switch {
		case target == ".":
			rest = filePath
		case strings.HasPrefix(filePath, prefix):
			rest = strings.TrimPrefix(filePath, prefix)
		default:
			continue
		}

		child := rest
		if idx := strings.IndexByte(rest, '/'); idx >= 0 {
			child = rest[:idx]
		}
		if child != "" {
			children[child] = struct{}{}
		}
	}

	if len(children) == 0 {
		return nil, false
	}

	entries := make([]string, 0, len(children))
	for child := range children {
		entries = append(entries, child)
	}
	sort.Strings(entries)
	return entries, true
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
	Metrics         Metrics
	Deductions      []ScoreAdjustment
	Bonuses         []ScoreAdjustment
	TotalScore      int
	MaxScore        int
	NormalizedScore float64
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

	maxScore := 100
	for _, rule := range caseDef.Scoring.Bonuses {
		if rule.Points > 0 {
			maxScore += rule.Points
		}
	}

	return scoredResult{
		Metrics:         ctx.metrics,
		Deductions:      deductions,
		Bonuses:         bonuses,
		TotalScore:      total,
		MaxScore:        maxScore,
		NormalizedScore: normalizeScore(total, maxScore),
	}
}

func normalizeScore(rawScore, maxScore int) float64 {
	if maxScore <= 0 {
		return 0
	}
	if rawScore > maxScore {
		rawScore = maxScore
	}
	return (float64(rawScore) / float64(maxScore)) * 100
}

func isPassingScore(rawScore int) bool {
	return rawScore > 0
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
	case "duplicate_read_same_content":
		return duplicateReadMatches(ctx.fs.logs)
	case "read_missing_file_after_list_dir":
		return missingFileReadsAfterListDir(ctx.fs.logs)
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
	case "read_all_before_first_write":
		if readAllBeforeFirstWrite(ctx.fs.logs, check.Paths) {
			return []string{"all required files were read before the first write"}
		}
	case "file_contains_all":
		if fileContainsAll(ctx.fileContent(check.File), check.Substrings) {
			return []string{"file contains all required substrings"}
		}
	case "file_missing_any_substrings":
		if fileMissingAnySubstrings(ctx.fileContent(check.File), check.Substrings) {
			return []string{"file is missing required substrings"}
		}
	case "file_matches_all_regex":
		if fileMatchesAllRegex(ctx.fileContent(check.File), check.Regex) {
			return []string{"file matches all required regex patterns"}
		}
	case "file_matches_any_regex":
		if fileMatchesAnyRegex(ctx.fileContent(check.File), check.Regex) {
			return []string{"file matches forbidden regex patterns"}
		}
	case "missing_go_function":
		if !goFileDefinesFunction(ctx.fileContent(check.File), check.FunctionName) {
			return []string{fmt.Sprintf("%s does not define function %s", check.File, check.FunctionName)}
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

func missingFileReadsAfterListDir(logs []ToolCallLog) []string {
	matches := make([]string, 0)
	for _, log := range logs {
		if !isMissingFileReadError(log) {
			continue
		}
		filePath := normalizeFile(stringValue(log.Input["path"]))
		parentDir := normalizeDir(path.Dir(filePath))
		if !hasSuccessfulListDirBefore(logs, parentDir, log.Sequence) {
			continue
		}
		matches = append(matches, fmt.Sprintf("%s was read after listing %s, but the file does not exist", filePath, parentDir))
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

func fileMissingAnySubstrings(content string, substrings []string) bool {
	return len(substrings) > 0 && !fileContainsAll(content, substrings)
}

func fileMatchesAllRegex(content string, patterns []string) bool {
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil || !re.MatchString(content) {
			return false
		}
	}
	return len(patterns) > 0
}

func fileMatchesAnyRegex(content string, patterns []string) bool {
	for _, pattern := range patterns {
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

func goFileDefinesFunction(source, functionName string) bool {
	if strings.TrimSpace(source) == "" || strings.TrimSpace(functionName) == "" {
		return false
	}

	file, err := parser.ParseFile(token.NewFileSet(), "main.go", source, parser.AllErrors)
	if err != nil {
		return false
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Recv != nil || fn.Name == nil || fn.Name.Name != functionName {
			continue
		}
		if fn.Body != nil {
			return true
		}
	}
	return false
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

func hasSuccessfulListDirBefore(logs []ToolCallLog, dir string, beforeSeq int) bool {
	target := normalizeDir(dir)
	for _, log := range logs {
		if log.Sequence >= beforeSeq {
			return false
		}
		if log.Tool == "list_dir" && !log.IsError && normalizeDir(stringValue(log.Input["path"])) == target {
			return true
		}
	}
	return false
}

func isMissingFileReadError(log ToolCallLog) bool {
	if log.Tool != "read_file" || !log.IsError {
		return false
	}
	msg := strings.TrimSpace(fmt.Sprint(log.Result))
	return strings.HasPrefix(msg, `error: file "`) && strings.HasSuffix(msg, `" not found`)
}

func normalizeDir(value string) string {
	cleaned := path.Clean(strings.TrimSpace(value))
	if cleaned == "." || cleaned == "/" || cleaned == "" {
		return "."
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
