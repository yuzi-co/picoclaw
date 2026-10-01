//go:build ironkvm

package gateway

// The ironkvm build talks to the KVM server over the pico WebSocket only,
// so it links that channel and none of the others (see channels.go).
import (
	_ "github.com/sipeed/picoclaw/pkg/channels/pico"
)
