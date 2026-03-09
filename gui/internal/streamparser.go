package internal

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// ── Claude CLI stream-json wire format ──

type streamEvent struct {
	Type   string          `json:"type"`   // "stream_event", "result", "system", "assistant", ...
	Event  *streamSubEvent `json:"event"`  // present when Type == "stream_event"
	Result string          `json:"result"` // present when Type == "result"
}

type streamSubEvent struct {
	Type         string        `json:"type"` // "content_block_start", "content_block_delta", "content_block_stop", ...
	Index        int           `json:"index"`
	ContentBlock *contentBlock `json:"content_block"`
	Delta        *streamDelta  `json:"delta"`
}

type contentBlock struct {
	Type string `json:"type"` // "thinking" or "text"
}

type streamDelta struct {
	Type     string `json:"type"`     // "thinking_delta", "text_delta", "signature_delta"
	Text     string `json:"text"`     // for text_delta
	Thinking string `json:"thinking"` // for thinking_delta
}

// ── Callbacks ──

type StreamCallbacks struct {
	OnText     func(text string) // accumulated text chunk (flushed on newline or 80+ chars)
	OnThinking func(text string) // accumulated thinking chunk
	OnResult   func(result string)
}

// ParseStream reads Claude CLI stream-json lines from reader and calls
// the appropriate callbacks. Buffers text/thinking and flushes on newline
// or when the buffer exceeds 80 characters.
func ParseStream(reader io.Reader, cb StreamCallbacks) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 128*1024), 512*1024)

	var textBuf strings.Builder
	var thinkBuf strings.Builder

	flushText := func() {
		if textBuf.Len() > 0 && cb.OnText != nil {
			cb.OnText(textBuf.String())
			textBuf.Reset()
		}
	}
	flushThink := func() {
		if thinkBuf.Len() > 0 && cb.OnThinking != nil {
			cb.OnThinking(thinkBuf.String())
			thinkBuf.Reset()
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var evt streamEvent
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			continue
		}

		switch evt.Type {
		case "stream_event":
			if evt.Event == nil {
				continue
			}
			switch evt.Event.Type {
			case "content_block_delta":
				if evt.Event.Delta == nil {
					continue
				}
				switch evt.Event.Delta.Type {
				case "text_delta":
					textBuf.WriteString(evt.Event.Delta.Text)
					if strings.Contains(evt.Event.Delta.Text, "\n") {
						flushText()
					}
				case "thinking_delta":
					thinkBuf.WriteString(evt.Event.Delta.Thinking)
					if strings.Contains(evt.Event.Delta.Thinking, "\n") {
						flushThink()
					}
				}
			case "content_block_stop":
				flushText()
				flushThink()
			}

		case "result":
			flushText()
			flushThink()
			if cb.OnResult != nil {
				cb.OnResult(evt.Result)
			}
		}
	}

	// Final flush
	flushText()
	flushThink()
}
