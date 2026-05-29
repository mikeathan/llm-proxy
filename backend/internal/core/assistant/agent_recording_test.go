//go:build recordreplay

package assistant

import (
	"context"
	"fmt"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/testing/llmprofiles"
	"strings"
	"sync"
	"testing"
)

type ReplayEngine struct {
	calls []llmprofiles.RecordedCall
	turn  int
	idx   int
	mu    sync.Mutex
}

func (e *ReplayEngine) ExecuteTool(ctx context.Context, tc proxy.ToolCall) (any, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.turn >= len(e.calls) {
		return nil, fmt.Errorf("replay: no recorded turn %d (total %d)", e.turn, len(e.calls))
	}

	tr := e.calls[e.turn].ToolResults
	if e.idx >= len(tr) {
		return nil, fmt.Errorf("replay: tool %d out of range (turn %d has %d results)", e.idx, e.turn, len(tr))
	}

	result := tr[e.idx].Result
	e.idx++
	if e.idx >= len(tr) {
		e.turn++
		e.idx = 0
	}

	return result, nil
}

func TestAgent_Execute_AgainstRecordings(t *testing.T) {
	llmprofiles.RunAgainstFixtures(t, "../../../testdata/recordings", func(t *testing.T, client proxy.Client, name string) {
		fc, ok := client.(*llmprofiles.FixtureClient)
		if !ok {
			t.Fatal("expected FixtureClient")
		}
		calls := fc.Calls()
		if len(calls) == 0 {
			t.Fatal("fixture has no calls")
		}

		// Build tool list from first call's request
		provider := &MockProvider{
			Tools: calls[0].Request.Tools,
		}
		engine := &ReplayEngine{calls: calls}

		agent := NewAgent(client, provider, engine, AgentOptions{
			MaxSteps:       len(calls) + 5,
			UseNativeTools: boolPtr(false),
		})

		// Use the recorded initial messages as starting history
		initialMessages := calls[0].Request.Messages
		if len(initialMessages) == 0 {
			initialMessages = []proxy.Message{{Role: proxy.UserRole, Content: "Hello"}}
		}

		_, _, err := agent.Execute(context.Background(), initialMessages)
		if err != nil && !strings.Contains(err.Error(), "max steps") {
			t.Errorf("unexpected error for fixture %s: %v", name, err)
		}
	})
}
