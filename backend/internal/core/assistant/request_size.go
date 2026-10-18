package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"

	"llm-proxy/internal/core/llm/providers"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
)

// shrinkFloorChars is the least a message is cut to when a history is made to fit, so every kept result stays readable.
const shrinkFloorChars = 256

// requestSize is one prepared request measured against what the model may take, in characters.
type requestSize struct {
	chars   int // the whole request: its messages and, for a local native-tools request, the tool schemas
	history int // the conversation's own messages, before the prompt is enriched
	limit   int // what the request may carry; 0 means no limit is known
}

func (s requestSize) over() bool { return s.limit > 0 && s.chars > s.limit }

// overhead is what the request carries beyond the conversation: the system prompt's additions and the tool schemas.
func (s requestSize) overhead() int { return max(s.chars-s.history, 0) }

func (a *Agent) isLocalWorkload() bool { return a.config.WorkloadClass == models.WorkloadLocal }

// requestLimit is the characters a request may carry. On a local model ContextBudget is derived from the serving window
// (window minus the output reserve, at 4 characters a token), so it is scaled by what the model's tokens really cost:
// dense tool output (JSON, HTML) costs more tokens per character than prose, and the nominal figure let a 16k window
// overflow. On a cloud model ContextBudget is a history allowance chosen for cost, not a window, and is used as given.
func (a *Agent) requestLimit() int {
	budget := a.config.ContextBudget
	if budget <= 0 || !a.isLocalWorkload() {
		return max(budget, 0)
	}
	var ratios *orchestrator.TokenRatios
	if a.deps.Orchestrator != nil {
		ratios = a.deps.Orchestrator.Ratios
	}
	return int(math.Round(float64(budget) * ratios.CharsPerToken(a.config.ModelName) / orchestrator.LocalCharsPerToken))
}

// measureRequest sizes the request a turn would send for history: the prepared messages plus, when the model takes its
// tools natively, their schemas, which travel in the tools field and are rendered into the prompt by the server. A cloud
// request is measured by its messages alone, the basis its allowance is stated on.
func (a *Agent) measureRequest(history []proxy.Message, tools []proxy.Tool) requestSize {
	if a.config.ContextBudget <= 0 {
		return requestSize{}
	}
	llmTools := tools
	if !a.config.UseNativeTools {
		llmTools = nil
	}
	prepared, _ := a.prepareMessagesForTurn(history, tools, llmTools)
	size := requestSize{chars: historyMessagesChars(prepared), history: historyMessagesChars(history), limit: a.requestLimit()}
	if a.isLocalWorkload() {
		size.chars += toolSchemaChars(llmTools)
	}
	return size
}

// sentRequestChars is the size of a request as measured for learning its token ratio: everything the server counts.
func sentRequestChars(req proxy.ChatRequest) int {
	return historyMessagesChars(req.Messages) + toolSchemaChars(req.Tools)
}

func historyMessagesChars(messages []proxy.Message) int {
	total := 0
	for _, m := range messages {
		total += messageChars(m)
	}
	return total
}

// toolSchemaChars is the size of the tool definitions a native-tools request carries in its tools field (about 6 KB
// for the twelve built-in tools).
func toolSchemaChars(tools []proxy.Tool) int {
	if len(tools) == 0 {
		return 0
	}
	b, err := json.Marshal(tools)
	if err != nil {
		return 0
	}
	return len(b)
}

// sendStream and sendChat are the one place a request leaves the agent: a local request's size is noted on the way out
// so the server's count of it can be matched to it when it comes back.
func (a *Agent) sendStream(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
	a.noteSent(req)
	return a.deps.Client.Stream(ctx, req)
}

func (a *Agent) sendChat(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
	a.noteSent(req)
	return a.deps.Client.Chat(ctx, req)
}

func (a *Agent) noteSent(req proxy.ChatRequest) {
	if a.isLocalWorkload() {
		a.sentChars.Store(int64(sentRequestChars(req)))
	}
}

// recordUsage adds a call's reported tokens to the run's tracker and learns the model's token ratio from them.
func (a *Agent) recordUsage(ctx context.Context, usage *proxy.TokenUsage) {
	recordReportedUsage(ctx, usage)
	a.learnTokenRatio(int(a.sentChars.Load()), usage)
}

// learnTokenRatio teaches the orchestrator how many characters of this model's requests cost one token, from the
// server's own count of a request of sentChars characters. Only a local model is measured: its budget is a window.
func (a *Agent) learnTokenRatio(sentChars int, usage *proxy.TokenUsage) {
	if usage == nil || !a.isLocalWorkload() || a.deps.Orchestrator == nil {
		return
	}
	a.deps.Orchestrator.Ratios.Observe(a.config.ModelName, sentChars, usage.PromptTokens)
}

// shrinkHistory cuts a history down to target characters, oldest first, and reports whether it fits. The locked head
// is never cut, nor are the sieve's own note, ledger and warning, and the newest message is cut last, only when
// nothing older can give: the model is about to read it. Messages are cut to a floor, head and tail around a marker; tool-call arguments count towards the size but are never
// cut, so they stay valid JSON. The caller's slice is not modified.
func shrinkHistory(history []proxy.Message, target int) ([]proxy.Message, bool) {
	total := historyMessagesChars(history)
	if total <= target {
		return history, true
	}
	out := slices.Clone(history)
	last := len(out) - 1
	for i := sieveLockedHead; i <= last && total > target; i++ {
		if !isSieveMessage(out[i]) {
			total -= cutMessage(&out[i], total-target)
		}
	}
	return out, total <= target
}

// cutMessage removes about want characters from a message's content, then its reasoning, never below the floor, and
// returns how many it removed.
func cutMessage(m *proxy.Message, want int) int {
	before := messageChars(*m)
	m.Content = cutText(m.Content, want)
	if removed := before - messageChars(*m); removed < want {
		m.ReasoningContent = cutText(m.ReasoningContent, want-removed)
	}
	return before - messageChars(*m)
}

// cutText shortens s by about want characters, counting the marker that replaces the middle, and leaves it alone when
// that would take it below the floor or gain nothing.
func cutText(s string, want int) string {
	limit := max(len(s)-want-len(sieveTruncationMarker), shrinkFloorChars)
	if limit+len(sieveTruncationMarker) >= len(s) {
		return s
	}
	return truncateLongContent(s, limit)
}

// shrinkToReportedWindow answers a local model's typed overflow rejection. The server said how many tokens the request
// was and how many its window holds, and the agent knows how many characters it sent, so the target is exact: the sent
// size scaled to the window less the output reserve, less what the request carries beyond the conversation. The
// rejection also teaches the model's token ratio. handled is false when the rejection carries no counts or the model
// is not local, and the caller then uses the fixed-tail sieves. When the part that cannot be cut already fills the
// window the run ends with ErrPromptTooLarge: retrying could only be rejected again.
func (s *runSession) shrinkToReportedWindow(cause error) (handled bool, err error) {
	var overflow *providers.ContextOverflowError
	a := s.agent
	sent := int(a.sentChars.Load())
	if !errors.As(cause, &overflow) || overflow.PromptTokens <= 0 || overflow.WindowTokens <= 0 || sent <= 0 || !a.isLocalWorkload() {
		return false, nil
	}
	a.learnTokenRatio(sent, &proxy.TokenUsage{PromptTokens: overflow.PromptTokens})
	reserve := min(a.config.MaxTokens, overflow.WindowTokens/2)
	request := int(float64(sent) * float64(overflow.WindowTokens-reserve) / float64(overflow.PromptTokens))
	target := request - max(sent-historyMessagesChars(s.history), 0)
	shrunk, fits := shrinkHistory(s.history, target)
	if target <= 0 || !fits {
		return true, fmt.Errorf("%w: the system prompt, task and tools leave no room in the %d-token window (the request was %d tokens)",
			models.ErrPromptTooLarge, overflow.WindowTokens, overflow.PromptTokens)
	}
	s.history = shrunk
	return true, nil
}
