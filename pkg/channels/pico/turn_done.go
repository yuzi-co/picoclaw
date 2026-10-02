package pico

import (
	"context"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
)

// SendTurnDone implements channels.TurnDoneNotifier: it sends turn.done to
// every connection of the session once the agent has finished a turn. The
// channel worker sends it after the turn's replies and after typing.stop.
func (c *PicoChannel) SendTurnDone(ctx context.Context, chatID string, done channels.TurnDone) error {
	if !c.IsRunning() {
		return channels.ErrNotRunning
	}
	payload := map[string]any{
		"status": done.Status,
	}
	if done.RequestID != "" {
		payload["request_id"] = done.RequestID
	}
	if len(done.RequestIDs) > 0 {
		payload["request_ids"] = done.RequestIDs
	}
	if done.Usage != nil {
		payload[PayloadKeyUsage] = turnUsagePayload(done.Usage)
	}
	return c.broadcast(chatID, newMessage(TypeTurnDone, payload))
}

func turnUsagePayload(u *channels.TurnUsage) map[string]any {
	return map[string]any{
		"input_tokens":  u.InputTokens,
		"output_tokens": u.OutputTokens,
		"total_tokens":  u.TotalTokens,
		"llm_calls":     u.LLMCalls,
	}
}

// setFinalTurnUsagePayload adds the usage the agent attached to a turn's
// final reply. A streamed reply gets it from the streamer instead.
func setFinalTurnUsagePayload(payload map[string]any, msg bus.OutboundMessage) {
	if usage, ok := channels.TurnUsageFromOutbound(msg); ok {
		payload[PayloadKeyUsage] = turnUsagePayload(usage)
	}
}
