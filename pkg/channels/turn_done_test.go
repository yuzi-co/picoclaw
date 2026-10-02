package channels

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/sipeed/picoclaw/pkg/bus"
)

type turnDoneChannel struct {
	mockChannel
	mu     sync.Mutex
	events []string
	dones  []TurnDone
}

func (c *turnDoneChannel) SendTurnDone(_ context.Context, chatID string, done TurnDone) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, "turn_done:"+chatID)
	c.dones = append(c.dones, done)
	return nil
}

func (c *turnDoneChannel) record(event string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}

func turnDoneOutbound(chatID string, done TurnDone) bus.OutboundMessage {
	data, _ := json.Marshal(done)
	msg := testOutboundMessage(bus.OutboundMessage{Channel: "test", ChatID: chatID})
	msg.Context.Raw = map[string]string{
		RawKeyOutboundKind: OutboundKindTurnDone,
		RawKeyTurnDone:     string(data),
	}
	return msg
}

func TestRunWorker_TurnDoneFollowsQueuedRepliesAndTypingStop(t *testing.T) {
	m := newTestManager()
	ch := &turnDoneChannel{}
	ch.sendFn = func(_ context.Context, msg bus.OutboundMessage) error {
		ch.record("send:" + msg.Content)
		return nil
	}
	w := &channelWorker{
		ch:      ch,
		queue:   make(chan bus.OutboundMessage, 10),
		done:    make(chan struct{}),
		limiter: rate.NewLimiter(rate.Inf, 1),
	}
	m.RecordTypingStop("test", "chat-1", func() { ch.record("typing_stop") })

	w.queue <- testOutboundMessage(bus.OutboundMessage{Channel: "test", ChatID: "chat-1", Content: "reply"})
	w.queue <- turnDoneOutbound("chat-1", TurnDone{RequestID: "r1", Status: TurnDoneStatusOK})
	go m.runWorker(t.Context(), "test", w)

	deadline := time.Now().Add(time.Second)
	for {
		ch.mu.Lock()
		n := len(ch.dones)
		ch.mu.Unlock()
		if n == 1 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	ch.mu.Lock()
	defer ch.mu.Unlock()
	// The reply stops typing in preSend; turn.done must come after it.
	want := []string{"typing_stop", "send:reply", "turn_done:chat-1"}
	if len(ch.events) != len(want) {
		t.Fatalf("events = %v, want %v", ch.events, want)
	}
	for i := range want {
		if ch.events[i] != want[i] {
			t.Fatalf("events = %v, want %v", ch.events, want)
		}
	}
	if ch.dones[0].RequestID != "r1" || ch.dones[0].Status != TurnDoneStatusOK {
		t.Fatalf("done = %+v", ch.dones[0])
	}
}

func TestRunWorker_TurnDoneStopsTypingWithoutReply(t *testing.T) {
	m := newTestManager()
	ch := &turnDoneChannel{}
	w := &channelWorker{
		ch:      ch,
		queue:   make(chan bus.OutboundMessage, 10),
		done:    make(chan struct{}),
		limiter: rate.NewLimiter(rate.Inf, 1),
	}
	m.RecordTypingStop("test", "chat-1", func() { ch.record("typing_stop") })
	w.queue <- turnDoneOutbound("chat-1", TurnDone{Status: TurnDoneStatusError})
	go m.runWorker(t.Context(), "test", w)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		ch.mu.Lock()
		n := len(ch.events)
		ch.mu.Unlock()
		if n == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if len(ch.events) != 2 || ch.events[0] != "typing_stop" || ch.events[1] != "turn_done:chat-1" {
		t.Fatalf("events = %v, want typing_stop then turn_done", ch.events)
	}
}

func TestRunWorker_TurnDoneIgnoredByOtherChannels(t *testing.T) {
	m := newTestManager()
	var mu sync.Mutex
	var sent []bus.OutboundMessage
	ch := &mockChannel{sendFn: func(_ context.Context, msg bus.OutboundMessage) error {
		mu.Lock()
		sent = append(sent, msg)
		mu.Unlock()
		return nil
	}}
	w := &channelWorker{
		ch:      ch,
		queue:   make(chan bus.OutboundMessage, 10),
		done:    make(chan struct{}),
		limiter: rate.NewLimiter(rate.Inf, 1),
	}
	w.queue <- turnDoneOutbound("chat-1", TurnDone{Status: TurnDoneStatusOK})
	w.queue <- testOutboundMessage(bus.OutboundMessage{Channel: "test", ChatID: "chat-1", Content: "next"})
	go m.runWorker(t.Context(), "test", w)
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(sent) != 1 || sent[0].Content != "next" {
		t.Fatalf("sent = %+v, want only the next reply", sent)
	}
}
