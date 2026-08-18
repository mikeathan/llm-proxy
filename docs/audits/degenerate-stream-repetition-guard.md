---
status: complete
last_reviewed: 2026-08-20
---

# Audit: Degenerate Stream Repetition Loop — Content Guard & Duration Cap

## Symptom

The `workspace-health-test` automation (model `deepseek-v4-flash-0731` on NVIDIA,
native tools, `reasoning_budget 0`) streamed garbage for ~49s with no progress
until the user manually stopped it. The run produced 193 events: 182 `tool_stream`
events that were byte-identical repeated fragments, zero parsed tool calls, and no
completion. `run-meta.json` ended with `error: agent execution failed: context canceled`.

## Root Cause (model behavior, not our grammar)

1. The model emitted a tool-call dialect `<konjll invoke name="...">` that matches
   **no** parser (`<tool>`, `<function name=>`, or native OpenAI-style `tool_calls`).
   The `<konjll>` string appears nowhere in the codebase — fully hallucinated.
2. After 3 malformed tool blocks it degenerated into ~189 repeated `</konjll>`
   closing tags (~2,539 chars, ~55 chars/sec, `tool_calls 0`).
3. **Not our grammar logic:** the GBNF grammar constraint is only applied for local
   llama.cpp providers. NVIDIA is cloud (`ProviderNVIDIA` = `ToolCallFormat: native`,
   `Prefill: false`), so no grammar was active. The model was given proper native
   `tools` schemas (`has_tools: true`) and still chose a made-up text dialect.
4. The earlier `dev-test` run used the same model + native tools successfully — the
   failure is model nondeterminism, not plumbing.

## Why the existing guards missed it

- `checkStreamStuck` (stream.go) bails when `Content` is non-empty — the intro text
  ("I'll execute the workspace health audit...") counted as content, so reasoning
  stuck/spiral detection never ran.
- `tryExtractToolCallFromReasoning` also bails on non-empty content and only matches
  `<tool_call>` — the malformed `<konjll>` was never parsed.
- `repetitionDetector` inspects only parsed `toolCalls`; zero were parsed.
- Char cap `maxTokens*4` = 32,768 chars vs. 2,736 reached in 49s — never fired.
- No per-stream duration cap; only the 10-minute per-turn timeout.

## Reference: Hermes Agent

Hermes solves the identical problem with `agent/repetition_guard.py`
(`is_repetition_dominated`, incident #86581): a fragment is repetition-dominated when
a single 60+ char verbatim window (or repeated line) covers ≥50% of the text, checked
with a fail-open minimum. On detection it aborts the turn instead of flooding.

## Fix (generic / provider-agnostic)

- **`isRepetitionDominated`** — port of Hermes's heuristic into `stream.go`
  (`minRepetitionFragmentLen` 400, `repetitionWindow` 60, `minRepetitionCount` 5,
  `repetitionDominanceRatio` 0.5). Keys purely off streamed bytes; independent of
  grammar/tool format.
- **Content repetition guard** — in `processStream`, when `Content` is
  repetition-dominated and zero native tool calls are parsed, abort into the existing
  `[stuck]` recovery (clear content, set `[stuck]` → `handleEmptyStream` →
  `handleNoToolCalls` → progressive-sieve nag). Real tool calls are never discarded.
- **Per-stream duration cap** — `streamMaxDuration` (90s, test-shortenable). A stream
  with no native tool calls and no completion beyond the cap terminates while
  **preserving** content (mirroring char-cap termination) so a genuine slow report is
  evaluated/salvaged rather than dropped.

## Verification

`go build ./...`, `go test ./...`, `go run ./tools/check-complexity/`, and
`go test -race ./internal/core/assistant/` all pass. New tests cover the primitive,
the guard (terminates degenerate loop, preserves distinct content, preserves native
tool calls), and the duration cap.

## References

- Hermes Agent: `agent/repetition_guard.py`
- SPEC-001 §II.8 (updated), `docs/skills/agent-loop.md` (updated)
- Run logs: `~/.config/llm-proxy/runs/workspace-1/deepseek-v4-flash-0731/workspace-health-test/20260820T154509Z_7af85d8a9a44af34/`
