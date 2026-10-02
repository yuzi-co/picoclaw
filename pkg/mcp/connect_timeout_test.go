package mcp

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestConnectServer_TimesOutAndCancelsTheAttempt(t *testing.T) {
	original := connectServerFunc
	t.Cleanup(func() { connectServerFunc = original })

	canceled := make(chan struct{})
	connectServerFunc = func(ctx context.Context, _ string, _ config.MCPServerConfig) (*ServerConnection, error) {
		<-ctx.Done()
		close(canceled)
		return nil, ctx.Err()
	}

	mgr := NewManager(WithConnectTimeout(200 * time.Millisecond))
	start := time.Now()
	err := mgr.ConnectServer(context.Background(), "hang", config.MCPServerConfig{Command: "x"})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want timeout", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("ConnectServer took %s", elapsed)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("the timed-out attempt's context was not canceled")
	}
	if len(mgr.GetServers()) != 0 {
		t.Fatal("timed-out server registered")
	}
}

func TestLoadFromMCPConfig_HangingServerDoesNotBlockOthers(t *testing.T) {
	original := connectServerFunc
	t.Cleanup(func() { connectServerFunc = original })
	connectServerFunc = func(ctx context.Context, name string, cfg config.MCPServerConfig) (*ServerConnection, error) {
		if name == "hang" {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return &ServerConnection{Name: name, Config: cfg, Tools: []*sdkmcp.Tool{{Name: "t"}}}, nil
	}

	mgr := NewManager()
	start := time.Now()
	err := mgr.LoadFromMCPConfig(context.Background(), config.MCPConfig{
		ToolConfig:            config.ToolConfig{Enabled: true},
		ConnectTimeoutSeconds: 1,
		Servers: map[string]config.MCPServerConfig{
			"hang": {Enabled: true, Command: "x"},
			"good": {Enabled: true, Command: "y"},
		},
	}, t.TempDir())
	if err != nil {
		t.Fatalf("LoadFromMCPConfig: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("LoadFromMCPConfig took %s", elapsed)
	}
	if _, ok := mgr.GetServer("good"); !ok {
		t.Fatal("good server not connected")
	}
	if _, ok := mgr.GetServer("hang"); ok {
		t.Fatal("hanging server registered")
	}
}

// A stdio server that never answers initialize is given up on, and its
// process is killed rather than left running.
func TestConnectServer_SilentStdioServerTimesOut(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available")
	}
	mgr := NewManager(WithConnectTimeout(500 * time.Millisecond))
	start := time.Now()
	err := mgr.ConnectServer(context.Background(), "silent", config.MCPServerConfig{
		Type:    "stdio",
		Command: "sleep",
		Args:    []string{"30"},
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want timeout", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("ConnectServer took %s", elapsed)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		out, _ := exec.Command("sh", "-c", "ps -eo args 2>/dev/null | grep -c '^sleep 30$'").Output()
		if strings.TrimSpace(string(out)) == "0" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("sleep 30 still running after the timeout (ps count %s)", strings.TrimSpace(string(out)))
		}
		time.Sleep(50 * time.Millisecond)
	}
}
