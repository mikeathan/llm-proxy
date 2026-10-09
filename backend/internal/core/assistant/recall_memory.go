package assistant

import (
	"context"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/models"
)

// Per-turn recall (SPEC-004 §4.1): in the operator's own chat, the run's user message is searched against saved
// facts once per run, and the best matches are appended to that message in the request copy. No model call, no
// tool call; the stored history and the head system message never change.
const (
	recallMaxFacts = 3
	// recallSearchLimit leaves room to skip facts already in the hot block.
	recallSearchLimit = recallMaxFacts * 3
	// recallDateLen is the date part ("2026-10-08") of a stored timestamp.
	recallDateLen = len("2006-01-02")
)

// recallFacts returns the best-ranked facts for message, skipping ids already injected, at most recallMaxFacts.
func recallFacts(ctx context.Context, store *memory.Store, workspaceID, message string, skip map[int64]bool) ([]memory.MemoryEntry, error) {
	query := memory.RecallQuery(message)
	if query == "" {
		return nil, nil
	}
	found, err := store.Search(ctx, workspaceID, query, recallSearchLimit, memory.SearchOption{SearchAllWorkspaces: true})
	if err != nil {
		return nil, err
	}
	out := make([]memory.MemoryEntry, 0, recallMaxFacts)
	for _, e := range found {
		if len(out) == recallMaxFacts {
			break
		}
		if !skip[e.ID] {
			out = append(out, e)
		}
	}
	return out, nil
}

// recallFactLine is one recalled fact with its last-updated date, so the model can prefer the newer of two.
func recallFactLine(e memory.MemoryEntry) string {
	date := e.UpdatedAt
	if len(date) > recallDateLen {
		date = date[:recallDateLen]
	}
	return "- [" + date + "] " + factText(e)
}

// renderRecall renders the leading facts that fit budget chars; kept are the facts in the block.
func renderRecall(entries []memory.MemoryEntry, budget int) (block string, kept []memory.MemoryEntry) {
	var lines []string
	for _, e := range entries {
		candidate := prompts.RecalledMemoryBlock(append(lines, recallFactLine(e)))
		if len(candidate) > budget {
			break
		}
		lines = append(lines, recallFactLine(e))
		block, kept = candidate, append(kept, e)
	}
	return block, kept
}

// recallBudget is what the hot memory cap leaves after the hot block: hot plus recall never exceed the cap.
func recallBudget(contextBudget int, workload models.WorkloadClass, hotBlock string) int {
	return hotMemoryCharBudget(contextBudget, workload) - len(hotBlock)
}

// selectRecall is the single code path from a message to its recall block, shared by the run and the preview.
func selectRecall(ctx context.Context, store *memory.Store, workspaceID, message string, skip map[int64]bool, budget int) (string, []memory.MemoryEntry, error) {
	entries, err := recallFacts(ctx, store, workspaceID, message, skip)
	if err != nil {
		return "", nil, err
	}
	block, kept := renderRecall(entries, budget)
	return block, kept, nil
}

// RecallPreview is the recall block a chat message would get, and which facts are in it.
type RecallPreview struct {
	Block string  `json:"block"`
	IDs   []int64 `json:"ids"`
}

// PreviewRecall shows what a chat run configured by opts would recall for message, given the hot block it would
// carry (hot facts are skipped and share the budget). Only ContextBudget and WorkloadClass of opts are read.
func PreviewRecall(ctx context.Context, store *memory.Store, workspaceID, message string, hot HotMemoryPreview, opts AgentOptions) (RecallPreview, error) {
	skip := make(map[int64]bool, len(hot.Included))
	for _, e := range hot.Included {
		skip[e.ID] = true
	}
	block, kept, err := selectRecall(ctx, store, workspaceID, message, skip, recallBudget(opts.ContextBudget, opts.WorkloadClass, hot.Block))
	if err != nil {
		return RecallPreview{}, err
	}
	return RecallPreview{Block: block, IDs: recallIDs(kept)}, nil
}

// snapshotRecall freezes the run's recall block, after the hot snapshot so it can skip the hot facts.
func (a *Agent) snapshotRecall(ctx context.Context) {
	if a.runS == nil || !a.operatorMemoryChat() {
		return
	}
	anchor := runUserMessage(a.runS.history)
	if anchor == "" {
		return
	}
	budget := recallBudget(a.config.ContextBudget, a.config.WorkloadClass, a.runS.prompt.memoryBlock)
	block, kept, err := selectRecall(ctx, a.deps.MemoryStore, a.config.WorkspaceID, anchor, a.runS.prompt.hotIDs, budget)
	if err != nil {
		if a.deps.Logger != nil {
			a.deps.Logger.Warn("memory recall failed; continuing without it", "error", err)
		}
		return
	}
	if block == "" {
		return
	}
	a.runS.prompt.recallBlock, a.runS.prompt.recallAnchor = block, anchor
	a.deps.MemoryStore.RecordSearched(kept)
	if a.deps.Logger != nil {
		a.deps.Logger.Info("memory recalled for chat turn", "workspace", a.config.WorkspaceID, "facts", len(kept), "ids", recallIDs(kept))
	}
}

func recallIDs(entries []memory.MemoryEntry) []int64 {
	ids := make([]int64, len(entries)) // never nil, so the preview reports [] rather than null
	for i, e := range entries {
		ids[i] = e.ID
	}
	return ids
}

// runUserMessage is the run's user message: the last message when the run starts, if it is the user's.
func runUserMessage(history []proxy.Message) string {
	if n := len(history); n > 0 && history[n-1].Role == proxy.UserRole {
		return history[n-1].Content
	}
	return ""
}

// injectRecall appends the frozen recall block to the run's user message in a copy of the request. It finds that
// message by role and content (the sieve's nags are user messages too) and skips recall if the sieve dropped it.
func (a *Agent) injectRecall(prepared []proxy.Message) []proxy.Message {
	if a.runS == nil || a.runS.prompt.recallBlock == "" {
		return prepared
	}
	for i := len(prepared) - 1; i >= 0; i-- {
		if prepared[i].Role == proxy.UserRole && prepared[i].Content == a.runS.prompt.recallAnchor {
			out := append([]proxy.Message(nil), prepared...)
			out[i].Content += "\n\n" + a.runS.prompt.recallBlock
			return out
		}
	}
	return prepared
}
