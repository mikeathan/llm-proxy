package assistant

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/models"
)

// Hot-memory sizing. The cap is a share of the model's resolved context budget
// (SPEC-005 §II.3: derived from the probed serving window for local models), so
// an 8K, 16K and 32K model each get a proportionate block and a 128K cloud
// model is bounded by the max. Cloud gets the smaller share: it pays per token.
const (
	hotMemoryShareLocal    = 0.08
	hotMemoryShareCloud    = 0.05
	hotMemoryMinChars      = 400  // floor so a tiny window still carries a fact
	hotMemoryMaxChars      = 6000 // ceiling so a huge window is not flooded
	hotMemoryFallbackChars = 2000 // used when ContextBudget is unresolved (<= 0)
	hotMemoryMinFactRoom   = 80   // below this much room a clipped fact is noise: show only the overflow hint

	// hotMemoryFactsReserve is the share of the block the saved facts may claim before the operator notes: the notes outrank
	// the facts, but a block of notes alone would push every fact out. The facts reserve only what they need up to this
	// share, and the notes get the rest (all of it when there are no facts).
	hotMemoryFactsReserve = 0.25
	notesSeparator        = "\n\n" // between the global and the workspace notes (OperatorNotes.Text)

	// hotMemoryCharsPerToken is the tokenizer ratio the context budgets use
	// (SPEC-005 §II.3), for the preview's token estimate.
	hotMemoryCharsPerToken = 4

	hotMemoryOpenTag  = "<memory>\n"
	hotMemoryCloseTag = "\n</memory>"
)

// hotMemoryCharBudget returns the character cap for the injected <memory>
// block, derived from the model's context budget and workload class.
func hotMemoryCharBudget(contextBudget int, workload models.WorkloadClass) int {
	share := hotMemoryShareCloud
	if workload == models.WorkloadLocal {
		share = hotMemoryShareLocal
	}
	return contextShare(contextBudget, share, hotMemoryMinChars, hotMemoryMaxChars, hotMemoryFallbackChars)
}

// snapshotHotMemory freezes the run's <memory> block. It is taken once per
// Execute so every request carries byte-identical memory: the llama.cpp KV
// cache is reused only for an identical prompt prefix, and a block that moved
// or changed per turn would force the server to re-process everything after it.
// Facts saved mid-run become visible on the next run.
func (a *Agent) snapshotHotMemory(ctx context.Context) {
	if a.runS == nil || !a.config.EnableHotMemory || a.deps.MemoryStore == nil {
		return
	}
	entries, err := a.deps.MemoryStore.SearchHot(ctx, a.config.WorkspaceID)
	if err != nil {
		if a.deps.Logger != nil {
			a.deps.Logger.Warn("hot memory snapshot failed; running without memory", "error", err)
		}
		return
	}
	// A notes read failure must not cost the run its saved facts: carry on with
	// whatever was read (the store returns empty notes on error).
	notes, err := a.deps.MemoryStore.OperatorNotes(ctx, a.config.WorkspaceID)
	if err != nil && a.deps.Logger != nil {
		a.deps.Logger.Warn("operator notes unreadable; running without them", "error", err)
	}
	block, kept := a.buildHotMemoryBlock(entries, notes)
	a.runS.prompt.memoryBlock = block
	a.runS.prompt.hotIDs = make(map[int64]bool, kept)
	for _, e := range entries[:kept] {
		a.runS.prompt.hotIDs[e.ID] = true
	}
	// Count the facts that were really sent (not the ones the budget cut), once
	// per run. In-memory only: a background flusher writes it, never this path.
	a.deps.MemoryStore.RecordInjected(entries[:kept])
}

// buildHotMemoryBlock wraps the operator notes and the entries that fit the
// context-derived budget in <memory> tags, or returns "" when there is nothing
// to inject; kept is how many leading entries made it in.
func (a *Agent) buildHotMemoryBlock(entries []memory.MemoryEntry, notes memory.OperatorNotes) (block string, kept int) {
	r := renderHotMemory(hotMemoryInput{
		entries: entries, notes: notes,
		contextBudget: a.config.ContextBudget, workload: a.config.WorkloadClass,
	})
	return r.block, r.kept
}

// hotMemoryInput is everything that decides the injected block.
type hotMemoryInput struct {
	entries       []memory.MemoryEntry // newest first, as SearchHot returns them
	notes         memory.OperatorNotes // operator notes: global, then workspace
	contextBudget int
	workload      models.WorkloadClass
}

// hotMemoryRender is the rendered <memory> block: how many leading entries are in it and how much of the operator notes
// did not fit.
type hotMemoryRender struct {
	block    string
	kept     int
	notesCut int
}

// renderHotMemory is the single code path that turns notes and hot entries into
// the injected block: the agent and the operator preview both call it, so the
// preview cannot drift from what a run receives.
//
// The block never exceeds the model's memory cap. Operator notes come first and outrank the saved facts, but they
// share the cap: the facts reserve the room they need up to hotMemoryFactsReserve of it, the notes take the rest, and
// notes that do not fit are cut with a visible hint (fitNotes). The facts fill what the notes leave.
func renderHotMemory(in hotMemoryInput) hotMemoryRender {
	notesText := in.notes.Text()
	if notesText == "" && len(in.entries) == 0 {
		return hotMemoryRender{}
	}
	budget := hotMemoryCharBudget(in.contextBudget, in.workload) - len(hotMemoryOpenTag) - len(hotMemoryCloseTag)

	var b strings.Builder
	b.WriteString(hotMemoryOpenTag)
	var r hotMemoryRender
	if notesText != "" {
		section, cut := renderNotesSection(in.notes, budget-factsReserve(in.entries, budget))
		b.WriteString(section)
		budget -= len(section)
		r.notesCut = cut
	}
	if len(in.entries) > 0 {
		if notesText != "" {
			header := "\n" + prompts.HotMemorySavedHeader + "\n"
			b.WriteString(header)
			budget -= len(header)
		}
		var facts string
		if notesText != "" && budget < hotMemoryMinFactRoom {
			facts = fmt.Sprintf(prompts.HotMemoryOverflowHint, len(in.entries))
		} else {
			facts, r.kept = buildHotInjection(in.entries, budget)
		}
		b.WriteString(strings.TrimSuffix(facts, "\n"))
	}
	b.WriteString(hotMemoryCloseTag)
	r.block = b.String()
	return r
}

// factsReserve is the room the saved facts keep before the operator notes are fitted: what the facts need (their lines
// and the "Saved facts:" header), at most hotMemoryFactsReserve of the block. No facts, no reserve.
func factsReserve(entries []memory.MemoryEntry, budget int) int {
	if len(entries) == 0 {
		return 0
	}
	need := len("\n" + prompts.HotMemorySavedHeader + "\n")
	for _, e := range entries {
		need += len(hotFactLine(e))
	}
	return min(need, int(float64(budget)*hotMemoryFactsReserve))
}

// renderNotesSection is the operator-notes section within budget characters (the room left for it in the block).
func renderNotesSection(notes memory.OperatorNotes, budget int) (section string, cut int) {
	header := prompts.HotMemoryOperatorHeader + "\n"
	text, cut := fitNotes(notes, budget-len(header)-len("\n"))
	return header + text + "\n", cut
}

// fitNotes fits the operator notes (global, then workspace) into room characters. Notes that fit are returned whole.
// Otherwise each file keeps a fair share of the room (a short file is kept whole and the other gets the rest), is cut at
// a line boundary, and a hint says how many characters are not shown; cut is that count. The result stays within room
// unless room is smaller than the hint itself.
func fitNotes(notes memory.OperatorNotes, room int) (text string, cut int) {
	var parts []string
	rawLen := 0
	for _, t := range []string{notes.Global, notes.Workspace} {
		if t = strings.TrimSpace(t); t != "" {
			parts = append(parts, t)
			rawLen += len(t)
		}
	}
	whole := strings.Join(parts, notesSeparator)
	if len(whole) <= room {
		return whole, 0
	}
	hintReserve := len(fmt.Sprintf(prompts.HotMemoryNotesCutHint, 999999)) + len("\n")
	shares := notesShares(parts, max(room-hintReserve-len(notesSeparator)*(len(parts)-1), 0))
	var shown []string
	shownLen := 0
	for i, p := range parts {
		if s := clipAtLine(p, shares[i]); s != "" {
			shown = append(shown, s)
			shownLen += len(s)
		}
	}
	cut = rawLen - shownLen
	hint := fmt.Sprintf(prompts.HotMemoryNotesCutHint, cut)
	if len(shown) == 0 {
		return hint, cut
	}
	return strings.Join(shown, notesSeparator) + "\n" + hint, cut
}

// notesShares splits avail characters between the notes files: a file that fits in half is kept whole and the other
// gets the rest; two long files get half each.
func notesShares(parts []string, avail int) []int {
	if len(parts) < 2 {
		return []int{avail}
	}
	half := avail / 2
	switch {
	case len(parts[0]) <= half:
		return []int{len(parts[0]), avail - len(parts[0])}
	case len(parts[1]) <= half:
		return []int{avail - len(parts[1]), len(parts[1])}
	}
	return []int{half, avail - half}
}

// clipAtLine returns at most limit bytes of s, cut at the last line break (or, for a single long line, the last space) so
// no line is left half-written, and never inside a multi-byte character.
func clipAtLine(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	head := s[:limit]
	if i := strings.LastIndex(head, "\n"); i > 0 {
		return strings.TrimRight(head[:i], " \t")
	}
	if i := strings.LastIndexByte(head, ' '); i > 0 {
		return head[:i]
	}
	return head
}

// HotMemoryPreviewEntry is one hot fact in a preview.
type HotMemoryPreviewEntry struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Chars    int    `json:"chars"`
	Priority int    `json:"priority"`
}

// HotMemoryPreview is what a run with the given model settings would receive.
type HotMemoryPreview struct {
	Block              string                  `json:"block"`
	Chars              int                     `json:"chars"`
	TokensEstimate     int                     `json:"tokens_estimate"`
	BudgetChars        int                     `json:"budget_chars"`         // cap for the block
	ContextBudgetChars int                     `json:"context_budget_chars"` // the model's context budget; 0 = unresolved
	OperatorChars      int                     `json:"operator_chars"`       // size of the operator notes (all of them, shown or not)
	NotesCutChars      int                     `json:"notes_cut_chars"`      // how much of the operator notes did not fit the budget
	Included           []HotMemoryPreviewEntry `json:"included"`
	Cut                []HotMemoryPreviewEntry `json:"cut"`
}

// PreviewHotMemory renders what a run configured by opts would inject for the
// given operator notes and hot entries (newest first, as SearchHot returns
// them), and which entries made it in. Only ContextBudget and WorkloadClass of
// opts are read.
func PreviewHotMemory(entries []memory.MemoryEntry, notes memory.OperatorNotes, opts AgentOptions) HotMemoryPreview {
	r := renderHotMemory(hotMemoryInput{
		entries: entries, notes: notes,
		contextBudget: opts.ContextBudget, workload: opts.WorkloadClass,
	})
	block, kept := r.block, r.kept
	budget := hotMemoryCharBudget(opts.ContextBudget, opts.WorkloadClass)
	p := HotMemoryPreview{
		Block:              block,
		Chars:              len(block),
		TokensEstimate:     len(block) / hotMemoryCharsPerToken,
		BudgetChars:        budget,
		OperatorChars:      len(notes.Text()),
		NotesCutChars:      r.notesCut,
		ContextBudgetChars: max(opts.ContextBudget, 0),
		Included:           []HotMemoryPreviewEntry{},
		Cut:                []HotMemoryPreviewEntry{},
	}
	for i, e := range entries {
		item := HotMemoryPreviewEntry{ID: e.ID, Title: e.Title, Chars: len(hotFactLine(e)), Priority: e.Priority}
		if i < kept {
			p.Included = append(p.Included, item)
		} else {
			p.Cut = append(p.Cut, item)
		}
	}
	return p
}

// injectActiveMemory appends the run's frozen memory block to the head system
// message (creating one if the history has none). It is idempotent and pure:
// it reads the snapshot and never queries the store, so it is safe to call for
// sieve measurement as well as for the real request.
func (a *Agent) injectActiveMemory(prepared []proxy.Message) []proxy.Message {
	text := a.memorySystemText()
	if text == "" {
		return prepared
	}
	out := append([]proxy.Message(nil), prepared...)
	if len(out) > 0 && out[0].Role == proxy.SystemRole {
		out[0].Content += "\n\n" + text
		return out
	}
	return append([]proxy.Message{{Role: proxy.SystemRole, Content: text}}, out...)
}

// memorySystemText is what memory adds to the head system message: the run's frozen <memory> block, then — for the
// operator's own assistant chats — the save guidance. Both are fixed for the run, so the head stays byte-identical.
func (a *Agent) memorySystemText() string {
	if a.runS == nil {
		return ""
	}
	var parts []string
	if block := a.runS.prompt.memoryBlock; block != "" {
		parts = append(parts, block)
	}
	if a.guidesMemorySaves() {
		parts = append(parts, prompts.MemorySaveGuidance)
	}
	return strings.Join(parts, "\n\n")
}

// guidesMemorySaves reports whether the model should be told when to save: memory is on and there is a store, the agent
// serves the assistant channel, and the conversation is the operator's own — a connector chat is an outside sender
// whose messages must not steer what gets remembered.
func (a *Agent) guidesMemorySaves() bool {
	return a.operatorMemoryChat() && a.config.Profile.MemoryGuidance
}

// operatorMemoryChat is the one predicate for the operator's own assistant chat with memory on and a store: it
// gates both the save guidance and per-turn recall (SPEC-004 §4.1).
func (a *Agent) operatorMemoryChat() bool {
	return a.config.EnableHotMemory && a.deps.MemoryStore != nil &&
		a.config.Channel == ChannelAssistant &&
		models.SessionSource(a.config.ConversationID) == models.SessionSourceManual
}

// hotFactLine renders one fact as a bullet. A fact saved without an explicit title
// has its first characters as the title, so printing "Title: Content" would say
// the same words twice and double the cost of every short fact on a small window:
// the title is shown only when it adds something (an explicit, different title).
func hotFactLine(e memory.MemoryEntry) string {
	return "- " + factText(e) + "\n"
}

// factText is a fact as the model reads it, shared by the hot block and recall: "Title: Content", or just the
// content when the title is empty or auto-derived from it.
func factText(e memory.MemoryEntry) string {
	title := strings.TrimSpace(e.Title)
	if title == "" || strings.HasPrefix(e.Content, title) {
		return e.Content
	}
	return title + ": " + e.Content
}

// buildHotInjection formats entries (newest first) as "- Title: Content" lines
// within maxChars and the number of leading entries that made it in. When
// entries are cut it ends with an overflow hint so the model knows facts were
// dropped and how to fetch them. The newest fact is always kept (truncated if it
// alone exceeds the budget): a silent empty block is worse than a clipped fact.
func buildHotInjection(entries []memory.MemoryEntry, maxChars int) (string, int) {
	if len(entries) == 0 {
		return "", 0
	}
	lines := make([]string, len(entries))
	total := 0
	for i, e := range entries {
		lines[i] = hotFactLine(e)
		total += len(lines[i])
	}
	if total <= maxChars {
		return strings.Join(lines, ""), len(entries)
	}

	hintLen := len(fmt.Sprintf(prompts.HotMemoryOverflowHint, len(entries)))
	var b strings.Builder
	kept := 0
	for _, line := range lines {
		if b.Len()+len(line)+hintLen > maxChars {
			break
		}
		b.WriteString(line)
		kept++
	}
	if kept == 0 {
		room := max(maxChars-hintLen-len("…\n"), 0)
		b.WriteString(strings.ToValidUTF8(lines[0][:min(room, len(lines[0]))], "") + "…\n")
		kept = 1
	}
	b.WriteString(fmt.Sprintf(prompts.HotMemoryOverflowHint, len(entries)-kept))
	return b.String(), kept
}
