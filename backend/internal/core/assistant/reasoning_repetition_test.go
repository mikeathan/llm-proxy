package assistant

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant/reasoning"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
)

// variedReasoning builds deterministic prose that never repeats a 64-char
// stretch, the shape of a model thinking through new material.
func variedReasoning(chars int) string {
	vocab := strings.Fields("model release memory date window source brief check entry news verify item " +
		"search result snippet publish article report table column order newest oldest likely probably " +
		"however instead because therefore remember include exclude duplicate confirm missing weekend")
	rnd := rand.New(rand.NewSource(7))
	var b strings.Builder
	for b.Len() < chars {
		fmt.Fprintf(&b, "%s ", vocab[rnd.Intn(len(vocab))])
		if rnd.Intn(14) == 0 {
			b.WriteString("\n")
		}
	}
	return b.String()[:chars]
}

func readReasoningFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "reasoning", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(b)
}

// firstRepeatFlag replays a growing reasoning text through the same gate the
// stream uses (a check per reasoningRepeatStep of growth) and returns the length
// at which it first flags, or -1.
func firstRepeatFlag(text string) int {
	checked := 0
	for n := reasoningRepeatStep; n <= len(text)+reasoningRepeatStep; n += reasoningRepeatStep {
		end := min(n, len(text))
		if reasoningRepeating(text[:end], &checked) {
			return end
		}
	}
	return -1
}

func TestReasoningRepeating(t *testing.T) {
	prefix := variedReasoning(6000)
	cycle := func(unit string, total int) string {
		return strings.Repeat(unit, total/len(unit)+1)[:total]
	}
	structured := map[string]string{
		"a csv draft": func() string {
			var b strings.Builder
			b.WriteString("Let me draft the file:\nid,name,score,date\n")
			for i := 0; i < 120; i++ {
				fmt.Fprintf(&b, "%d,item-%d,%d.%d,2026-10-%02d\n", 1000+i, i, i*3%97, i%10, i%28+1)
			}
			return b.String()
		}(),
		"an html table": func() string {
			var b strings.Builder
			b.WriteString("<table>\n")
			for _, n := range strings.Fields("Alder Birch Cedar Dogwood Elm Fir Ginkgo Hazel Ironwood Juniper Katsura Larch") {
				fmt.Fprintf(&b, "  <tr><td class=\"name\">%s</td><td class=\"count\">%d</td></tr>\n", n, len(n)*7)
			}
			return b.String() + "</table>\n"
		}(),
		"a per-file checklist": func() string {
			var b strings.Builder
			for i := 1; i <= 60; i++ {
				fmt.Fprintf(&b, "Check item %d: file component_%d.vue — imports fine, props typed, nothing to change.\n", i, i)
			}
			return b.String()
		}(),
	}
	paragraphs := "Actually, wait. I need to verify whether the Reflection item was already reported in today's memory entries before including it.\n" +
		"Looking at the memory, entry 3 lists the same open-weight model, so it is a duplicate and should be excluded from the brief.\n" +
		"But the scoop is dated Oct 4 which is inside the window, so maybe it is still new. Let me re-check the dates carefully.\n"

	t.Run("real reasoning is never flagged", func(t *testing.T) {
		for _, name := range []string{"brief_short.txt", "brief_long_a.txt", "brief_long_b.txt"} {
			if at := firstRepeatFlag(readReasoningFixture(t, name)); at != -1 {
				t.Errorf("%s flagged at %d chars", name, at)
			}
		}
	})
	for name, text := range structured {
		t.Run(name+" is not a loop", func(t *testing.T) {
			if at := firstRepeatFlag(variedReasoning(1500) + text); at != -1 {
				t.Errorf("flagged at %d chars", at)
			}
		})
	}
	t.Run("long varied thinking is never flagged", func(t *testing.T) {
		if at := firstRepeatFlag(variedReasoning(60_000)); at != -1 {
			t.Errorf("flagged at %d chars", at)
		}
	})

	loops := []struct {
		name string
		loop string
	}{
		{"one sentence", cycle("Let me check the search results again to make sure I have everything. ", 6000)},
		{"paragraph cycle", cycle(paragraphs, 6000)},
	}
	for _, tc := range loops {
		t.Run("flags "+tc.name, func(t *testing.T) {
			at := firstRepeatFlag(prefix + tc.loop)
			if at == -1 {
				t.Fatal("loop was not flagged")
			}
			if limit := len(prefix) + 4096; at > limit {
				t.Errorf("flagged at %d chars, want within 4K of the loop start (%d)", at, limit)
			}
		})
	}

	t.Run("short text is never judged", func(t *testing.T) {
		checked := 0
		if reasoningRepeating(strings.Repeat("same line\n", 100), &checked) {
			t.Error("1K of text flagged")
		}
	})
	t.Run("only checks after the text has grown", func(t *testing.T) {
		text := cycle("Let me check the search results again to make sure I have everything. ", 6000)
		checked := 0
		if !reasoningRepeating(text, &checked) {
			t.Fatal("loop not flagged")
		}
		checked = len(text)
		if reasoningRepeating(text+"x", &checked) {
			t.Error("re-evaluated before the text grew by one step")
		}
	})
}

// A looping stream is cut within seconds of generation, long before the length
// ceiling, and the abort follows the same recovery as every other stuck stream.
func TestProcessStream_ReasoningLoopIsAborted(t *testing.T) {
	for _, skip := range []bool{false, true} {
		t.Run(fmt.Sprintf("skip_stuck_check=%v", skip), func(t *testing.T) {
			agent := newRepetitionTestAgent()
			agent.config.SkipStuckCheck = skip
			var fullMsg proxy.Message
			fullMsg.Role = proxy.AssistantRole

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ch := make(chan *proxy.ChatResponse, 1)
			go func() {
				defer close(ch)
				for sent := 0; sent < 200_000; sent += 40 {
					select {
					case <-ctx.Done():
						return
					case ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{ReasoningContent: "Let me check the results again. "}}}}:
					}
				}
			}()

			if err := agent.processStream(ctx, ch, &fullMsg, false, false); err != nil {
				t.Fatalf("processStream: %v", err)
			}
			if fullMsg.ReasoningContent != "[stuck]" {
				t.Errorf("reasoning loop was not aborted as stuck (got %d chars)", len(fullMsg.ReasoningContent))
			}
		})
	}
}

// Real thinking that runs past the old max_tokens-characters cut must finish.
func TestProcessStream_LongReasoningIsNotCut(t *testing.T) {
	agent := newRepetitionTestAgent()
	agent.config.MaxTokens = 8192
	agent.config.ReasoningBudget = 0
	text := variedReasoning(12_000)

	ch := make(chan *proxy.ChatResponse, 1)
	go func() {
		defer close(ch)
		for i := 0; i < len(text); i += 4 {
			ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{ReasoningContent: text[i:min(i+4, len(text))]}}}}
		}
		ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "answer"}}}}
	}()
	var fullMsg proxy.Message
	fullMsg.Role = proxy.AssistantRole
	if err := agent.processStream(context.Background(), ch, &fullMsg, false, false); err != nil {
		t.Fatalf("processStream: %v", err)
	}
	if fullMsg.Content != "answer" || len(fullMsg.ReasoningContent) != len(text) {
		t.Errorf("stream was cut: content %q, reasoning %d of %d chars", fullMsg.Content, len(fullMsg.ReasoningContent), len(text))
	}
}

// The duration cap bounds generation, not the wait for the first token: prompt
// processing on a busy local server can take half a minute or more.
func TestProcessStream_DurationCapStartsAtFirstToken(t *testing.T) {
	old := streamMaxDuration
	streamMaxDuration = 200 * time.Millisecond
	t.Cleanup(func() { streamMaxDuration = old })
	oldBeat := streamHeartbeatInterval
	streamHeartbeatInterval = 20 * time.Millisecond
	t.Cleanup(func() { streamHeartbeatInterval = oldBeat })

	agent := newRepetitionTestAgent()
	ch := make(chan *proxy.ChatResponse, 1)
	go func() {
		defer close(ch)
		time.Sleep(400 * time.Millisecond) // slower than the cap before any token
		ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: "late but whole"}}}}
	}()
	var fullMsg proxy.Message
	fullMsg.Role = proxy.AssistantRole
	if err := agent.processStream(context.Background(), ch, &fullMsg, false, false); err != nil {
		t.Fatalf("processStream: %v", err)
	}
	if fullMsg.Content != "late but whole" {
		t.Errorf("content = %q, want the late first token kept", fullMsg.Content)
	}
}

// A stream that never produces a token is still bounded, now by its own
// first-token timeout rather than the generation cap.
func TestProcessStream_FirstTokenTimeout(t *testing.T) {
	old := streamFirstTokenTimeout
	streamFirstTokenTimeout = 100 * time.Millisecond
	t.Cleanup(func() { streamFirstTokenTimeout = old })
	oldBeat := streamHeartbeatInterval
	streamHeartbeatInterval = 20 * time.Millisecond
	t.Cleanup(func() { streamHeartbeatInterval = oldBeat })

	run := func(class models.WorkloadClass) (terminated bool) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		agent := newRepetitionTestAgent()
		agent.config.WorkloadClass = class
		ch := make(chan *proxy.ChatResponse) // headers sent, no token ever
		done := make(chan error, 1)
		go func() {
			var fullMsg proxy.Message
			done <- agent.processStream(ctx, ch, &fullMsg, false, false)
		}()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("processStream: %v", err)
			}
			return true
		case <-time.After(600 * time.Millisecond):
			return false
		}
	}
	if !run(models.WorkloadCloud) {
		t.Error("a cloud stream with no token was not terminated")
	}
	if run(models.WorkloadLocal) {
		t.Error("a local stream was cut while waiting for its first token (prefill can be slow)")
	}
}

// After a local stream got stuck thinking, the retry answers without thinking
// (once), so it cannot loop in thinking again. Cloud providers are untouched.
func TestStuckThinkingRecovery(t *testing.T) {
	newAgent := func(class models.WorkloadClass) *Agent {
		a := newRepetitionTestAgent()
		a.config.WorkloadClass = class
		a.config.ProviderType = models.ProviderOpenAI
		a.config.ReasoningSpec = reasoning.ReasoningSpec{Mode: reasoning.ModeThinkTokens, Effort: reasoning.EffortMedium, Budget: 1800}
		a.config.ReasoningBudget = 1800
		return a
	}
	request := func(a *Agent) proxy.ChatRequest {
		var req proxy.ChatRequest
		a.applyRequestConfig(&req)
		return req
	}

	t.Run("local: the request after a stuck stream has thinking off, then thinking returns", func(t *testing.T) {
		a := newAgent(models.WorkloadLocal)
		if req := request(a); req.ThinkingBudgetTokens != 1800 || req.ChatTemplateKwargs != nil {
			t.Fatalf("normal request = %+v, want the thinking budget", req)
		}
		a.noteStuckThinking()
		req := request(a)
		if req.ChatTemplateKwargs == nil || req.ChatTemplateKwargs.EnableThinking == nil || *req.ChatTemplateKwargs.EnableThinking || req.ThinkingBudgetTokens != 0 {
			t.Errorf("recovery request = %+v, want enable_thinking=false and no budget", req)
		}
		if again := request(a); again.ThinkingBudgetTokens != 1800 || again.ChatTemplateKwargs != nil {
			t.Errorf("the request after recovery = %+v, want thinking back on", again)
		}
	})

	t.Run("cloud: a stuck stream never switches reasoning off", func(t *testing.T) {
		a := newAgent(models.WorkloadCloud)
		a.config.ReasoningSpec = reasoning.ReasoningSpec{Mode: reasoning.ModeEffort, Effort: reasoning.EffortMedium}
		a.noteStuckThinking()
		if req := request(a); req.ChatTemplateKwargs != nil {
			t.Errorf("cloud request = %+v, want reasoning untouched", req)
		}
	})
}

// The server enforces the think-token budget (and sends its wrap-up message); the
// client's own cut must be a backstop, not a second, tighter limit. With a 1820
// token budget, 5K characters of real thinking (about 1.3K tokens) must finish,
// while thinking far past the budget is cut and arms the no-thinking retry.
func TestProcessStream_ClientReasoningCutIsABackstop(t *testing.T) {
	newAgent := func() *Agent {
		a := newRepetitionTestAgent()
		a.config.WorkloadClass = models.WorkloadLocal
		a.config.MaxTokens = 5461
		a.config.ReasoningBudget = 1820
		a.deps.Orchestrator = &orchestrator.Orchestrator{Interceptor: orchestrator.NewStreamInterceptor(nil, orchestrator.NewReasoningNormalizer())}
		return a
	}
	feed := func(reasoning string, content string) <-chan *proxy.ChatResponse {
		ch := make(chan *proxy.ChatResponse, 1)
		go func() {
			defer close(ch)
			for i := 0; i < len(reasoning); i += 4 {
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{ReasoningContent: reasoning[i:min(i+4, len(reasoning))]}}}}
			}
			if content != "" {
				ch <- &proxy.ChatResponse{Choices: []proxy.Choice{{Delta: proxy.Message{Content: content}}}}
			}
		}()
		return ch
	}

	t.Run("thinking within what the server allows is not cut", func(t *testing.T) {
		a := newAgent()
		var msg proxy.Message
		msg.Role = proxy.AssistantRole
		if err := a.processStream(context.Background(), feed(variedReasoning(6000), "answer"), &msg, false, false); err != nil {
			t.Fatalf("processStream: %v", err)
		}
		if msg.Content != "answer" {
			t.Errorf("content = %q, reasoning %d chars: the stream was cut before the answer", msg.Content, len(msg.ReasoningContent))
		}
		if a.answerWithoutThinking.Load() {
			t.Error("no recovery should be armed for a stream that finished")
		}
	})

	t.Run("thinking far past the budget is cut and arms the no-thinking retry", func(t *testing.T) {
		a := newAgent()
		var msg proxy.Message
		msg.Role = proxy.AssistantRole
		if err := a.processStream(context.Background(), feed(variedReasoning(30000), ""), &msg, false, false); err != nil {
			t.Fatalf("processStream: %v", err)
		}
		if len(msg.ReasoningContent) >= 30000 {
			t.Error("a runaway stream was not cut")
		}
		if !a.answerWithoutThinking.Load() {
			t.Error("the cut must arm the answer-without-thinking retry")
		}
	})
}
