package benchmark

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"llmsnare/internal/benchcase"
	"llmsnare/internal/config"

	"github.com/quailyquaily/uniai"
	uniaichat "github.com/quailyquaily/uniai/chat"
)

type stubChatClient struct {
	results []*uniai.ChatResult
	calls   int
}

func (s *stubChatClient) Chat(ctx context.Context, opts ...uniai.ChatOption) (*uniai.ChatResult, error) {
	if s.calls >= len(s.results) {
		return nil, fmt.Errorf("unexpected chat call %d", s.calls+1)
	}
	result := s.results[s.calls]
	s.calls++
	return result, nil
}

type recordingChatClient struct {
	result *uniai.ChatResult
	reqs   []*uniaichat.Request
}

func (r *recordingChatClient) Chat(ctx context.Context, opts ...uniai.ChatOption) (*uniai.ChatResult, error) {
	req, err := uniaichat.BuildRequest(opts...)
	if err != nil {
		return nil, err
	}
	r.reqs = append(r.reqs, req)
	if r.result == nil {
		return &uniai.ChatResult{Text: "done"}, nil
	}
	return r.result, nil
}

type scriptedRecordingChatClient struct {
	results []*uniai.ChatResult
	reqs    []*uniaichat.Request
	calls   int
}

func (r *scriptedRecordingChatClient) Chat(ctx context.Context, opts ...uniai.ChatOption) (*uniai.ChatResult, error) {
	req, err := uniaichat.BuildRequest(opts...)
	if err != nil {
		return nil, err
	}
	r.reqs = append(r.reqs, req)
	if r.calls >= len(r.results) {
		return nil, fmt.Errorf("unexpected chat call %d", r.calls+1)
	}
	result := r.results[r.calls]
	r.calls++
	return result, nil
}

func TestRunWithClientReportsProgress(t *testing.T) {
	caseDef := loadGoProcessDocumentsCaseForTest(t)
	client := &stubChatClient{
		results: []*uniai.ChatResult{
			{
				Text: "inspect and edit",
				ToolCalls: []uniai.ToolCall{
					{
						ID: "call_read_main",
						Function: uniai.ToolCallFunction{
							Name:      "read_file",
							Arguments: `{"path":"main.go"}`,
						},
					},
					{
						ID: "call_write_main",
						Function: uniai.ToolCallFunction{
							Name:      "write_file",
							Arguments: `{"path":"main.go","content":"package main\n"}`,
						},
					},
				},
			},
			{Text: "done"},
		},
	}

	var events []ProgressEvent
	runner := NewRunner(WithProgressReporter(func(event ProgressEvent) {
		events = append(events, event)
	}))

	result, err := runner.RunWithClient(
		context.Background(),
		caseDef,
		"demo_profile",
		config.Profile{Provider: "openai", Model: "gpt-4o", ModelVendor: "openai", InferenceProvider: "groq"},
		client,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error %q", result.Error)
	}

	wantKinds := []ProgressEventKind{
		ProgressRunStarted,
		ProgressRoundStarted,
		ProgressToolBatch,
		ProgressToolExecuted,
		ProgressToolExecuted,
		ProgressRoundStarted,
		ProgressRunFinished,
	}

	if len(events) != len(wantKinds) {
		t.Fatalf("events = %d, want %d", len(events), len(wantKinds))
	}
	for i, kind := range wantKinds {
		if events[i].Kind != kind {
			t.Fatalf("event[%d].Kind = %q, want %q", i, events[i].Kind, kind)
		}
	}

	if events[3].Tool != "read_file" || events[3].ToolPath != "main.go" {
		t.Fatalf("read event = %#v, want read_file main.go", events[3])
	}
	if events[4].Tool != "write_file" || events[4].ToolPath != "main.go" {
		t.Fatalf("write event = %#v, want write_file main.go", events[4])
	}
	if !events[len(events)-1].Success {
		t.Fatal("expected final progress event to report success")
	}
	if events[0].ModelVendor != "openai" || events[0].InferenceProvider != "groq" {
		t.Fatalf("start event metadata = %#v, want model_vendor=openai inference_provider=groq", events[0])
	}
	if events[len(events)-1].RawScore != result.RawScore {
		t.Fatalf("final raw score = %d, want %d", events[len(events)-1].RawScore, result.RawScore)
	}
	if events[len(events)-1].NormalizedScore != result.NormalizedScore {
		t.Fatalf("final normalized score = %v, want %v", events[len(events)-1].NormalizedScore, result.NormalizedScore)
	}
	if result.ModelVendor != "openai" || result.InferenceProvider != "groq" {
		t.Fatalf("result metadata = %#v, want model_vendor=openai inference_provider=groq", result)
	}
}

func TestRunWithClientMarksNonPositiveScoreAsFailure(t *testing.T) {
	caseDef := loadGoProcessDocumentsCaseForTest(t)
	client := &stubChatClient{
		results: []*uniai.ChatResult{
			{
				Text: "write immediately",
				ToolCalls: []uniai.ToolCall{
					{
						ID: "call_write_main",
						Function: uniai.ToolCallFunction{
							Name:      "write_file",
							Arguments: `{"path":"main.go","content":"package main\nfunc ProcessDocuments(ids []string) string { return \"ok\" }\n"}`,
						},
					},
				},
			},
			{Text: "done"},
		},
	}

	result, err := NewRunner().RunWithClient(
		context.Background(),
		caseDef,
		"demo_profile",
		config.Profile{Provider: "openai", Model: "gpt-4o"},
		client,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatalf("expected failure for non-positive score, got success with raw score %d", result.RawScore)
	}
	if result.RawScore >= 0 {
		t.Fatalf("expected negative raw score, got %d", result.RawScore)
	}
	if result.NormalizedScore >= 0 {
		t.Fatalf("expected negative normalized score, got %v", result.NormalizedScore)
	}
}

func TestRunWithClientDisablesToolEmulationFallbackForGemini(t *testing.T) {
	caseDef := loadGoProcessDocumentsCaseForTest(t)
	client := &recordingChatClient{}
	runner := NewRunner()

	_, err := runner.RunWithClient(
		context.Background(),
		caseDef,
		"gemini_profile",
		config.Profile{Provider: "gemini", Model: "gemini-3.1-pro-preview"},
		client,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.reqs) == 0 {
		t.Fatal("expected at least one chat request")
	}
	if got := client.reqs[0].Options.ToolsEmulationMode; got != uniai.ToolsEmulationOff {
		t.Fatalf("tools emulation mode = %q, want %q", got, uniai.ToolsEmulationOff)
	}
}

func TestRunWithClientDisablesToolEmulationFallbackForOpenAI(t *testing.T) {
	caseDef := loadGoProcessDocumentsCaseForTest(t)
	client := &recordingChatClient{}
	runner := NewRunner()

	_, err := runner.RunWithClient(
		context.Background(),
		caseDef,
		"openai_profile",
		config.Profile{Provider: "openai", Model: "gpt-4o"},
		client,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.reqs) == 0 {
		t.Fatal("expected at least one chat request")
	}
	if got := client.reqs[0].Options.ToolsEmulationMode; got != uniai.ToolsEmulationOff {
		t.Fatalf("tools emulation mode = %q, want %q", got, uniai.ToolsEmulationOff)
	}
}

func TestRunWithClientUsesCaseTools(t *testing.T) {
	caseDef := loadGoProcessDocumentsCaseForTest(t)
	caseDef.Tools = []string{
		benchcase.ToolSearchText,
		benchcase.ToolReadFile,
		benchcase.ToolWriteFile,
	}
	client := &recordingChatClient{}

	_, err := NewRunner().RunWithClient(
		context.Background(),
		caseDef,
		"openai_profile",
		config.Profile{Provider: "openai", Model: "gpt-4o"},
		client,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.reqs) == 0 {
		t.Fatal("expected at least one chat request")
	}

	var got []string
	for _, tool := range client.reqs[0].Tools {
		got = append(got, tool.Function.Name)
	}
	if !reflect.DeepEqual(got, caseDef.Tools) {
		t.Fatalf("tools = %#v, want %#v", got, caseDef.Tools)
	}
}

func TestRunWithClientOmitsTemperatureWhenUnset(t *testing.T) {
	caseDef := loadGoProcessDocumentsCaseForTest(t)
	client := &recordingChatClient{}

	_, err := NewRunner().RunWithClient(
		context.Background(),
		caseDef,
		"openai_profile",
		config.Profile{Provider: "openai", Model: "gpt-4o"},
		client,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.reqs) == 0 {
		t.Fatal("expected at least one chat request")
	}
	if got := client.reqs[0].Options.Temperature; got != nil {
		t.Fatalf("temperature = %#v, want nil when unset", got)
	}
}

func TestRunWithClientPassesExplicitTemperature(t *testing.T) {
	caseDef := loadGoProcessDocumentsCaseForTest(t)
	client := &recordingChatClient{}
	temperature := 0.0

	_, err := NewRunner().RunWithClient(
		context.Background(),
		caseDef,
		"openai_profile",
		config.Profile{Provider: "openai", Model: "gpt-4o", Temperature: &temperature},
		client,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.reqs) == 0 {
		t.Fatal("expected at least one chat request")
	}
	if client.reqs[0].Options.Temperature == nil {
		t.Fatal("temperature = nil, want explicit value")
	}
	if got := *client.reqs[0].Options.Temperature; got != 0 {
		t.Fatalf("temperature = %v, want 0", got)
	}
}

func TestAssistantToolReplayMessagePreservesToolCallsAndContent(t *testing.T) {
	toolCalls := []uniai.ToolCall{
		{
			ID:               "call_2|ts:encoded",
			Type:             "function",
			ThoughtSignature: "sig_xyz",
			Function: uniai.ToolCallFunction{
				Name:      "read_file",
				Arguments: `{"path":"main.go"}`,
			},
		},
	}
	msg := assistantToolReplayMessage("inspect", toolCalls)
	if msg.Role != uniai.RoleAssistant {
		t.Fatalf("message role = %q, want %q", msg.Role, uniai.RoleAssistant)
	}
	if msg.Content != "inspect" {
		t.Fatalf("message content = %q, want %q", msg.Content, "inspect")
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(msg.ToolCalls))
	}
	if got := msg.ToolCalls[0].ThoughtSignature; got != "sig_xyz" {
		t.Fatalf("thought signature = %q, want %q", got, "sig_xyz")
	}
}

func TestReplayToolCallsForProviderBackfillsGeminiBatchSignatures(t *testing.T) {
	toolCalls := replayToolCallsForProvider("gemini", []uniai.ToolCall{
		{
			ID:               "call_1",
			Type:             "function",
			ThoughtSignature: "sig_batch",
			Function: uniai.ToolCallFunction{
				Name:      "read_file",
				Arguments: `{"path":"main.go"}`,
			},
		},
		{
			ID:   "call_2",
			Type: "function",
			Function: uniai.ToolCallFunction{
				Name:      "read_file",
				Arguments: `{"path":"utils.go"}`,
			},
		},
	})
	if len(toolCalls) != 2 {
		t.Fatalf("tool calls = %d, want 2", len(toolCalls))
	}
	if got := toolCalls[1].ThoughtSignature; got != "sig_batch" {
		t.Fatalf("second thought signature = %q, want %q", got, "sig_batch")
	}
}

func TestReplayToolCallsForProviderDecodesGeminiSignatureFromID(t *testing.T) {
	encodedID := "call_2|ts:" + base64.RawURLEncoding.EncodeToString([]byte("sig_xyz"))
	toolCalls := replayToolCallsForProvider("gemini", []uniai.ToolCall{
		{
			ID:   encodedID,
			Type: "function",
			Function: uniai.ToolCallFunction{
				Name:      "read_file",
				Arguments: `{"path":"main.go"}`,
			},
		},
	})
	if len(toolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(toolCalls))
	}
	if got := toolCalls[0].ThoughtSignature; got != "sig_xyz" {
		t.Fatalf("thought signature = %q, want %q", got, "sig_xyz")
	}
}

func TestToolResultMessageForProviderWrapsGeminiListDirAsObject(t *testing.T) {
	msg, err := toolResultMessageForProvider("gemini", uniai.ToolCall{
		ID: "call_2",
		Function: uniai.ToolCallFunction{
			Name: "list_dir",
		},
	}, toolResponse{
		result:      []string{"main.go", "utils"},
		modelOutput: `["main.go","utils"]`,
	})
	if err != nil {
		t.Fatalf("tool result message: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(msg.Content), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	entries, ok := payload["entries"].([]any)
	if !ok {
		t.Fatalf("entries = %#v, want array", payload["entries"])
	}
	if len(entries) != 2 || entries[0] != "main.go" || entries[1] != "utils" {
		t.Fatalf("entries = %#v, want [main.go utils]", entries)
	}
}

func TestToolResultMessageForProviderKeepsOpenAIListDirPayload(t *testing.T) {
	msg, err := toolResultMessageForProvider("openai", uniai.ToolCall{
		ID: "call_2",
		Function: uniai.ToolCallFunction{
			Name: "list_dir",
		},
	}, toolResponse{
		result:      []string{"main.go", "utils"},
		modelOutput: `["main.go","utils"]`,
	})
	if err != nil {
		t.Fatalf("tool result message: %v", err)
	}
	if msg.Content != `["main.go","utils"]` {
		t.Fatalf("payload = %q, want raw model output", msg.Content)
	}
}

func TestRunWithClientCarriesGeminiThoughtSignatureIntoNextRound(t *testing.T) {
	caseDef := loadGoProcessDocumentsCaseForTest(t)
	client := &scriptedRecordingChatClient{
		results: []*uniai.ChatResult{
			{
				Text: "inspect",
				ToolCalls: []uniai.ToolCall{
					{
						ID:               "call_2|ts:encoded",
						Type:             "function",
						ThoughtSignature: "sig_xyz",
						Function: uniai.ToolCallFunction{
							Name:      "read_file",
							Arguments: `{"path":"main.go"}`,
						},
					},
				},
			},
			{Text: "done"},
		},
	}
	runner := NewRunner()

	result, err := runner.RunWithClient(
		context.Background(),
		caseDef,
		"gemini_profile",
		config.Profile{Provider: "gemini", Model: "gemini-3.1-pro-preview"},
		client,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error %q", result.Error)
	}
	if len(client.reqs) != 2 {
		t.Fatalf("requests = %d, want 2", len(client.reqs))
	}
	if len(client.reqs[1].Messages) < 3 {
		t.Fatalf("second request messages = %d, want at least 3", len(client.reqs[1].Messages))
	}
	got := client.reqs[1].Messages[1].ToolCalls[0].ThoughtSignature
	if got != "sig_xyz" {
		t.Fatalf("thought signature = %q, want %q", got, "sig_xyz")
	}
}

func TestRunWithClientBackfillsGeminiParallelToolCallSignaturesIntoNextRound(t *testing.T) {
	caseDef := loadGoProcessDocumentsCaseForTest(t)
	client := &scriptedRecordingChatClient{
		results: []*uniai.ChatResult{
			{
				Text: "inspect",
				ToolCalls: []uniai.ToolCall{
					{
						ID:               "call_1",
						Type:             "function",
						ThoughtSignature: "sig_batch",
						Function: uniai.ToolCallFunction{
							Name:      "read_file",
							Arguments: `{"path":"main.go"}`,
						},
					},
					{
						ID:   "call_2",
						Type: "function",
						Function: uniai.ToolCallFunction{
							Name:      "read_file",
							Arguments: `{"path":"utils/utils.go"}`,
						},
					},
				},
			},
			{Text: "done"},
		},
	}
	runner := NewRunner()

	result, err := runner.RunWithClient(
		context.Background(),
		caseDef,
		"gemini_profile",
		config.Profile{Provider: "gemini", Model: "gemini-3.1-pro-preview"},
		client,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error %q", result.Error)
	}
	if len(client.reqs) != 2 {
		t.Fatalf("requests = %d, want 2", len(client.reqs))
	}
	if len(client.reqs[1].Messages) < 4 {
		t.Fatalf("second request messages = %d, want at least 4", len(client.reqs[1].Messages))
	}
	got := client.reqs[1].Messages[1].ToolCalls[1].ThoughtSignature
	if got != "sig_batch" {
		t.Fatalf("second tool call thought signature = %q, want %q", got, "sig_batch")
	}
}

func TestRunWithClientWrapsGeminiToolResultPayloadAsObject(t *testing.T) {
	caseDef := loadGoProcessDocumentsCaseForTest(t)
	client := &scriptedRecordingChatClient{
		results: []*uniai.ChatResult{
			{
				Text: "inspect",
				ToolCalls: []uniai.ToolCall{
					{
						ID:               "call_2",
						Type:             "function",
						ThoughtSignature: "sig_xyz",
						Function: uniai.ToolCallFunction{
							Name:      "list_dir",
							Arguments: `{"path":"."}`,
						},
					},
				},
			},
			{Text: "done"},
		},
	}
	runner := NewRunner()

	result, err := runner.RunWithClient(
		context.Background(),
		caseDef,
		"gemini_profile",
		config.Profile{Provider: "gemini", Model: "gemini-3.1-pro-preview"},
		client,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error %q", result.Error)
	}
	if len(client.reqs) != 2 {
		t.Fatalf("requests = %d, want 2", len(client.reqs))
	}
	if len(client.reqs[1].Messages) < 3 {
		t.Fatalf("second request messages = %d, want at least 3", len(client.reqs[1].Messages))
	}
	if got := client.reqs[1].Messages[2].Content; got != `{"entries":["main.go","utils","vendor"]}` {
		t.Fatalf("tool result content = %q, want wrapped entries object", got)
	}
}
