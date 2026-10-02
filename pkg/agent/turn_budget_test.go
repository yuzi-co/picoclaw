package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/providers"
)

// =============================================================================
// Wall-clock turn budget tests
// =============================================================================

// alwaysToolCallProvider always returns a tool call so that, absent the budget,
// the turn would keep looping until MaxIterations.
type alwaysToolCallProvider struct {
	toolName string
	toolArgs map[string]any
	mu       sync.Mutex
	calls    int
}

func (p *alwaysToolCallProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	return &providers.LLMResponse{
		Content: "working",
		ToolCalls: []providers.ToolCall{
			{ID: "call_x", Name: p.toolName, Arguments: p.toolArgs},
		},
		FinishReason: "tool_calls",
	}, nil
}

func (p *alwaysToolCallProvider) GetDefaultModel() string { return "always-tool-model" }

func TestTurnState_MarkBudgetStopRequested(t *testing.T) {
	ts := &turnState{}
	if !ts.markBudgetStopRequested() {
		t.Fatal("first markBudgetStopRequested should return true")
	}
	if ts.markBudgetStopRequested() {
		t.Fatal("second markBudgetStopRequested should return false (dedupe)")
	}
}

func TestRunTurn_TurnTimeBudgetTriggersGracefulStop(t *testing.T) {
	provider := &alwaysToolCallProvider{
		toolName: "search",
		toolArgs: map[string]any{"q": "x"},
	}
	al, agent, cleanup := newTurnCoordTestLoop(t, provider)
	defer cleanup()

	// Keep the loop alive long enough that only the budget can stop it.
	agent.MaxIterations = 1000
	// A non-positive elapsed budget is always exceeded on the first iteration.
	agent.TurnTimeBudget = time.Nanosecond

	pipeline := NewPipeline(al)
	opts := makeTestProcessOpts("test-session-budget")

	ts := newTurnState(agent, opts, turnEventScope{
		turnID:  "turn-budget",
		context: newTurnContext(nil, nil, nil),
	})

	result, err := al.runTurn(context.Background(), ts, pipeline)
	if err != nil {
		t.Fatalf("runTurn failed: %v", err)
	}
	if result.status != TurnEndStatusCompleted {
		t.Errorf("expected status Completed, got %v", result.status)
	}
	if !ts.budgetStopRequested {
		t.Error("expected budgetStopRequested to be set")
	}
	if ts.currentIteration() >= agent.MaxIterations {
		t.Errorf("turn ran to MaxIterations (%d) instead of stopping on budget", agent.MaxIterations)
	}
}
