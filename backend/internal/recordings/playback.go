package recordings

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

type PlaybackClient struct {
	turns []RecordedTurn
	index int
	mu    sync.Mutex
}

func NewPlaybackClient(path string) (*PlaybackClient, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("playback: open %s: %w", path, err)
	}
	defer f.Close()

	var turns []RecordedTurn
	var current *RecordedTurn

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var line rawLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			return nil, fmt.Errorf("playback: parse line: %w", err)
		}

		switch line.Type {
		case "request":
			if current != nil {
				turns = append(turns, *current)
			}
			current = &RecordedTurn{}
			current.Request.Messages = line.Messages
			current.Request.Tools = line.Tools

		case "chunk":
			if current != nil {
				current.Chunks = append(current.Chunks, line.Choices)
			}

		case "response":
			if current != nil {
				current.Response.Choices = line.Choices
			}

		case "error":
			if current != nil {
				current.Error = line.Message
			}
		}
	}
	if current != nil {
		turns = append(turns, *current)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("playback: scan: %w", err)
	}

	if len(turns) == 0 {
		return nil, fmt.Errorf("playback: no turns found in %s", path)
	}

	return &PlaybackClient{turns: turns}, nil
}

func (p *PlaybackClient) NextTurn() *RecordedTurn {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.index >= len(p.turns) {
		return nil
	}
	turn := p.turns[p.index]
	p.index++
	return &turn
}

func (p *PlaybackClient) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.index = 0
}

func (p *PlaybackClient) TurnCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.turns)
}
