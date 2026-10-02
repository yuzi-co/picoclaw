package agent

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
)

// An MCP server that accepts the connection but never answers (upstream
// #3269) delays the loop's start by at most the connect timeout; messages
// are then answered without its tools.
func TestAgentLoopRun_SilentMCPServerDoesNotBlockReplies(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available")
	}
	al, cfg, msgBus, _, cleanup := newTestAgentLoop(t)
	defer cleanup()
	defer al.Close()

	cfg.Tools = config.ToolsConfig{
		MCP: config.MCPConfig{
			ToolConfig:            config.ToolConfig{Enabled: true},
			ConnectTimeoutSeconds: 1,
			Servers: map[string]config.MCPServerConfig{
				"silent": {Enabled: true, Type: "stdio", Command: "sleep", Args: []string{"60"}},
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- al.Run(ctx) }()

	start := time.Now()
	if err := msgBus.PublishInbound(context.Background(), bus.InboundMessage{
		Context: bus.InboundContext{Channel: "telegram", ChatID: "chat1", ChatType: "direct", SenderID: "user1"},
		Channel: "telegram", ChatID: "chat1", SenderID: "user1",
		Content: "hello",
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case out := <-msgBus.OutboundChan():
		if out.Content == "" {
			t.Fatal("empty reply")
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("reply took %s", elapsed)
		}
	case err := <-runErr:
		t.Fatalf("Run() returned: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("no reply while an MCP server hangs")
	}
	cancel()
	select {
	case <-runErr:
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not stop")
	}
}
