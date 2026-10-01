//go:build ironkvm

// The ironkvm build leaves out the Azure OpenAI provider, which links the
// OpenAI SDK. This stub keeps the exported surface the provider factory
// uses; a configured azure model fails on its first request.

package azure

import (
	"context"
	"errors"

	"github.com/sipeed/picoclaw/pkg/providers/protocoltypes"
)

type (
	LLMResponse    = protocoltypes.LLMResponse
	Message        = protocoltypes.Message
	ToolDefinition = protocoltypes.ToolDefinition
)

var errOmitted = errors.New("azure provider is not included in the ironkvm build")

// Provider is a placeholder that rejects every request.
type Provider struct{}

// Option configures the Azure Provider. Options are ignored in this build.
type Option func(*Provider)

// NewProvider returns a provider whose Chat always fails.
func NewProvider(_, _, _, _ string, _ ...Option) *Provider { return &Provider{} }

// NewProviderWithTimeout returns a provider whose Chat always fails.
func NewProviderWithTimeout(_, _, _, _ string, _ int) *Provider { return &Provider{} }

// Chat reports that the Azure provider is not part of this build.
func (p *Provider) Chat(
	context.Context, []Message, []ToolDefinition, string, map[string]any,
) (*LLMResponse, error) {
	return nil, errOmitted
}

// GetDefaultModel returns an empty model name.
func (p *Provider) GetDefaultModel() string { return "" }
