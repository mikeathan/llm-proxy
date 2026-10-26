# Agent Loop — Execution, Sieve, Stuck Detection & Fallback — Natural Completion, Control Plane and write_file Salvage

Reference for the [`agent-loop`](../SKILL.md) skill, loaded on demand. A run finishes too early, never finishes, or a truncated write is salvaged.

## Natural Completion vs Control Plane

Completion (Hermes-aligned, Phase 2b): a content-only assistant message with
at least one tool result anywhere in the run's history → done. The model signals
completion by writing plain text without further tool calls. Reasoning-only
interleaves (empty content, large `ReasoningContent`) do not block completion.

**Invisible reasoning in content:** Local models (Qwen3, Ollama) may emit
`<think>`/`<reasoning>` blocks inline in `content`. `stripThinkBlocks` removes
these blocks before evaluating completion. If nothing substantive remains after
stripping, the turn is NOT a final answer.

**Embedded tool calls in reasoning:** Some models (Qwen3) write `<tool_call>`
blocks inside `reasoning_content` instead of using native tool call deltas.
`tryExtractToolCallFromReasoning()` (stream.go) extracts these tool calls
directly into `fullMsg.ToolCalls` without copying reasoning text into `Content`.
The llama.cpp server already separates `reasoning_content` from `content` at
the wire level — reasoning text stays in `ReasoningContent` and is never
promoted to visible output. This prevents a feedback loop where the model
sees its own thinking text in conversation history and reuses it on subsequent
turns, wasting tokens and producing duplicated output.

Recovery messages (nags, sieve notes, parse feedback, stuck placeholders) are
**control plane** — `IsAgentControlMessage` identifies them so they are never
confused with real conversation. See `checkTaskCompletion` in `session.go`.

**Content-with-tools fallback:** When the model writes visible text AND calls
tools in the same turn, and every call is housekeeping (`hasOnlyHousekeepingTools`
→ `toolpolicy.EffectHousekeeping`: `memory_update`, `automation_journal`), the text
is saved (`lastContentWithTools`). If the next turn is empty, the saved text is used
as the final answer. This handles the common pattern where a model delivers its
report while calling `memory_update`. Text beside a substantive tool (`write_file`,
terminal, reads) is mid-task narration and is not saved, and such a turn clears any
text saved earlier (`noteContentWithTools`). Until 2026-10-08 the housekeeping switch
matched no tool, so this path never fired in production.
A natural-completion turn that is at least `signOffRatio` (2x) shorter than the saved text is a sign-off, so
`keepReportOverSignOff` keeps the saved report (a playbook's "save to memory before you answer" puts the report beside
the `memory_update` call; the next turn is a one-line "done"). A comparable or longer final answer wins.

**One-shot nag:** Empty turns after tool results trigger a single
`AutomationNagPrompt` injection. If the next turn is still empty, a fallback
(fallback content or best-available assistant text) finishes the run — the
agent does not nag perpetually.

**Finalization-turn failure fallback:** When the tools-disabled `finalizeReport`
turn fails (LLM/provider error) or returns empty, the fallback chain is
`bestAvailableAnswer()` (last substantive assistant text) → `synthesizeRunSummary()`
(a degraded-but-real summary of the run's tool activity: per-tool call counts +
recorded `{"error": ...}` failures) → error only when the run did no tool work.
This is what keeps plan_execute runs alive when a provider outage hits the report
turn — their history is pure tool calls, so there is no assistant text to salvage.

## Truncated write_file salvage

When `write_file` / `append_file` args JSON is truncated mid-stream, the agent:
1. Recovers `content` (≥100 chars) from partial args
2. Best-effort extracts `path` and persists via Engine (guardrails + FS)
3. Completes with the recovered report even if path is missing or persist fails

No path is invented. See `salvageTruncatedWrite` in `tool_exec.go`.
