package channels

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// Turn completion travels from the agent to the channel as an outbound
// message marked with OutboundKindTurnDone in Context.Raw, so the channel
// worker delivers it after every reply queued before it. Channels that do not
// implement TurnDoneNotifier never see it.
const (
	// RawKeyOutboundKind is the Context.Raw key that classifies an outbound
	// message ("final" for a turn's reply).
	RawKeyOutboundKind = "outbound_kind"
	// OutboundKindTurnDone marks the turn completion notice.
	OutboundKindTurnDone = "turn_done"
	// RawKeyTurnDone carries the TurnDone value as JSON.
	RawKeyTurnDone = "turn_done"
	// RawKeyTurnUsage carries the turn's TurnUsage as JSON on a final reply.
	RawKeyTurnUsage = "turn_usage"
)

// Turn completion statuses.
const (
	TurnDoneStatusOK       = "ok"
	TurnDoneStatusError    = "error"
	TurnDoneStatusCanceled = "canceled"
)

// TurnUsage is the token usage of every LLM call made for a turn.
type TurnUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
	LLMCalls     int `json:"llm_calls"`
}

// TurnDone describes a finished turn: the agent will send nothing more for
// the inbound messages listed in RequestIDs.
type TurnDone struct {
	// RequestID is the platform message ID of the message that started the
	// turn.
	RequestID string `json:"request_id,omitempty"`
	// RequestIDs lists every inbound message the turn handled, RequestID
	// first, then messages that arrived while it ran and were folded into it.
	RequestIDs []string `json:"request_ids,omitempty"`
	// Status is TurnDoneStatusOK, TurnDoneStatusError or
	// TurnDoneStatusCanceled.
	Status string `json:"status"`
	// Usage is nil when the turn made no LLM call (a command, for example).
	Usage *TurnUsage `json:"usage,omitempty"`
}

// TurnDoneNotifier is implemented by channels that tell their clients when a
// turn is complete.
type TurnDoneNotifier interface {
	SendTurnDone(ctx context.Context, chatID string, done TurnDone) error
}

// OutboundMessageIsTurnDone reports whether msg is a turn completion notice.
func OutboundMessageIsTurnDone(msg bus.OutboundMessage) bool {
	return len(msg.Context.Raw) > 0 &&
		strings.EqualFold(strings.TrimSpace(msg.Context.Raw[RawKeyOutboundKind]), OutboundKindTurnDone)
}

// TurnDoneFromOutbound decodes the notice carried by msg.
func TurnDoneFromOutbound(msg bus.OutboundMessage) (TurnDone, bool) {
	var done TurnDone
	raw := msg.Context.Raw[RawKeyTurnDone]
	if raw == "" || json.Unmarshal([]byte(raw), &done) != nil {
		return TurnDone{}, false
	}
	return done, true
}

// TurnUsageFromOutbound decodes the turn usage attached to a final reply.
func TurnUsageFromOutbound(msg bus.OutboundMessage) (*TurnUsage, bool) {
	raw := msg.Context.Raw[RawKeyTurnUsage]
	if raw == "" {
		return nil, false
	}
	var usage TurnUsage
	if json.Unmarshal([]byte(raw), &usage) != nil {
		return nil, false
	}
	return &usage, true
}

// deliverTurnDone stops the chat's typing indicator, so a client sees
// typing.stop before the notice, and hands the notice to the channel.
func (m *Manager) deliverTurnDone(ctx context.Context, name string, ch Channel, msg bus.OutboundMessage) {
	notifier, ok := ch.(TurnDoneNotifier)
	if !ok {
		return
	}
	done, ok := TurnDoneFromOutbound(msg)
	if !ok {
		logger.WarnCF("channels", "Dropping malformed turn completion notice", map[string]any{
			"channel": name,
		})
		return
	}
	chatID := outboundMessageChatID(msg)
	m.InvokeTypingStop(name, chatID)
	if err := notifier.SendTurnDone(ctx, chatID, done); err != nil {
		logger.WarnCF("channels", "Failed to send turn completion notice", map[string]any{
			"channel": name,
			"chat_id": chatID,
			"error":   err.Error(),
		})
	}
}
