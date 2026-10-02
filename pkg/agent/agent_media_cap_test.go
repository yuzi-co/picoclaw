package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/media"
	"github.com/sipeed/picoclaw/pkg/providers"
)

func countInlineImages(messages []providers.Message) int {
	n := 0
	for _, m := range messages {
		for _, ref := range m.Media {
			if isInlineImage(ref) {
				n++
			}
		}
	}
	return n
}

func TestCapContextImages_KeepsNewest(t *testing.T) {
	messages := []providers.Message{
		{Role: "user", Content: "look"},
		{Role: "tool", Content: "shot 1", ToolCallID: "c1"},
		toolImageFollowUpPromptMessage([]string{"data:image/png;base64,ONE"}),
		{Role: "tool", Content: "shot 2", ToolCallID: "c2"},
		toolImageFollowUpPromptMessage([]string{"data:image/png;base64,TWO", "data:image/png;base64,THREE"}),
		{Role: "tool", Content: "shot 4", ToolCallID: "c4"},
		toolImageFollowUpPromptMessage([]string{"data:image/png;base64,FOUR"}),
	}
	orig := slices.Clone(messages)

	got := capContextImages(messages, 2)

	if len(got) != len(messages) {
		t.Fatalf("len = %d, want %d (messages are kept, only images dropped)", len(got), len(messages))
	}
	if n := countInlineImages(got); n != 2 {
		t.Fatalf("inline images = %d, want 2", n)
	}
	if !slices.Equal(got[6].Media, []string{"data:image/png;base64,FOUR"}) {
		t.Fatalf("newest message media = %v", got[6].Media)
	}
	if !slices.Equal(got[4].Media, []string{"data:image/png;base64,THREE"}) {
		t.Fatalf("second message media = %v, want only THREE", got[4].Media)
	}
	if len(got[2].Media) != 0 || got[2].Content != cappedImageOmittedNote {
		t.Fatalf("oldest synthetic message = %+v, want no media and the omitted note", got[2])
	}
	for i := range orig {
		if !slices.Equal(orig[i].Media, messages[i].Media) || orig[i].Content != messages[i].Content {
			t.Fatalf("input message %d mutated", i)
		}
	}
}

func TestCapContextImages_ZeroKeepsAll(t *testing.T) {
	messages := []providers.Message{
		toolImageFollowUpPromptMessage([]string{"data:image/png;base64,ONE"}),
		toolImageFollowUpPromptMessage([]string{"data:image/png;base64,TWO"}),
		toolImageFollowUpPromptMessage([]string{"data:image/png;base64,THREE"}),
	}
	if n := countInlineImages(capContextImages(messages, 0)); n != 3 {
		t.Fatalf("inline images = %d, want 3", n)
	}
}

func TestCapContextImages_LeavesOtherMediaAlone(t *testing.T) {
	messages := []providers.Message{
		{Role: "user", Content: "a", Media: []string{"https://example.com/a.png", "data:image/png;base64,ONE"}},
		toolImageFollowUpPromptMessage([]string{"data:image/png;base64,TWO"}),
	}
	got := capContextImages(messages, 1)
	if !slices.Equal(got[0].Media, []string{"https://example.com/a.png"}) || got[0].Content != "a" {
		t.Fatalf("first message = %+v, want URL kept and content unchanged", got[0])
	}
}

func TestResolveMediaRefs_DropsHistoricalDataURLs(t *testing.T) {
	messages := []providers.Message{
		{Role: "user", Content: "", Media: []string{"data:image/png;base64,OLD"}},
		{Role: "user", Content: "with url", Media: []string{"https://example.com/a.png", "data:image/png;base64,OLD2"}},
		{Role: "assistant", Content: "ok"},
		{Role: "user", Content: "now", Media: []string{"data:image/png;base64,NEW"}},
	}
	got := resolveMediaRefs(messages, media.NewFileMediaStore(), config.DefaultMaxMediaSize, 3)

	if len(got[0].Media) != 0 || got[0].Content != historicalImageOmittedNote {
		t.Fatalf("historical image-only message = %+v, want no media and the omitted note", got[0])
	}
	if !slices.Equal(got[1].Media, []string{"https://example.com/a.png"}) || got[1].Content != "with url" {
		t.Fatalf("historical message with URL = %+v", got[1])
	}
	if !slices.Equal(got[3].Media, []string{"data:image/png;base64,NEW"}) {
		t.Fatalf("current message media = %v, want the data URL kept", got[3].Media)
	}
}

// repeatedScreenshotProvider asks for load_image a fixed number of times and
// records how many inline images each call carried.
type repeatedScreenshotProvider struct {
	path       string
	shots      int
	calls      int
	imagesSeen []int
}

func (p *repeatedScreenshotProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	p.calls++
	p.imagesSeen = append(p.imagesSeen, countInlineImages(messages))
	if p.calls <= p.shots {
		return &providers.LLMResponse{
			Content: "",
			ToolCalls: []providers.ToolCall{{
				ID:        fmt.Sprintf("call_shot_%d", p.calls),
				Type:      "function",
				Name:      "load_image",
				Arguments: map[string]any{"path": p.path},
			}},
		}, nil
	}
	return &providers.LLMResponse{Content: "done"}, nil
}

func (p *repeatedScreenshotProvider) GetDefaultModel() string { return "test-model" }

func runRepeatedScreenshots(t *testing.T, maxImages int) []int {
	t.Helper()
	workspace := t.TempDir()
	pngPath := filepath.Join(workspace, "screen.png")
	pngBytes := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x02,
		0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xDE,
	}
	if err := os.WriteFile(pngPath, pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         workspace,
				ModelName:         "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
				MaxContextImages:  maxImages,
			},
		},
		Tools: config.ToolsConfig{LoadImage: config.ToolConfig{Enabled: true}},
		ModelList: []*config.ModelConfig{
			{ModelName: "test-model", Model: "openai/test-model"},
		},
	}
	provider := &repeatedScreenshotProvider{path: pngPath, shots: 4}
	al := NewAgentLoop(cfg, bus.NewMessageBus(), provider)
	al.SetMediaStore(media.NewFileMediaStore())

	ctx, cancel := context.WithTimeout(context.Background(), responseTimeout)
	defer cancel()
	resp, err := al.processMessage(ctx, testInboundMessage(bus.InboundMessage{
		Context: bus.InboundContext{
			Channel: "telegram", ChatID: "chat1", ChatType: "direct", SenderID: "user1", MessageID: "m1",
		},
		Content:    "take screenshots",
		SessionKey: "agent:main:telegram:direct:user1",
	}))
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if !strings.Contains(resp, "done") {
		t.Fatalf("response = %q, want done", resp)
	}
	return provider.imagesSeen
}

func TestAgentLoop_MaxContextImagesCapsScreenshotsWithinTurn(t *testing.T) {
	if got, want := runRepeatedScreenshots(t, 2), []int{0, 1, 2, 2, 2}; !slices.Equal(got, want) {
		t.Fatalf("images per call = %v, want %v", got, want)
	}
	if got, want := runRepeatedScreenshots(t, 0), []int{0, 1, 2, 3, 4}; !slices.Equal(got, want) {
		t.Fatalf("images per call without a cap = %v, want %v", got, want)
	}
}
