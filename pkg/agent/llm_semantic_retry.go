// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT

package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
)

// errSemanticRetry marks a response that arrived without a transport error
// but is not usable as a final answer: no text and no tool calls (a
// thinking-only reply, common with Qwen on Ollama), or a finish_reason the
// config marks as retriable ("length" by default). The retry loop treats it
// like a transient provider error, under max_llm_retries and
// llm_retry_backoff_secs. Based on sstpnk/picoclaw 18f3843e.
var errSemanticRetry = errors.New("llm semantic retry")

// semanticRetryError returns a non-nil error wrapping errSemanticRetry when
// resp should be requested again. A reply with tool calls is never empty: its
// text is optional. A truncated reply whose text the streaming publisher
// already showed to the user is kept, since a retry would show it twice.
func semanticRetryError(
	defaults config.AgentDefaults,
	resp *providers.LLMResponse,
	publisher *streamingChunkPublisher,
) error {
	if resp == nil {
		return nil
	}
	if defaults.LLMRetryOnEmptyContent &&
		len(resp.ToolCalls) == 0 &&
		strings.TrimSpace(resp.Content) == "" {
		return fmt.Errorf("%w: empty LLM response (finish_reason=%q)", errSemanticRetry, resp.FinishReason)
	}
	if resp.FinishReason == "" || (publisher != nil && publisher.Published()) {
		return nil
	}
	for _, reason := range defaults.LLMRetryOnFinishReasons {
		if strings.EqualFold(strings.TrimSpace(reason), resp.FinishReason) {
			return fmt.Errorf("%w: finish_reason=%q", errSemanticRetry, resp.FinishReason)
		}
	}
	return nil
}

// semanticRetryReason is the retry reason reported in events and logs.
func semanticRetryReason(err error) (string, bool) {
	if !errors.Is(err, errSemanticRetry) {
		return "", false
	}
	if strings.Contains(err.Error(), "empty LLM response") {
		return "empty_response", true
	}
	return "finish_reason", true
}
