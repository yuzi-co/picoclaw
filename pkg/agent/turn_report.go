// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
)

// A turnReport collects, per chat, what a client needs to know when the
// agent has finished with the messages it sent: which inbound messages were
// handled, how the turn ended and the token usage of all its LLM calls. A
// worker opens the report when it starts on an inbound message and closes it
// once the turn and the queued steering after it are done; closing publishes
// a channels.TurnDone notice, which channels implementing
// channels.TurnDoneNotifier deliver after the turn's replies (pico sends
// turn.done).
type turnReport struct {
	mu         sync.Mutex
	requestIDs []string
	usage      channels.TurnUsage
	failed     bool
	canceled   bool
}

func turnReportKey(channel, chatID string) string {
	return channel + "\x00" + chatID
}

// inboundTurnIDs returns the channel, chat and platform message ID of msg,
// from the convenience mirrors or, when those are empty, from Context.
func inboundTurnIDs(msg bus.InboundMessage) (channel, chatID, messageID string) {
	channel, chatID, messageID = msg.Channel, msg.ChatID, msg.MessageID
	if channel == "" {
		channel = msg.Context.Channel
	}
	if chatID == "" {
		chatID = msg.Context.ChatID
	}
	if messageID == "" {
		messageID = msg.Context.MessageID
	}
	return channel, chatID, messageID
}

// turnDoneSupported reports whether the chat's channel delivers turn
// completion notices; for other channels no report is kept.
func (al *AgentLoop) turnDoneSupported(channel string) bool {
	if al.channelManager == nil || strings.TrimSpace(channel) == "" {
		return false
	}
	ch, ok := al.channelManager.GetChannel(channel)
	if !ok {
		return false
	}
	_, ok = ch.(channels.TurnDoneNotifier)
	return ok
}

func (al *AgentLoop) turnReportFor(channel, chatID string, create bool) *turnReport {
	key := turnReportKey(channel, chatID)
	if v, ok := al.turnReports.Load(key); ok {
		return v.(*turnReport)
	}
	if !create || !al.turnDoneSupported(channel) {
		return nil
	}
	v, _ := al.turnReports.LoadOrStore(key, &turnReport{})
	return v.(*turnReport)
}

func (r *turnReport) addRequest(messageID string) {
	if r == nil || strings.TrimSpace(messageID) == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range r.requestIDs {
		if id == messageID {
			return
		}
	}
	r.requestIDs = append(r.requestIDs, messageID)
}

// noteTurnRequest records an inbound message that the chat's current turn
// handles: the one that starts it, or one folded into it as steering.
func (al *AgentLoop) noteTurnRequest(msg bus.InboundMessage) {
	channel, chatID, messageID := inboundTurnIDs(msg)
	if r := al.turnReportFor(channel, chatID, true); r != nil {
		r.addRequest(messageID)
	}
}

// noteTurnUsage adds the usage of one LLM call to the chat's open report.
func (al *AgentLoop) noteTurnUsage(ts *turnState, usage *providers.UsageInfo) {
	if ts == nil || usage == nil {
		return
	}
	r := al.turnReportFor(ts.channel, ts.chatID, false)
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.usage.InputTokens += usage.PromptTokens
	r.usage.OutputTokens += usage.CompletionTokens
	total := usage.TotalTokens
	if total == 0 {
		total = usage.PromptTokens + usage.CompletionTokens
	}
	r.usage.TotalTokens += total
	r.usage.LLMCalls++
}

// noteTurnEnd records how one agent turn of the chat's open report ended.
func (al *AgentLoop) noteTurnEnd(ts *turnState, status TurnEndStatus) {
	if ts == nil {
		return
	}
	r := al.turnReportFor(ts.channel, ts.chatID, false)
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	switch status {
	case TurnEndStatusError:
		r.failed = true
	case TurnEndStatusAborted:
		r.canceled = true
	}
}

// noteTurnFailed marks the chat's open report as failed (a failure notice
// was sent to the user).
func (al *AgentLoop) noteTurnFailed(channel, chatID string) {
	r := al.turnReportFor(channel, chatID, false)
	if r == nil {
		return
	}
	r.mu.Lock()
	r.failed = true
	r.mu.Unlock()
}

// turnUsageSnapshot returns the usage collected so far for the chat, or nil.
func (al *AgentLoop) turnUsageSnapshot(channel, chatID string) *channels.TurnUsage {
	r := al.turnReportFor(channel, chatID, false)
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.usage.LLMCalls == 0 {
		return nil
	}
	usage := r.usage
	return &usage
}

// attachTurnUsage puts the turn's usage so far on a final reply.
func (al *AgentLoop) attachTurnUsage(msg *bus.OutboundMessage) {
	usage := al.turnUsageSnapshot(msg.Context.Channel, msg.Context.ChatID)
	if usage == nil {
		return
	}
	data, err := json.Marshal(usage)
	if err != nil {
		return
	}
	if msg.Context.Raw == nil {
		msg.Context.Raw = make(map[string]string, 1)
	}
	msg.Context.Raw[channels.RawKeyTurnUsage] = string(data)
}

// beginTurnReport opens the chat's report for an inbound message a worker is
// about to handle. Steering messages recorded since the last report closed
// stay in it.
func (al *AgentLoop) beginTurnReport(msg bus.InboundMessage) {
	al.noteTurnRequest(msg)
}

// finishTurnReport closes the chat's report and publishes the turn
// completion notice. err is the context error, if any, when the worker
// stopped.
func (al *AgentLoop) finishTurnReport(ctx context.Context, msg bus.InboundMessage, err error) {
	channel, chatID, messageID := inboundTurnIDs(msg)
	key := turnReportKey(channel, chatID)
	v, ok := al.turnReports.LoadAndDelete(key)
	if !ok {
		return
	}
	r := v.(*turnReport)
	r.mu.Lock()
	done := channels.TurnDone{
		RequestIDs: append([]string(nil), r.requestIDs...),
		Status:     channels.TurnDoneStatusOK,
	}
	switch {
	case r.canceled || errors.Is(err, context.Canceled):
		done.Status = channels.TurnDoneStatusCanceled
	case r.failed || err != nil:
		done.Status = channels.TurnDoneStatusError
	}
	if r.usage.LLMCalls > 0 {
		usage := r.usage
		done.Usage = &usage
	}
	r.mu.Unlock()
	if messageID != "" {
		done.RequestID = messageID
	} else if len(done.RequestIDs) > 0 {
		done.RequestID = done.RequestIDs[0]
	}

	data, marshalErr := json.Marshal(done)
	if marshalErr != nil {
		return
	}
	out := bus.OutboundMessage{
		Context:    bus.NewOutboundContext(channel, chatID, ""),
		SessionKey: msg.SessionKey,
	}
	out.Context.Raw = map[string]string{
		channels.RawKeyOutboundKind: channels.OutboundKindTurnDone,
		channels.RawKeyTurnDone:     string(data),
	}
	// The worker may be stopping because ctx was canceled; the notice still
	// has to reach the bus.
	pubCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if pubErr := al.bus.PublishOutbound(pubCtx, out); pubErr != nil {
		logger.WarnCF("agent", "Failed to publish turn completion notice", map[string]any{
			"channel": channel,
			"chat_id": chatID,
			"error":   pubErr.Error(),
		})
	}
}

// turnUsageTokens returns the input and output tokens to show on a streamed
// final reply: the turn's usage so far when the chat has an open report,
// else the usage of the last LLM call.
func (p *streamingChunkPublisher) turnUsageTokens() (int, int, bool) {
	if p.al != nil {
		if usage := p.al.turnUsageSnapshot(p.channel, p.chatID); usage != nil {
			return usage.InputTokens, usage.OutputTokens, true
		}
	}
	if p.ts != nil {
		if usage := p.ts.GetLastUsage(); usage != nil {
			return usage.PromptTokens, usage.CompletionTokens, true
		}
	}
	return 0, 0, false
}
