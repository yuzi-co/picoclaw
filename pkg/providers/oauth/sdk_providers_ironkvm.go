//go:build ironkvm

// The ironkvm build leaves out the Claude and Codex OAuth providers, which
// link the Anthropic and OpenAI SDKs. These stubs keep the exported surface
// the provider facade uses: the token sources fail with a clear message, and
// a provider that does get constructed fails on its first request.

package oauthprovider

import (
	"context"
	"errors"

	"github.com/sipeed/picoclaw/pkg/auth"
)

var (
	errClaudeOmitted = errors.New("anthropic oauth provider is not included in the ironkvm build")
	errCodexOmitted  = errors.New("openai oauth (codex) provider is not included in the ironkvm build")
)

// ClaudeProvider is a placeholder that rejects every request.
type ClaudeProvider struct{}

func NewClaudeProvider(string) *ClaudeProvider                    { return &ClaudeProvider{} }
func NewClaudeProviderWithBaseURL(string, string) *ClaudeProvider { return &ClaudeProvider{} }

func NewClaudeProviderWithTokenSource(string, func() (string, error)) *ClaudeProvider {
	return &ClaudeProvider{}
}

func NewClaudeProviderWithTokenSourceAndBaseURL(string, func() (string, error), string) *ClaudeProvider {
	return &ClaudeProvider{}
}

func (p *ClaudeProvider) Chat(
	context.Context, []Message, []ToolDefinition, string, map[string]any,
) (*LLMResponse, error) {
	return nil, errClaudeOmitted
}

func (p *ClaudeProvider) GetDefaultModel() string { return "" }

func CreateClaudeTokenSource(func(string) (*auth.AuthCredential, error)) func() (string, error) {
	return func() (string, error) { return "", errClaudeOmitted }
}

// CodexProvider is a placeholder that rejects every request.
type CodexProvider struct{}

func NewCodexProvider(string, string) *CodexProvider { return &CodexProvider{} }

func NewCodexProviderWithTokenSource(string, string, func() (string, string, error)) *CodexProvider {
	return &CodexProvider{}
}

func (p *CodexProvider) Chat(
	context.Context, []Message, []ToolDefinition, string, map[string]any,
) (*LLMResponse, error) {
	return nil, errCodexOmitted
}

func (p *CodexProvider) GetDefaultModel() string { return "" }

func (p *CodexProvider) SupportsNativeSearch() bool { return false }

func CreateCodexTokenSource() func() (string, string, error) {
	return func() (string, string, error) { return "", "", errCodexOmitted }
}
