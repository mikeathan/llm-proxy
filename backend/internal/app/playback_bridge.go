package app

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/recordings"
	"llm-proxy/models"
)

const (
	defaultReplayDelay    = 500 * time.Millisecond
	defaultChunkInterval  = 5 * time.Millisecond
)

type PlaybackBridge struct {
	client        *recordings.PlaybackClient
	replayDelay   time.Duration
	chunkInterval time.Duration
}

func NewPlaybackBridge(client *recordings.PlaybackClient) *PlaybackBridge {
	return &PlaybackBridge{
		client:        client,
		replayDelay:   defaultReplayDelay,
		chunkInterval: defaultChunkInterval,
	}
}

// nextValidTurn skips error-only turns (transient LLM connection errors from
// the original recording) and returns the next turn with usable data.
func (b *PlaybackBridge) nextValidTurn() *recordings.RecordedTurn {
	for {
		turn := b.client.NextTurn()
		if turn == nil {
			return nil
		}
		// Turns with both error and data: clear the error, serve the data
		if turn.Error != "" && (len(turn.Chunks) > 0 || len(turn.Response.Choices) > 0) {
			turn.Error = ""
			return turn
		}
		// Error-only turns: skip (transient noise)
		if turn.Error != "" {
			continue
		}
		return turn
	}
}

func emptyChatResponse() *proxy.ChatResponse {
	return &proxy.ChatResponse{
		Choices: []proxy.Choice{{
			Message: proxy.Message{Role: proxy.AssistantRole},
		}},
	}
}

func normalizeToolCalls(msg *proxy.Message) {
	for j := range msg.ToolCalls {
		tc := &msg.ToolCalls[j]
		if tc.Function.Name == models.ToolTerminalExecute {
			tc.Function.Arguments = flattenCommandArgs(tc.Function.Arguments)
		}
	}
}

func flattenCommandArgs(argsJSON string) string {
	var args struct {
		Command string `json:"command"`
		Cwd     string `json:"cwd,omitempty"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return argsJSON
	}
	if !strings.Contains(args.Command, "\n") {
		return argsJSON
	}
	args.Command = flattenCommand(args.Command)
	out, _ := json.Marshal(args)
	return string(out)
}

func flattenCommand(cmd string) string {
	cmd = strings.ReplaceAll(cmd, "\r\n", "\n")

	if strings.Contains(cmd, "<<") {
		return flattenHeredoc(cmd)
	}

	lines := strings.Split(cmd, "\n")
	var cleaned []string
	for _, p := range lines {
		p = strings.TrimSpace(p)
		if p != "" {
			cleaned = append(cleaned, p)
		}
	}
	return strings.Join(cleaned, " && ")
}

var heredocOpenRe = regexp.MustCompile(`cat\s+(>|>>)\s+(\S+)\s*<<\s*['"]?(\w+)['"]?`)

func flattenHeredoc(cmd string) string {
	m := heredocOpenRe.FindStringSubmatch(cmd)
	if m == nil {
		if idx := strings.Index(cmd, "<<"); idx >= 0 {
			cmd = strings.TrimSpace(cmd[:idx])
		}
		return strings.Join(strings.Fields(cmd), " ")
	}
	op, target, delim := m[1], m[2], m[3]
	// Find content between the opening line and the closing delimiter on its own line
	afterOpen := cmd[len(m[0]):]
	contentEnd := strings.Index(afterOpen, "\n"+delim)
	if contentEnd < 0 {
		if idx := strings.Index(cmd, "<<"); idx >= 0 {
			cmd = strings.TrimSpace(cmd[:idx])
		}
		return strings.Join(strings.Fields(cmd), " ")
	}
	content := strings.TrimSpace(afterOpen[1:contentEnd])
	trailing := strings.TrimSpace(afterOpen[contentEnd+1+len(delim):])
	var quoted []string
	for _, line := range strings.Split(content, "\n") {
		quoted = append(quoted, fmt.Sprintf("'%s'", strings.ReplaceAll(line, "'", "'\\''")))
	}
	result := fmt.Sprintf("printf '%%s\\n' %s %s %s", strings.Join(quoted, " "), op, target)
	if trailing != "" {
		result += " && " + trailing
	}
	return result
}

func (b *PlaybackBridge) Chat(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
	turn := b.nextValidTurn()
	if turn == nil {
		return emptyChatResponse(), nil
	}

	select {
	case <-time.After(b.replayDelay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	if len(turn.Response.Choices) > 0 {
		var choices []proxy.Choice
		if err := json.Unmarshal(turn.Response.Choices, &choices); err != nil {
			return nil, fmt.Errorf("playback: unmarshal response choices: %w", err)
		}
		for i := range choices {
			normalizeToolCalls(&choices[i].Message)
		}
		return &proxy.ChatResponse{Choices: choices}, nil
	}

	if len(turn.Chunks) > 0 {
		resp := synthesizeFromChunks(turn.Chunks)
		if len(resp.Choices) > 0 {
			normalizeToolCalls(&resp.Choices[0].Message)
		}
		return resp, nil
	}

	return nil, fmt.Errorf("playback: turn has no response or chunks")
}

func (b *PlaybackBridge) Stream(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
	turn := b.nextValidTurn()
	if turn == nil {
		ch := make(chan *proxy.ChatResponse, 1)
		ch <- emptyChatResponse()
		close(ch)
		return ch, nil
	}

	select {
	case <-time.After(b.replayDelay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	if len(turn.Chunks) > 0 {
		// Reconstruct complete tool calls from all chunk deltas, normalize them,
		// then send content/reasoning chunks with pacing as-is (minus tool call
		// deltas), followed by the complete normalized tool calls.
		var reconstructed []proxy.ToolCall
		for _, raw := range turn.Chunks {
			var choices []proxy.Choice
			if err := json.Unmarshal(raw, &choices); err != nil {
				continue
			}
			for _, choice := range choices {
				for _, tc := range choice.Delta.ToolCalls {
					if tc.ID != "" {
						reconstructed = append(reconstructed, tc)
					} else if len(reconstructed) > 0 {
						last := &reconstructed[len(reconstructed)-1]
						last.Function.Arguments += tc.Function.Arguments
					}
				}
			}
		}
		msg := proxy.Message{Role: proxy.AssistantRole, ToolCalls: reconstructed}
		normalizeToolCalls(&msg)

		ch := make(chan *proxy.ChatResponse)
		go func() {
			defer close(ch)
			for _, raw := range turn.Chunks {
				var choices []proxy.Choice
				if err := json.Unmarshal(raw, &choices); err != nil {
					return
				}

				hasNonTool := false
				for i := range choices {
					if choices[i].Delta.Content != "" || choices[i].Delta.ReasoningContent != "" {
						hasNonTool = true
					}
					choices[i].Delta.ToolCalls = nil
				}

				if hasNonTool {
					select {
					case ch <- &proxy.ChatResponse{Choices: choices}:
					case <-ctx.Done():
						return
					}
					select {
					case <-time.After(b.chunkInterval):
					case <-ctx.Done():
						return
					}
				}
			}

			for _, tc := range msg.ToolCalls {
				select {
				case ch <- &proxy.ChatResponse{
					Choices: []proxy.Choice{{
						Delta: proxy.Message{
							Role:      proxy.AssistantRole,
							ToolCalls: []proxy.ToolCall{tc},
						},
					}},
				}:
				case <-ctx.Done():
					return
				}
				select {
				case <-time.After(b.chunkInterval):
				case <-ctx.Done():
					return
				}
			}
		}()
		return ch, nil
	}

	if len(turn.Response.Choices) > 0 {
		var choices []proxy.Choice
		if err := json.Unmarshal(turn.Response.Choices, &choices); err != nil {
			return nil, fmt.Errorf("playback: unmarshal response choices: %w", err)
		}
		ch := make(chan *proxy.ChatResponse, 1)
		deltas := make([]proxy.Choice, len(choices))
		for i, c := range choices {
			normalizeToolCalls(&c.Message)
			deltas[i] = proxy.Choice{Delta: c.Message}
		}
		ch <- &proxy.ChatResponse{Choices: deltas}
		close(ch)
		return ch, nil
	}

	return nil, fmt.Errorf("playback: turn has no chunks or response")
}

func synthesizeFromChunks(rawChunks []json.RawMessage) *proxy.ChatResponse {
	msg := proxy.Message{Role: proxy.AssistantRole}
	for _, raw := range rawChunks {
		var choices []proxy.Choice
		if err := json.Unmarshal(raw, &choices); err != nil {
			continue
		}
		for _, choice := range choices {
			msg.Content += choice.Delta.Content
			msg.ReasoningContent += choice.Delta.ReasoningContent
			for _, tc := range choice.Delta.ToolCalls {
				if tc.ID != "" {
					msg.ToolCalls = append(msg.ToolCalls, tc)
				} else if len(msg.ToolCalls) > 0 {
					last := &msg.ToolCalls[len(msg.ToolCalls)-1]
					last.Function.Arguments += tc.Function.Arguments
				}
			}
		}
	}
	return &proxy.ChatResponse{Choices: []proxy.Choice{{Message: msg}}}
}

var _ proxy.Client = (*PlaybackBridge)(nil)
