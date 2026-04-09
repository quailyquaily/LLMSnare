package benchmark

import (
	"context"
	"fmt"
	"testing"

	"llmsnare/internal/config"

	"github.com/quailyquaily/uniai"
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
		config.Profile{Driver: "openai", Model: "gpt-4o"},
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
	if events[len(events)-1].RawScore != result.RawScore {
		t.Fatalf("final raw score = %d, want %d", events[len(events)-1].RawScore, result.RawScore)
	}
	if events[len(events)-1].NormalizedScore != result.NormalizedScore {
		t.Fatalf("final normalized score = %v, want %v", events[len(events)-1].NormalizedScore, result.NormalizedScore)
	}
}
