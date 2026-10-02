package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/channels/pico"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
)

// picoTurnHarness runs an AgentLoop behind a real pico channel and channel
// manager, with a WebSocket client connected to the channel.
type picoTurnHarness struct {
	t    *testing.T
	conn *websocket.Conn
	msgs chan pico.PicoMessage
}

func newPicoTurnHarness(t *testing.T, provider providers.LLMProvider) *picoTurnHarness {
	t.Helper()
	msgBus := bus.NewMessageBus()
	cfg := &config.Config{
		Agents: config.AgentsConfig{Defaults: config.AgentDefaults{
			Workspace:         t.TempDir(),
			ModelName:         "test-model",
			MaxTokens:         4096,
			MaxToolIterations: 5,
		}},
		ModelList: []*config.ModelConfig{{ModelName: "test-model", Model: "openai/test-model"}},
	}

	bc := &config.Channel{Type: config.ChannelPico, Enabled: true}
	picoCfg := &config.PicoSettings{}
	picoCfg.SetToken("test-token")
	ch, err := pico.NewPicoChannel(bc, picoCfg, msgBus)
	if err != nil {
		t.Fatal(err)
	}
	cm := newStartedTestChannelManager(t, msgBus, nil, "pico", ch)

	al := NewAgentLoop(cfg, msgBus, provider)
	al.SetChannelManager(cm)
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		_ = al.Run(ctx)
	}()

	server := httptest.NewServer(ch)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/pico/ws?session_id=s1"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, http.Header{"Authorization": []string{"Bearer test-token"}})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	resp.Body.Close()

	h := &picoTurnHarness{t: t, conn: conn, msgs: make(chan pico.PicoMessage, 64)}
	go func() {
		for {
			var msg pico.PicoMessage
			if err := conn.ReadJSON(&msg); err != nil {
				close(h.msgs)
				return
			}
			h.msgs <- msg
		}
	}()
	t.Cleanup(func() {
		conn.Close()
		server.Close()
		cancel()
		<-runDone
		al.Close()
	})
	return h
}

func (h *picoTurnHarness) send(id, content string) {
	h.t.Helper()
	if err := h.conn.WriteJSON(pico.PicoMessage{
		Type: pico.TypeMessageSend, ID: id, Payload: map[string]any{"content": content},
	}); err != nil {
		h.t.Fatal(err)
	}
}

// untilTurnDone returns every message up to and including turn.done.
func (h *picoTurnHarness) untilTurnDone() []pico.PicoMessage {
	h.t.Helper()
	var got []pico.PicoMessage
	timeout := time.After(10 * time.Second)
	for {
		select {
		case msg, ok := <-h.msgs:
			if !ok {
				h.t.Fatalf("connection closed; got %v", types(got))
			}
			got = append(got, msg)
			if msg.Type == pico.TypeTurnDone {
				return got
			}
		case <-timeout:
			h.t.Fatalf("no turn.done; got %v", types(got))
		}
	}
}

func types(msgs []pico.PicoMessage) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.Type
	}
	return out
}

func lastOfType(msgs []pico.PicoMessage, typ string) (int, pico.PicoMessage) {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Type == typ {
			return i, msgs[i]
		}
	}
	return -1, pico.PicoMessage{}
}

func TestPicoTurnDone_AfterReplyWithUsage(t *testing.T) {
	provider := &scriptedProvider{responses: []*providers.LLMResponse{{
		Content:      "hello there",
		FinishReason: "stop",
		Usage:        &providers.UsageInfo{PromptTokens: 120, CompletionTokens: 8, TotalTokens: 128},
	}}}
	h := newPicoTurnHarness(t, provider)
	h.send("req-1", "hi")
	msgs := h.untilTurnDone()

	replyIdx, reply := -1, pico.PicoMessage{}
	for i, m := range msgs {
		if m.Type == pico.TypeMessageCreate && m.Payload["content"] == "hello there" {
			replyIdx, reply = i, m
		}
	}
	if replyIdx < 0 {
		t.Fatalf("no reply in %v", types(msgs))
	}
	usage, ok := reply.Payload["usage"].(map[string]any)
	if !ok || usage["input_tokens"] != float64(120) || usage["output_tokens"] != float64(8) ||
		usage["total_tokens"] != float64(128) || usage["llm_calls"] != float64(1) {
		t.Fatalf("reply usage = %v", reply.Payload["usage"])
	}
	stopIdx, _ := lastOfType(msgs, pico.TypeTypingStop)
	doneIdx, done := lastOfType(msgs, pico.TypeTurnDone)
	if stopIdx > doneIdx || replyIdx > doneIdx {
		t.Fatalf("order = %v, want reply and typing.stop before turn.done", types(msgs))
	}
	if done.SessionID != "s1" || done.Payload["request_id"] != "req-1" || done.Payload["status"] != "ok" {
		t.Fatalf("turn.done = %+v", done)
	}
	if ids, _ := done.Payload["request_ids"].([]any); len(ids) != 1 || ids[0] != "req-1" {
		t.Fatalf("request_ids = %v", done.Payload["request_ids"])
	}
	if du, _ := done.Payload["usage"].(map[string]any); du["total_tokens"] != float64(128) || du["llm_calls"] != float64(1) {
		t.Fatalf("turn.done usage = %v", done.Payload["usage"])
	}
}

type failingProvider struct{}

func (failingProvider) Chat(context.Context, []providers.Message, []providers.ToolDefinition, string, map[string]any) (*providers.LLMResponse, error) {
	return nil, errors.New("model exploded")
}
func (failingProvider) GetDefaultModel() string { return "test-model" }

func TestPicoTurnDone_ErrorStatus(t *testing.T) {
	h := newPicoTurnHarness(t, failingProvider{})
	h.send("req-err", "hi")
	msgs := h.untilTurnDone()
	_, done := lastOfType(msgs, pico.TypeTurnDone)
	if done.Payload["status"] != "error" || done.Payload["request_id"] != "req-err" {
		t.Fatalf("turn.done = %+v", done.Payload)
	}
	if _, ok := done.Payload["usage"]; ok {
		t.Fatalf("usage present without a successful LLM call: %v", done.Payload)
	}
	foundNotice := false
	for _, m := range msgs {
		if m.Type == pico.TypeMessageCreate {
			foundNotice = true
		}
	}
	if !foundNotice {
		t.Fatalf("no failure notice before turn.done: %v", types(msgs))
	}
}

// blockingProvider holds its first call until release is closed.
type blockingProvider struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
	mu      sync.Mutex
	calls   int
}

func (p *blockingProvider) Chat(ctx context.Context, _ []providers.Message, _ []providers.ToolDefinition, _ string, _ map[string]any) (*providers.LLMResponse, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	p.once.Do(func() {
		close(p.started)
		<-p.release
	})
	return &providers.LLMResponse{
		Content: "ok",
		Usage:   &providers.UsageInfo{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12},
	}, nil
}
func (p *blockingProvider) GetDefaultModel() string { return "test-model" }

func TestPicoTurnDone_ListsMessagesFoldedIntoTheTurn(t *testing.T) {
	provider := &blockingProvider{started: make(chan struct{}), release: make(chan struct{})}
	h := newPicoTurnHarness(t, provider)
	h.send("req-a", "first")
	select {
	case <-provider.started:
	case <-time.After(5 * time.Second):
		t.Fatal("first turn did not start")
	}
	h.send("req-b", "second, while the first runs")
	time.Sleep(200 * time.Millisecond) // let the main loop queue it as steering
	close(provider.release)

	msgs := h.untilTurnDone()
	_, done := lastOfType(msgs, pico.TypeTurnDone)
	ids, _ := done.Payload["request_ids"].([]any)
	if done.Payload["request_id"] != "req-a" || len(ids) != 2 || ids[0] != "req-a" || ids[1] != "req-b" {
		t.Fatalf("turn.done = %+v, want request req-a with request_ids [req-a req-b]", done.Payload)
	}
	usage, _ := done.Payload["usage"].(map[string]any)
	provider.mu.Lock()
	calls := provider.calls
	provider.mu.Unlock()
	if usage["llm_calls"] != float64(calls) || usage["total_tokens"] != float64(12*calls) {
		t.Fatalf("usage = %v after %d calls", usage, calls)
	}
	// Exactly one turn.done for both messages.
	select {
	case extra := <-h.msgs:
		if extra.Type == pico.TypeTurnDone {
			t.Fatalf("second turn.done: %+v", extra)
		}
	case <-time.After(300 * time.Millisecond):
	}
}

func TestTurnReport_NotKeptForChannelsWithoutTurnDone(t *testing.T) {
	al := &AgentLoop{channelManager: &recordingChannelManager{}}
	al.beginTurnReport(bus.InboundMessage{Channel: "telegram", ChatID: "c", MessageID: "m"})
	if al.turnReportFor("telegram", "c", false) != nil {
		t.Fatal("report kept for a channel that cannot deliver turn.done")
	}
	var _ channels.TurnDoneNotifier = (*pico.PicoChannel)(nil)
}
