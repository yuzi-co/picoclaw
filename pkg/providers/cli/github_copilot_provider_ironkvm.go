//go:build ironkvm

// The ironkvm build leaves out the GitHub Copilot provider, which links the
// Copilot SDK. NewGitHubCopilotProvider always fails, so the provider
// factory reports the error when the model is configured.

package cliprovider

import (
	"context"
	"errors"
)

var errCopilotOmitted = errors.New("github-copilot provider is not included in the ironkvm build")

// GitHubCopilotProvider is a placeholder; it is never constructed.
type GitHubCopilotProvider struct{}

// NewGitHubCopilotProvider reports that the provider is not part of this build.
func NewGitHubCopilotProvider(_, _, _ string) (*GitHubCopilotProvider, error) {
	return nil, errCopilotOmitted
}

// Close does nothing.
func (p *GitHubCopilotProvider) Close() {}

// Chat reports that the provider is not part of this build.
func (p *GitHubCopilotProvider) Chat(
	context.Context, []Message, []ToolDefinition, string, map[string]any,
) (*LLMResponse, error) {
	return nil, errCopilotOmitted
}

// GetDefaultModel returns an empty model name.
func (p *GitHubCopilotProvider) GetDefaultModel() string { return "" }
