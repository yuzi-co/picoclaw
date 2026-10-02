// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT

package agent

import (
	"strings"

	"github.com/sipeed/picoclaw/pkg/providers"
)

// Placeholders left where an image was taken out of the model context, so a
// message never ends up empty (strict endpoints reject empty user content)
// and the model knows an image was there.
const (
	historicalImageOmittedNote = "[earlier image omitted from context]"
	cappedImageOmittedNote     = "[older image omitted from context; only the most recent images are kept]"
)

func isInlineImage(ref string) bool {
	return strings.HasPrefix(ref, "data:image/")
}

// isHistoricalDataURL reports whether ref is an inline data URL in a message
// before the current turn. Such URLs are base64 images rehydrated in an
// earlier turn; sending them again on every call costs a vision model far
// more than the path tag that stays in the history (idea from
// Vexx336/picoclaw b0e89004).
func isHistoricalDataURL(ref string, idx, currentTurnStart int) bool {
	return idx < currentTurnStart && strings.HasPrefix(ref, "data:")
}

// noteOmittedImage keeps a message that lost all its media from becoming
// empty.
func noteOmittedImage(msg providers.Message, note string) providers.Message {
	if len(msg.Media) == 0 && strings.TrimSpace(msg.Content) == "" {
		msg.Content = note
	}
	return msg
}

// capContextImages keeps only the newest maxImages inline images in
// messages and drops older ones. Each screenshot a tool returns is sent back
// to the model on every following call of the turn; small local vision
// models run out of context after a few of them. maxImages <= 0 keeps every
// image. A synthetic "[Loaded image from tool result above]" message that
// loses its image says so instead. Returns messages unchanged when nothing
// is dropped; the input slice is never mutated.
func capContextImages(messages []providers.Message, maxImages int) []providers.Message {
	if maxImages <= 0 {
		return messages
	}

	kept := 0
	var result []providers.Message
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if len(m.Media) == 0 {
			continue
		}
		var media []string
		dropped := false
		for j := len(m.Media) - 1; j >= 0; j-- {
			ref := m.Media[j]
			if isInlineImage(ref) {
				if kept >= maxImages {
					dropped = true
					continue
				}
				kept++
			}
			media = append(media, ref)
		}
		if !dropped {
			continue
		}
		if result == nil {
			result = append([]providers.Message(nil), messages...)
		}
		// media was collected newest first; restore the original order.
		for l, r := 0, len(media)-1; l < r; l, r = l+1, r-1 {
			media[l], media[r] = media[r], media[l]
		}
		m.Media = media
		if len(media) == 0 && m.Role == "user" && strings.HasPrefix(m.Content, "[Loaded image from tool result") {
			m.Content = cappedImageOmittedNote
		}
		result[i] = noteOmittedImage(m, cappedImageOmittedNote)
	}
	if result == nil {
		return messages
	}
	return result
}
