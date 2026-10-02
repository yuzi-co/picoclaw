package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// DefaultConnectTimeout bounds connecting to one MCP server, including the
// initialize handshake and the tool listing. Without it an unreachable
// server (a blackholed URL, or a stdio command that never answers) blocks
// the agent loop's start, and with it every reply (upstream #3269).
const DefaultConnectTimeout = 30 * time.Second

// WithConnectTimeout sets how long connecting to one server may take; d <= 0
// keeps DefaultConnectTimeout.
func WithConnectTimeout(d time.Duration) ManagerOption {
	return func(m *Manager) {
		if d > 0 {
			m.connectTimeout = d
		}
	}
}

func (m *Manager) applyConnectTimeout(cfg config.MCPConfig) {
	if cfg.ConnectTimeoutSeconds > 0 {
		m.connectTimeout = time.Duration(cfg.ConnectTimeoutSeconds) * time.Second
	}
}

// connect connects to one server within the manager's connect timeout.
//
// The connection runs under a context derived from ctx. On timeout that
// context is canceled, which aborts the handshake and kills a stdio
// server's process (it is started with exec.CommandContext). On success the
// context lives until the session ends, so the process is not killed early.
func (m *Manager) connect(ctx context.Context, name string, cfg config.MCPServerConfig) (*ServerConnection, error) {
	timeout := m.connectTimeout
	if timeout <= 0 {
		timeout = DefaultConnectTimeout
	}

	connCtx, cancel := context.WithCancel(ctx)
	type result struct {
		conn *ServerConnection
		err  error
	}
	done := make(chan result, 1)
	go func() {
		conn, err := connectServerFunc(connCtx, name, cfg)
		done <- result{conn, err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case r := <-done:
		if r.err != nil || r.conn == nil || r.conn.Session == nil {
			cancel()
			return r.conn, r.err
		}
		go func() {
			_ = r.conn.Session.Wait()
			cancel()
		}()
		return r.conn, nil
	case <-timer.C:
		cancel()
		// A connection that completes after the deadline is closed so the
		// server process does not linger.
		go func() {
			if r := <-done; r.conn != nil && r.conn.Session != nil {
				_ = r.conn.Session.Close()
			}
		}()
		logger.WarnCF("mcp", "MCP server connection timed out", map[string]any{
			"server":  name,
			"timeout": timeout.String(),
		})
		return nil, fmt.Errorf("connecting to MCP server %s timed out after %s", name, timeout)
	}
}
