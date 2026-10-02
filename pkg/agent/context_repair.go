// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT

package agent

import (
	"strings"

	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
)

// missingToolResultContent stands in for a tool result that never reached the
// history, typically because the turn was interrupted between the tool call
// and its result.
const missingToolResultContent = "[Tool result missing: the call was interrupted before its result " +
	"was recorded. Its effect is unknown; check the current state before retrying.]"

// repairToolCallHistory fixes two malformations in stored history before
// sanitizeHistoryForProvider validates it (idea from adreamNyx/picoclaw
// 1bfd52d2):
//
//   - An assistant turn with no text, no tool calls and no media (an empty
//     reply that was persisted) is dropped. Strict endpoints reject empty
//     assistant content, and a placeholder text would only teach a small
//     model to repeat it.
//   - An assistant tool-call turn whose results are only partly present gets
//     a placeholder result for each missing call, right after the results it
//     has. sanitizeHistoryForProvider would otherwise drop the whole block,
//     including the assistant's text and the results that did arrive.
//
// The output is deterministic (placeholders follow the tool-call order), so
// a provider's prompt cache sees the same bytes on every replay. The input is
// not mutated.
func repairToolCallHistory(history []providers.Message) []providers.Message {
	if len(history) == 0 {
		return history
	}
	repaired := make([]providers.Message, 0, len(history))
	changed := false
	for i := 0; i < len(history); i++ {
		msg := history[i]
		if msg.Role != "assistant" {
			repaired = append(repaired, msg)
			continue
		}
		if len(msg.ToolCalls) == 0 {
			if strings.TrimSpace(msg.Content) == "" && len(msg.Media) == 0 && len(msg.Attachments) == 0 {
				logger.DebugCF("agent", "Dropping empty assistant turn from history", map[string]any{})
				changed = true
				continue
			}
			repaired = append(repaired, msg)
			continue
		}

		repaired = append(repaired, msg)
		found := make(map[string]bool, len(msg.ToolCalls))
		j := i + 1
		for ; j < len(history) && history[j].Role == "tool"; j++ {
			found[history[j].ToolCallID] = true
			repaired = append(repaired, history[j])
		}
		for _, tc := range msg.ToolCalls {
			// A call without an ID cannot be answered; sanitizeHistoryForProvider
			// drops that turn.
			if tc.ID == "" || found[tc.ID] {
				continue
			}
			logger.InfoCF("agent", "Adding placeholder for missing tool result", map[string]any{
				"tool_call_id": tc.ID,
			})
			repaired = append(repaired, providers.Message{
				Role:       "tool",
				Content:    missingToolResultContent,
				ToolCallID: tc.ID,
			})
			found[tc.ID] = true
			changed = true
		}
		i = j - 1
	}
	if !changed {
		return history
	}
	return repaired
}
