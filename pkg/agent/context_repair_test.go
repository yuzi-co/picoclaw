package agent

import (
	"slices"
	"testing"

	"github.com/sipeed/picoclaw/pkg/providers"
)

func TestRepairToolCallHistory_DropsEmptyAssistantTurns(t *testing.T) {
	history := []providers.Message{
		msg("user", "hi"),
		msg("assistant", "  "),
		msg("user", "hello?"),
		msg("assistant", "hello"),
	}
	got := sanitizeHistoryForProvider(history)
	assertRoles(t, got, "user", "user", "assistant")
}

func TestRepairToolCallHistory_KeepsAssistantWithMedia(t *testing.T) {
	history := []providers.Message{
		msg("user", "draw"),
		{Role: "assistant", Media: []string{"media://x"}},
	}
	got := repairToolCallHistory(history)
	if len(got) != 2 {
		t.Fatalf("got %v, want the media-only assistant turn kept", roles(got))
	}
}

func TestRepairToolCallHistory_PlaceholderOrderFollowsToolCalls(t *testing.T) {
	history := []providers.Message{
		msg("user", "do three things"),
		assistantWithTools("C", "A", "B"),
		toolResult("A"),
		msg("user", "next"),
	}
	got := repairToolCallHistory(history)
	ids := []string{}
	for _, m := range got {
		if m.Role == "tool" {
			ids = append(ids, m.ToolCallID)
		}
	}
	if !slices.Equal(ids, []string{"A", "C", "B"}) {
		t.Fatalf("tool result order = %v, want [A C B]", ids)
	}
	// The input must not change.
	if len(history) != 4 {
		t.Fatalf("input mutated: %v", roles(history))
	}
}

func TestRepairToolCallHistory_CompleteHistoryUnchanged(t *testing.T) {
	history := []providers.Message{
		msg("user", "q"),
		assistantWithTools("A"),
		toolResult("A"),
		msg("assistant", "a"),
	}
	got := repairToolCallHistory(history)
	if &got[0] != &history[0] {
		t.Fatal("complete history was copied, want it returned as is")
	}
}
