package pico

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
)

func captureBroadcasts(ch *PicoChannel) <-chan PicoMessage {
	out := make(chan PicoMessage, 8)
	ch.broadcastFn = func(chatID string, msg PicoMessage) error {
		msg.SessionID = strings.TrimPrefix(chatID, "pico:")
		out <- msg
		return nil
	}
	return out
}

// roundTrip encodes msg as the server would and decodes it as a client does,
// so assertions see the wire JSON.
func roundTrip(t *testing.T, msg PicoMessage) map[string]any {
	t.Helper()
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	return wire
}

func TestSendTurnDone_WireFormat(t *testing.T) {
	ch := newTestPicoChannel(t)
	ch.SetRunning(true)
	got := captureBroadcasts(ch)

	err := ch.SendTurnDone(context.Background(), "pico:sess-1", channels.TurnDone{
		RequestID:  "req-1",
		RequestIDs: []string{"req-1", "req-2"},
		Status:     channels.TurnDoneStatusOK,
		Usage:      &channels.TurnUsage{InputTokens: 100, OutputTokens: 20, TotalTokens: 120, LLMCalls: 2},
	})
	if err != nil {
		t.Fatalf("SendTurnDone: %v", err)
	}
	wire := roundTrip(t, mustReceivePicoMessage(t, got))
	if wire["type"] != "turn.done" || wire["session_id"] != "sess-1" {
		t.Fatalf("type/session = %v/%v", wire["type"], wire["session_id"])
	}
	if _, ok := wire["timestamp"].(float64); !ok {
		t.Fatalf("timestamp missing: %v", wire)
	}
	payload := wire["payload"].(map[string]any)
	if payload["request_id"] != "req-1" || payload["status"] != "ok" {
		t.Fatalf("payload = %v", payload)
	}
	ids := payload["request_ids"].([]any)
	if len(ids) != 2 || ids[0] != "req-1" || ids[1] != "req-2" {
		t.Fatalf("request_ids = %v", ids)
	}
	usage := payload["usage"].(map[string]any)
	want := map[string]float64{"input_tokens": 100, "output_tokens": 20, "total_tokens": 120, "llm_calls": 2}
	for k, v := range want {
		if usage[k] != v {
			t.Fatalf("usage[%s] = %v, want %v", k, usage[k], v)
		}
	}
}

func TestSendTurnDone_OmitsEmptyFields(t *testing.T) {
	ch := newTestPicoChannel(t)
	ch.SetRunning(true)
	got := captureBroadcasts(ch)

	if err := ch.SendTurnDone(context.Background(), "pico:s", channels.TurnDone{Status: "error"}); err != nil {
		t.Fatal(err)
	}
	payload := roundTrip(t, mustReceivePicoMessage(t, got))["payload"].(map[string]any)
	if len(payload) != 1 || payload["status"] != "error" {
		t.Fatalf("payload = %v, want only status", payload)
	}
}

func TestSend_FinalMessageCarriesTurnUsage(t *testing.T) {
	ch := newTestPicoChannel(t)
	ch.SetRunning(true)
	conn, received, cleanup := newTestPicoWebSocket(t)
	defer cleanup()
	ch.addConnForTest(&picoConn{id: "c1", conn: conn, sessionID: "s1"})

	msg := bus.OutboundMessage{
		Context: bus.NewOutboundContext("pico", "pico:s1", ""),
		Content: "answer",
	}
	msg.Context.Raw = map[string]string{
		"outbound_kind":          "final",
		channels.RawKeyTurnUsage: `{"input_tokens":7,"output_tokens":3,"total_tokens":10,"llm_calls":1}`,
	}
	if _, err := ch.Send(context.Background(), bus.NormalizeOutboundMessage(msg)); err != nil {
		t.Fatal(err)
	}
	out := mustReceivePicoMessage(t, received)
	usage, ok := out.Payload["usage"].(map[string]any)
	if out.Type != TypeMessageCreate || !ok || usage["total_tokens"] != float64(10) || usage["llm_calls"] != float64(1) {
		t.Fatalf("message = %+v, want message.create with usage", out)
	}

	// Messages without usage keep their old shape.
	plain := bus.OutboundMessage{Context: bus.NewOutboundContext("pico", "pico:s1", ""), Content: "x"}
	if _, err := ch.Send(context.Background(), bus.NormalizeOutboundMessage(plain)); err != nil {
		t.Fatal(err)
	}
	if out := mustReceivePicoMessage(t, received); out.Payload["usage"] != nil {
		t.Fatalf("plain message has usage: %+v", out.Payload)
	}
}

// Upstream #3391 reports multi-line input arriving as one message per line.
// The server keeps a multi-line message.send as a single inbound message, so
// a splitting client is the cause, not the pico channel.
func TestWebSocket_MultiLineMessageStaysOneInboundMessage(t *testing.T) {
	msgBus := bus.NewMessageBus()
	bc := &config.Channel{Type: config.ChannelPico, Enabled: true}
	cfg := &config.PicoSettings{}
	cfg.SetToken("test-token")
	ch, err := NewPicoChannel(bc, cfg, msgBus)
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer ch.Stop(context.Background())

	server := httptest.NewServer(ch)
	defer server.Close()
	header := http.Header{"Authorization": []string{"Bearer test-token"}}
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/pico/ws?session_id=s1"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	resp.Body.Close()

	text := "first line\nsecond line\n\n  indented third\r\nfourth"
	if err := conn.WriteJSON(PicoMessage{
		Type: TypeMessageSend, ID: "req-ml", Payload: map[string]any{"content": text},
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case inbound := <-msgBus.InboundChan():
		if inbound.Content != text {
			t.Fatalf("content = %q, want %q", inbound.Content, text)
		}
		if inbound.Context.MessageID != "req-ml" {
			t.Fatalf("message id = %q, want req-ml", inbound.Context.MessageID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no inbound message")
	}
	select {
	case extra := <-msgBus.InboundChan():
		t.Fatalf("multi-line message split, extra inbound %q", extra.Content)
	case <-time.After(200 * time.Millisecond):
	}
}
