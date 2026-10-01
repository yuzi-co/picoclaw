package pico

import (
	"context"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
)

func newControlTestChannel(t *testing.T, allow bool) (*PicoChannel, *bus.MessageBus) {
	t.Helper()
	msgBus := bus.NewMessageBus()
	bc := &config.Channel{Type: config.ChannelPico, Enabled: true}
	cfg := &config.PicoSettings{AllowControlCommands: allow}
	cfg.SetToken("test-token")
	ch, err := NewPicoChannel(bc, cfg, msgBus)
	if err != nil {
		t.Fatalf("NewPicoChannel: %v", err)
	}
	ch.ctx = context.Background()
	return ch, msgBus
}

func sendPicoText(ch *PicoChannel, pc *picoConn, text string) {
	ch.handleMessageSend(pc, PicoMessage{
		Type:      TypeMessageSend,
		ID:        "msg-1",
		SessionID: "sess-1",
		Payload:   map[string]any{PayloadKeyContent: text},
	})
}

func TestHandleMessageSend_RejectsControlCommandsByDefault(t *testing.T) {
	for _, text := range []string{"/reload", "!reload", "/switch model to other"} {
		t.Run(text, func(t *testing.T) {
			ch, msgBus := newControlTestChannel(t, false)
			clientConn, received, cleanup := newTestPicoWebSocket(t)
			defer cleanup()

			sendPicoText(ch, &picoConn{id: "conn-1", conn: clientConn, sessionID: "sess-1"}, text)

			reply := mustReceivePicoMessage(t, received)
			if reply.Type != TypeError || reply.Payload["code"] != "command_disabled" {
				t.Fatalf("reply = %+v, want error command_disabled", reply)
			}
			if reply.Payload["request_id"] != "msg-1" {
				t.Fatalf("request_id = %v, want msg-1", reply.Payload["request_id"])
			}
			select {
			case inbound := <-msgBus.InboundChan():
				t.Fatalf("control command reached the agent: %q", inbound.Content)
			case <-time.After(100 * time.Millisecond):
			}
		})
	}
}

func TestHandleMessageSend_ForwardsOrdinaryCommands(t *testing.T) {
	ch, msgBus := newControlTestChannel(t, false)
	sendPicoText(ch, &picoConn{id: "conn-1", sessionID: "sess-1"}, "/clear")

	select {
	case inbound := <-msgBus.InboundChan():
		if inbound.Content != "/clear" {
			t.Fatalf("content = %q, want /clear", inbound.Content)
		}
	case <-time.After(time.Second):
		t.Fatal("expected /clear to reach the agent")
	}
}

func TestHandleMessageSend_AllowsControlCommandsWhenEnabled(t *testing.T) {
	ch, msgBus := newControlTestChannel(t, true)
	sendPicoText(ch, &picoConn{id: "conn-1", sessionID: "sess-1"}, "/reload")

	select {
	case inbound := <-msgBus.InboundChan():
		if inbound.Content != "/reload" {
			t.Fatalf("content = %q, want /reload", inbound.Content)
		}
	case <-time.After(time.Second):
		t.Fatal("expected /reload to reach the agent when allowed")
	}
}

func TestClientHandleServerMessage_DropsControlCommandsByDefault(t *testing.T) {
	msgBus := bus.NewMessageBus()
	bc := &config.Channel{Type: config.ChannelPicoClient, Enabled: true}
	ch, err := NewPicoClientChannel(bc, &config.PicoClientSettings{URL: "ws://127.0.0.1:1/ws"}, msgBus)
	if err != nil {
		t.Fatalf("NewPicoClientChannel: %v", err)
	}
	ch.ctx = context.Background()
	pc := &picoConn{id: "conn-1", sessionID: "sess-1"}

	for _, text := range []string{"/reload", "/switch model to other", "/clear"} {
		ch.handleServerMessage(pc, PicoMessage{
			Type:    TypeMessageCreate,
			Payload: map[string]any{PayloadKeyContent: text},
		})
	}

	select {
	case inbound := <-msgBus.InboundChan():
		if inbound.Content != "/clear" {
			t.Fatalf("first inbound = %q, want /clear (control commands must be dropped)", inbound.Content)
		}
	case <-time.After(time.Second):
		t.Fatal("expected /clear to reach the agent")
	}
}
