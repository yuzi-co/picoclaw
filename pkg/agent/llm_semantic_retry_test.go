package agent

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
)

func TestSemanticRetryError(t *testing.T) {
	on := config.AgentDefaults{LLMRetryOnEmptyContent: true, LLMRetryOnFinishReasons: []string{"length"}}
	tests := []struct {
		name      string
		defaults  config.AgentDefaults
		resp      *providers.LLMResponse
		published bool
		want      string
	}{
		{"nil response", on, nil, false, ""},
		{"empty", on, &providers.LLMResponse{Content: "  \n"}, false, "empty_response"},
		{"thinking only", on, &providers.LLMResponse{ReasoningContent: "hmm", FinishReason: "stop"}, false, "empty_response"},
		{"empty with tool calls", on, &providers.LLMResponse{ToolCalls: []providers.ToolCall{{ID: "a", Name: "t"}}}, false, ""},
		{"truncated", on, &providers.LLMResponse{Content: "half an ans", FinishReason: "length"}, false, "finish_reason"},
		{"truncated, case differs", on, &providers.LLMResponse{Content: "x", FinishReason: "LENGTH"}, false, "finish_reason"},
		{"truncated, normalized name", on, &providers.LLMResponse{Content: "x", FinishReason: "truncated"}, false, "finish_reason"},
		{"truncated but already streamed", on, &providers.LLMResponse{Content: "half", FinishReason: "length"}, true, ""},
		{"complete", on, &providers.LLMResponse{Content: "answer", FinishReason: "stop"}, false, ""},
		{"empty, retry off", config.AgentDefaults{}, &providers.LLMResponse{}, false, ""},
		{"truncated, no reasons", config.AgentDefaults{}, &providers.LLMResponse{Content: "x", FinishReason: "length"}, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var publisher *streamingChunkPublisher
			if tt.published {
				publisher = &streamingChunkPublisher{published: true}
			}
			err := semanticRetryError(tt.defaults, tt.resp, publisher)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, errSemanticRetry) {
				t.Fatalf("err = %v, want errSemanticRetry", err)
			}
			reason, transient := transientLLMRetryReason(err)
			if !transient || reason != tt.want {
				t.Fatalf("transientLLMRetryReason = %q, %v; want %q, true", reason, transient, tt.want)
			}
		})
	}
}

// scriptedProvider returns the given responses in order, then repeats the
// last one.
type scriptedProvider struct {
	responses []*providers.LLMResponse
	mu        sync.Mutex
	calls     int
}

func (p *scriptedProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *scriptedProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	p.mu.Lock()
	i := p.calls
	if i >= len(p.responses) {
		i = len(p.responses) - 1
	}
	p.calls++
	p.mu.Unlock()
	resp := *p.responses[i]
	return &resp, nil
}

func (p *scriptedProvider) GetDefaultModel() string { return "test-model" }

func runScriptedTurn(t *testing.T, defaults config.AgentDefaults, provider *scriptedProvider) string {
	t.Helper()
	defaults.Workspace = t.TempDir()
	defaults.ModelName = "test-model"
	defaults.MaxTokens = 4096
	defaults.MaxToolIterations = 5
	defaults.LLMRetryBackoffSecs = 1
	cfg := &config.Config{
		Agents: config.AgentsConfig{Defaults: defaults},
		ModelList: []*config.ModelConfig{
			{ModelName: "test-model", Model: "openai/test-model"},
		},
	}
	al := NewAgentLoop(cfg, bus.NewMessageBus(), provider)
	ctx, cancel := context.WithTimeout(context.Background(), responseTimeout)
	defer cancel()
	resp, err := al.processMessage(ctx, testInboundMessage(bus.InboundMessage{
		Context: bus.InboundContext{
			Channel: "telegram", ChatID: "chat1", ChatType: "direct", SenderID: "user1", MessageID: "m1",
		},
		Content:    "hello",
		SessionKey: "agent:main:telegram:direct:user1",
	}))
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	return resp
}

func TestAgentLoop_EmptyReplyIsRetried(t *testing.T) {
	provider := &scriptedProvider{responses: []*providers.LLMResponse{
		{Content: "", ReasoningContent: "thinking only", FinishReason: "stop"},
		{Content: "real answer", FinishReason: "stop"},
	}}
	resp := runScriptedTurn(t, config.AgentDefaults{
		MaxLLMRetries:          2,
		LLMRetryOnEmptyContent: true,
	}, provider)
	if resp != "real answer" {
		t.Fatalf("response = %q, want %q", resp, "real answer")
	}
	if provider.calls != 2 {
		t.Fatalf("calls = %d, want 2", provider.calls)
	}
}

func TestAgentLoop_TruncatedReplyIsRetried(t *testing.T) {
	provider := &scriptedProvider{responses: []*providers.LLMResponse{
		{Content: "half an", FinishReason: "length"},
		{Content: "whole answer", FinishReason: "stop"},
	}}
	resp := runScriptedTurn(t, config.AgentDefaults{
		MaxLLMRetries:           2,
		LLMRetryOnFinishReasons: []string{"length"},
	}, provider)
	if resp != "whole answer" {
		t.Fatalf("response = %q, want %q", resp, "whole answer")
	}
}

// When every attempt is empty, the last reply is kept and the turn ends as
// it did before (no error, the default response).
func TestAgentLoop_EmptyReplyRetriesAreBounded(t *testing.T) {
	provider := &scriptedProvider{responses: []*providers.LLMResponse{{Content: ""}}}
	resp := runScriptedTurn(t, config.AgentDefaults{
		MaxLLMRetries:          1,
		LLMRetryOnEmptyContent: true,
	}, provider)
	if provider.calls != 2 {
		t.Fatalf("calls = %d, want 2 (one try and one retry)", provider.calls)
	}
	if resp != defaultResponse {
		t.Fatalf("response = %q, want the default response", resp)
	}
}

func TestDefaultConfig_RetriesEmptyAndTruncatedReplies(t *testing.T) {
	d := config.DefaultConfig().Agents.Defaults
	if !d.LLMRetryOnEmptyContent {
		t.Error("llm_retry_on_empty_content = false, want true")
	}
	if len(d.LLMRetryOnFinishReasons) != 1 || d.LLMRetryOnFinishReasons[0] != "length" {
		t.Errorf("llm_retry_on_finish_reasons = %v, want [length]", d.LLMRetryOnFinishReasons)
	}
}
