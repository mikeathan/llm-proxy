---
id: audit-2026-08-30-llm-smoke-test
title: "llm-smoke-test Incomplete Run — Root Cause & Generic Fixes"
status: complete
date: 2026-08-30
related_specs: [SPEC-001, SPEC-002]
---

# Audit: llm-smoke-test Finished "Incomplete" on a New Local OpenAI Model

## TL;DR

The automation `llm-smoke-test` run against `Ornith-1.5-35B-Q4_K_M.gguf`
(freshly added via the OpenAI provider, served by llama.cpp on a remote host)
completed after **2 of 10 steps**, with this "final report":

```
Thought: `uname -a` isn't supported on this system. I'll report this as-is. Now running the date and echo commands (Step 3).

Action:
```

Three distinct defects compounded:

1. **Terminal tool destroyed multi-line commands** — a generic bug that broke
   the smoke test's Step 3 for *any* model.
2. **Premature finalization** — the loop recorded a truncated ReAct scaffold
   ("Thought: … Action:" with no tool call) as a successful final answer.
3. **Per-model tool-format mismatch** — the model was forced into XML text
   mode even though its serving endpoint supports native OpenAI tool calling
   (the reason the operator kept having to patch per-model settings).

## Evidence (run `runs/workspace-1/Ornith-1.5-35B-Q4_K_M.gguf/llm-smoke-test/20260830T140612Z_*`)

- `events.jsonl`: 3 LLM calls, 2 tool calls, then `phase: completed` with the
  truncated thought above. The model's stream ended at `Action:` (EOS) — the
  last `tool_stream` deltas are `"Action"` then `":"`, then nothing.
- `recording.jsonl`: request has **no `tools` array** (XML text mode); the
  assistant messages are the "Thought: … Action:" scaffold.
- Backend log:
  - `tool execution failed | name execute_terminal_command error shell execution failed: shell execution failed: exit status 1`
  - `tool execution failed - stopping batch`
  - The tool result was exactly `usage: uname [-amnoprsv]` — reproduced by
    running the *whitespace-collapsed* command locally.
- llama.cpp log: the third call evaluated ~71 tokens and stopped
  (`truncated = 0`) — the model hit EOS mid-scaffold, it was not a
  max-token/context truncation.

## Root cause 1 — `sanitizeCommand` collapsed newlines (generic bug)

`backend/internal/core/tools/terminal.go` `sanitizeCommand` ended with:

```go
return strings.Join(strings.Fields(command), " "), nil
```

`strings.Fields` splits on **all** Unicode whitespace, including `\n`. The
model's (correct) batched Step 3 command

```
uname -a
date -u +%Y-%m-%dT%H:%M:%SZ
echo "terminal-tool-works"
```

became `uname -a date -u +%Y-%m-%dT%H:%M:%SZ echo "terminal-tool-works"` →
`uname` prints `usage:` and exits 1 (verified locally). This is a generic
system bug: **any** model that batches commands with newlines (Constitution
II.2 explicitly encourages batching) hit it. The guardrail validator had the
same collapse, so validation and execution also disagreed (a newline-separated
second command could bypass the per-segment whitelist once execution started
preserving newlines).

**Fix:** newline-preserving whitespace normalization
(`collapseWhitespacePreserveNewlines` — horizontal whitespace collapsed per
line, line structure kept) applied in both `sanitizeCommand` and
`ValidateTerminalCommand`; `splitCommandSegments` now treats `\n` as a command
separator (heredoc terminator consumed so heredoc bodies stay one segment).
Result: multi-line commands execute as written, and the whitelist is enforced
per line.

## Root cause 2 — truncated ReAct scaffold treated as a final report (generic)

`checkTaskCompletion` accepts a content-only turn with ≥20 chars and any tool
result in history. "Thought: … Action:" is 133 chars, has no `<tool_call>`
marker, and two tool results existed → accepted → run "completed".

The completion gate had a second back door: for plain text the XML parser
returns `ParseError{XMLFound: false}` (no tags), and
`handleParseErrorFeedback` *trusts any non-empty content as completion* when
`XMLFound=false` — so even content `checkTaskCompletion` rejected was
re-accepted there.

**Fix:** a turn whose **last non-empty line is a bare `Action:` marker** — the
delimiter the system prompt's own `Thought -> Action` contract places on its
own line before every tool call — is a truncated tool-call attempt, not a
final answer. `endsWithBareActionMarker` (line-based match, so a report ending
"...the action:" is untouched) is enforced at **every completion surface** so
all loop strategies behave identically: `checkTaskCompletion` and the
parse-error trust branch (react / evaluator-optimizer), `finalizeReport`
(plan-execute + the react recovery ladder), and `bestAvailableAnswer`. The
turn falls into the recovery ladder (parse-error feedback / nag) and the run
continues instead of finalizing.

This mirrors the existing `hasToolCallMarker` heuristic (unclosed
`<tool_call`/`<function` markers also reject completion): both validate the
model against **our own prompt contract**, not a foreign dialect.

## Root cause 3 — local model forced into XML text mode (why the operator kept patching)

`settings.yml` model overrides:

```yaml
Qwen3.6-35B-A3B-UD-Q4_K_M.gguf:   # works
    tool_call_format: native
Ornith-1.5-35B-Q4_K_M.gguf:        # broken
    prefill: false
    # no tool_call_format → "" → XML text mode
```

The model that worked had `tool_call_format: native`; the new model did not,
so it defaulted to XML text mode (`UseNativeTools=false`), where the model
must hand-write `<tool_call>{"tool":…,"args":{…}}</tool_call>` in its output.
Every patch the operator applied for this model — parse-error feedback,
`hasToolCallMarker`, nudge re-arming, truncation detection,
`stripThinkBlocks`, the model-compat warning, and per-model
`tool_call_format` settings — compensated for that format mismatch.

A direct probe of the serving endpoint proved the model supports native
OpenAI tool calling perfectly:

```json
{"choices":[{"finish_reason":"tool_calls","message":{"tool_calls":[
  {"function":{"name":"list_directory","arguments":"{\"path\":\".\"}"}}]}}]}
```

**Fix (removes the per-model patching):** `LLMRuntimeManager.EffectiveToolCallFormat`
auto-resolves the format for local OpenAI-compatible models with an unset
`tool_call_format`: a one-time, cached capability probe
(`OpenAICompatibleProvider.ProbeNativeTools` — a minimal chat request with a
tiny `tools` schema; `finish_reason == "tool_calls"` ⇒ native). The result is
persisted onto the stored model config so every consumer (agent builder,
system prompt, history normalization) sees the same format. Explicit
`tool_call_format` ("native"/"xml") always wins (Constitution II.5); probe
failures fall back to XML for that turn and are re-probed. The executor and
conversation service resolve the format through this single path.

## Root cause 4 — local `context_budget` used a wrong chars-per-token ratio

After native mode fixed the malformed-XML failures, the smoke test still ended
partial/confabulated (e.g. claiming `get_network_info` "not performed" when the
events show it ran). A **physical sieve** pruned the context at the final turn
(requests grew to 20K chars, then dropped 17→15 messages), so the model lost
early tool results and rationalized a premature stop as a "context limit".

The sieve threshold is the auto-derived `context_budget`, and the derivation
was miscalibrated:

```
context_budget = (serving_ctx − max_tokens) × 2     // 2 chars/token
```

Real LLM tokenizers average ~3.5–4.75 chars/token — **measured 4.75** on a
realistic mixed reasoning/JSON/code prompt against Ornith — and this project's
own reasoning-budget check (`agent.go`) plus the Hermes agent both use 4.0. So
the old budget (10924 chars ≈ only ~2,300 prompt tokens) was **~2× under the
intended 5,462-token prompt reserve**: the sieve pruned when the window was
barely a third full. This is **generic** (every local model), not Ornith-only.

This was "tried before and blocked": the archived `auto-context-budget.md` plan
proposed 3.5 c/t but with a formula that didn't reserve output (overflow risk),
and `memory-injection-investigation.md` (2026-06-03) hit the exact premature-
sieve symptom, recommended raising the budget, but backed off fearing the 8192
token limit based on a bogus "~0.75 chars/token" estimate — your run proves the
real ratio is ~3.7+ (20K chars fit with no overflow), not 0.75.

**Fix:** `LocalBudgetPolicy` now derives `context_budget = (ctx − maxTokens) ×
localContextCharsPerToken` with `localContextCharsPerToken = 4.0` — deliberately
under-estimating the measured 4.75 so the char budget maps to *fewer* prompt
tokens than the reserve (the sieve fires before the window fills, leaving room
for `max_tokens` of output). For Ornith: `(8192−2730) × 4 = 21848` chars. Cloud
models are unaffected (their budget is tier-capped and huge; the formula only
changed for the local path). The **reactive sieve** remains the overflow
backstop.

## Root cause 5 — sieve measured raw history, not the request (plus a misclassification)

The next run (18:45) failed hard with `output cap exceeded: requested 2730
max_tokens but the model supports at most 8192` — a backwards message that
turned out to be TWO bugs:

1. **The physical sieve measured RAW history, not the request.** The prepared
   request includes system-prompt enrichment (tool manual/reference, memory)
   that raw history omits (~6.5 KB in this run). So the sieve's char count was
   far below the real prompt: with the new 21848 budget it never fired while
   the request grew to 27.7 KB (~5,850 prompt tokens + 2,730 output > 8,192),
   and the server 400'd "context too long". Fix: `executeTurn` now measures
   `preparedOverContextBudget` (the prepared size, memory-injection excluded so
   the one-shot flag stays for the real request) before applying the sieve.
   This also explains the EARLIER partial runs: with the old 10924 budget the
   same mismatch made the sieve fire too early in raw terms (context loss).
2. **The proxy misclassified the context-400 as an output-cap error.**
   `outputCapPatterns` matched "context length/window/size ... N tokens"
   phrasings and built `OutputCapError{Requested: 2730, Available: 8192}` —
   backwards, and it suppressed the correct recovery (the reactive sieve via
   `isContextSizeError`). Fix: removed the context phrasing from the output-cap
   patterns; context overflows now flow to the reactive sieve.
3. **hello.txt `sh` execution was blocked (template vs allowlist).** The task
   template instructs `sh smoke-test-dir/hello.txt`, but `sh`/`bash` were not
   on the terminal allowlist while arbitrary-code interpreters (`node`,
   `python`, `go`, `npx`, ...) already were — an inconsistency, not a security
   boundary (the boundary is the workspace jail + path checks + blocked
   patterns). Hermes (reference agent) doesn't allowlist at all: it runs any
   shell in a sandbox with a dangerous-command approval prompt
   (`tools/approval.py`). Fix: `sh`/`bash` added to the manifest allowlist
   (merged with settings.yml). The template itself was left unchanged (it is a
   test fixture; the system must support what it exercises).

## Root cause 6 — the 20:08 run: sieve still missed native tool args; summary and finalization gaps

The 20:08 run (19:04Z) did Steps 1-7 correctly (including `sh` script execution)
but still overflowed context (server 400: `request (8350 tokens) exceeds the
available context size (8192 tokens)`) — the reactive sieve recovered it — and
then the final REPORT turn was aborted by the reasoning-stuck detector, leaving
only a synthesized summary that claimed "2 tool calls". Three follow-up fixes:

1. **Native tool-call ARGUMENTS were not counted by the sieve.** In native mode
   tool calls ride in `message.tool_calls` (not Content), and write_file/edit
   arguments can be thousands of chars. `preparedOverContextBudget` (and the
   physical sieve) summed Content only, so they under-measured the real request
   by the entire tool-args size and never fired before overflow. Fix: a shared
   `messageChars` helper counts Content + ReasoningContent + tool-call
   arguments, used by both the sieve decision and the sieve itself.
2. **The synthesized summary under-reported tool activity.** It scanned
   `s.history`, which the sieve prunes to head+tail — so it saw "2 of 18 tool
   calls". Fix: it now counts from the usage tracker's `UsedToolsSnapshot()`
   (the per-execution record that survives sieving), falling back to history.
3. **The finalization turn was killed by the reasoning-stuck abort.** The final
   report turn (tools disabled) streamed 36s of pure reasoning (2732 chars >
   the 2730 maxTokens threshold for reasoning_budget-0 models) and was aborted,
   so the report was never written. Fix: the stuck check is skipped on the
   finalization turn (a thinking model legitimately reasons before writing);
   the per-stream duration cap still bounds it.

## Root cause 7 — the 08-31 runs: server down + native-format model forced into XML

Two runs on 08-31:
1. 13:24 — the llama-server was DOWN (`connection refused`); the probe failed
   cleanly and the run failed with a clear upstream error. Infrastructure, not
   code.
2. 13:25 — the server was still LOADING (`503 Loading model`), so the
   capability probe failed and the run fell back to XML text mode. This model
   natively emits the `<function=name><parameter>…</parameter></function>` text
   format, and the parser only attempted it when `useNativeTools` was active —
   so every turn died with `invalid character '<' looking for beginning of
   value`, the run nudged 3×, forced finalization, and ended nearly empty
   (5 LLM calls, 1 tool call).

Fix: `handleContentToolCalls` now tries the native text-format parser as a
fallback in EVERY mode (it only matches the specific `<function=…>`/`<tool=…>`
tags, so no false positives on conversational text). A native-format model in
XML mode now parses correctly; the probe-failure fallback no longer
functionally breaks the run.

**13:31 run (after the fixes) — verification:** Steps 1/3/9 fully correct,
Step 2 written; the finalization turn streamed ~60s of pure reasoning and THEN
wrote the 1,399-char report (the SkipStuckCheck-on-finalization fix works — the
stuck detector would previously have aborted it). Remaining gap: Step 4 used
`printf` (not allowlisted — inconsistent with `echo`), got guardrail-blocked,
and the model gave up; the run finalized after 4 tool calls. Fix: `printf`
added to the allowlist.

**13:36 run — sieve + finalization fixes proven; new model dialect.**
The physical sieve fired correctly for the first time (`chars 22064`), the
finalization produced a 2,682-char report, and the model did real work (11
tool calls: list, terminal ×2, write_file ×3, read_file, fetch_url ✓,
get_network_info ✓ — Step 8's httpbin fetch SUCCEEDED). But the model's report
claimed "0/5 categories passed, all tool calls failed" — FALSE; it confabulated
because it kept hitting parse errors on a THIRD dialect it emits:
`<invoke name="…"><parameter name="…">…</parameter></invoke>` (the parser only knew
`<function=…>`/`<tool=…>`), plus 3× `read_file` calls missing the required
`path` arg (correctly rejected by validation). Fix: `invoke` added to the
native-format tag alternation; a regression test covers the exact dialect.

## Changes

- `backend/internal/core/orchestrator/budget_policy.go` — `LocalBudgetPolicy` now derives
  `context_budget = (ctx − maxTokens) × localContextCharsPerToken` (4.0 chars/token, the
  real tokenizer ratio; was ×2). Golden snapshot regenerated; pinned tests updated.
- `backend/internal/core/assistant/sieve.go` + `session.go` — the physical sieve now
  measures the PREPARED request size (`preparedOverContextBudget`, memory injection
  excluded) instead of raw history, so it fires on the real prompt before it
  overflows the serving window.
- `backend/internal/core/llm/providers/output_cap_error.go` — removed the
  "context length/window/size" phrasing from output-cap detection; context 400s
  now reach the reactive sieve instead of being mislabeled "output cap exceeded".
- `backend/internal/core/tools/manifests/terminal.json` — allowlisted `sh`/`bash`
  (script execution; consistent with node/python already being allowlisted).
- `backend/internal/core/tools/terminal.go` — newline-preserving sanitize +
  validation, `\n` as segment delimiter, heredoc terminator consumed.
- **Follow-up terminal review (same session)** — additional findings and fixes:
  - **Whitelist bypass via heredoc syntax (security)**: `splitCommandSegments`
    only recognized `<<EOF`; `<<-EOF`, `<<'EOF'`, `<<"EOF"`, `<<\EOF`
    never terminated the heredoc, so a disallowed command after it
    (e.g. `cat <<-EOF … EOF\nrm -rf /`) was swallowed into one segment and
    the whitelist checked only `cat`. `<<<` here-strings wrongly opened
    heredoc mode too. The scanner now handles every marker form and
    here-strings, and reports **unbalanced** commands (unterminated quote or
    heredoc); `ValidateTerminalCommand` rejects them fail-closed and
    `executeShell` re-checks before touching the persistent shell.
  - **Sentinel stall (robustness)**: a trailing shell comment
    (`cmd  # note`) commented out the completion sentinel in
    `persistentShell.Execute`, stalling the reader until the tool timeout and
    wedging the shared session. The sentinel is now newline-separated
    (`cmd\necho SENTINEL$?`), and the `(cd … && …)` cwd wrapper gets a
    trailing newline so a comment/heredoc cannot swallow its closing paren.
  - **Heredoc/string content mangling**: `sanitizeCommand` collapsed ALL
    whitespace (including heredoc bodies and string literals). It now applies
    path forgiveness only — validation uses its own collapsed copy, and both
    preserve line structure, so validation and execution still agree.
  - **Performance**: the scanner iterated `[]rune` and allocated a string from
    the tail on every character (`string(chars[i:])` — O(n²)); it now scans
    `[]byte` with `bytes.HasPrefix` and precomputed delimiters (O(n)).
- **Probe budget for thinking models (2026-08-30, second smoke-test run)**:
  the re-run still resolved to XML mode ("model does not support native tool
  calling") and the model again hit the malformed-XML wall at Step 6, then
  rationalized a premature stop as a "context limit" (no sieve fired; request
  context grew 16.3K→19.6K chars unpruned — the claim was false). Root cause:
  the probe used `max_tokens: 32`, and Ornith is a preserve-thinking model —
  it spends tokens on `reasoning_content` first, so 32 tokens ended with
  `finish_reason: "length"` and zero tool calls (reproduced live; `max_tokens:
  128` returns `finish_reason: "tool_calls"` in 71 tokens). Probe now uses a
  generation-time-based ladder so ANY model class resolves correctly: budgets
  512 → 2048 (verbose thinkers), deadlines 30s → 60s (slow hardware gets the
  same wall-clock chance as fast), one escalation on `finish_reason: "length"`
  OR deadline-exceeded, and immediate failure on other transport errors (a
  dead endpoint never stalls the agent through the ladder).
- `backend/internal/core/tools/terminal_test.go` — multi-line passthrough,
  per-line whitelist, heredoc segment tests.
- `backend/internal/core/assistant/session.go` — `endsWithBareActionMarker`
  guards in `checkTaskCompletion` and `handleParseErrorFeedback`.
- `backend/internal/core/assistant/agent_test.go` — truncated-scaffold
  completion cases.
- `backend/internal/core/assistant/recovery_ladder_test.go` — regression test
  for tool-error → truncated scaffold → continued run.
- `backend/internal/core/llm/providers/provider_openai_compatible.go` —
  `ProbeNativeTools` capability probe.
- `backend/internal/core/llm/manager.go` — `EffectiveToolCallFormat` (probe,
  cache on stored config).
- Wiring: `bootstrap.go` (`AppServices.EffectiveToolCallFormat`),
  `automation/executor.go`, `assistant/conversation_service.go` (+ mocks).
- Docs: `CONSTITUTION.md` II.5, `docs/SPECS/agent-loop.md`, `docs/INDEX.md`.

## Verification

- `go build ./...` clean.
- `go test ./...` — all packages pass (one pre-existing timing-flaky
  heartbeat test fails intermittently under full-suite load, passes in
  isolation; unrelated to this change).
- `go run ./tools/check-complexity/` — clean (≤12).
- New tests: terminal multi-line/whitelist, completion Action-scaffold,
  recovery-ladder regression, `ProbeNativeTools` (3 cases), manager
  `EffectiveToolCallFormat` (5 cases).

## Resolution — verified on run 20260831T134424Z (Ornith, 08-31)

The 13:44:24Z run is the first fully clean end-to-end pass and confirms every
fix above:

- 20 LLM calls, 15 tool calls, 2m7s, **0 parse errors, 0 guardrail
  rejections, 0 stuck-cuts** — the `<invoke>` dialect fix (native format
  parser) held for the whole run.
- All 8 exercise steps executed: list_directory ×6, execute_terminal_command
  ×15, write_file ×6, read_file ×6, edit_file_block ×6, fetch_url ×3,
  get_network_info ×3 — every tool result `error <nil>`.
- The finalization turn ran with `SkipStuckCheck` and produced a structured
  Markdown report (`final-report.md`).

Remaining observation (model behavior, not a system defect): the model hit
EOS mid-report at Step 3 — recording ends with a clean `done` at 433 chunks
(`content_len 964`, `reasoning_len 790`), i.e. the 35B Q4 model simply stopped
generating while writing the report table. No system cut or abort fired. The
smoke test's *work* (all tool steps) is complete and error-free.

## Follow-up — Hermes-style length-continuation (08-31, second pass)

The 13:52:50Z run produced a complete 64-line report (all 4 sections +
Summary, 0 errors), confirming the truncated 13:44 report was model variance,
not a system gap. To harden the same failure class generically (any model /
provider), we adopted the one Hermes mechanism we lacked — output-cap
truncation continuation:

- The stream layer now surfaces the upstream `finish_reason` on the turn
  message (`models.Choice.FinishReason` ← wire, → `models.Message.FinishReason`,
  `json:"-"` so it is never replayed into history — strict providers reject
  unknown message keys; mirrors Hermes's stripping in
  `chat_completion_helpers.py`).
- A content-only turn with `finish_reason == "length"` is a **truncated final
  answer**, not a completion: `handleTextTurn` and `finalizeReport` keep the
  partial, inject `LengthContinuationPrompt` (port of Hermes's
  `_LENGTH_CONTINUATION_OUTPUT_LIMIT` — "continue exactly where you left off,
  do not restart"), and run the next turn. Fragments are stitched at
  completion via `joinTruncatedParts` (port of `_join_truncated_parts`),
  bounded by `lengthContinuationMax = 2` per run.
- Clean `finish_reason="stop"` is untouched — the guard fires only on genuine
  output-cap truncation, never on a model choosing to stop (so the 13:44 EOS
  case is deliberately *not* continued; no heuristic can distinguish "done"
  from "stopped early" on a clean EOS).
- New tests: `resolveStreamChunk` finish_reason propagation, handleTextTurn
  continuation / bound / stitching / clean-stop-unchanged,
  `joinTruncatedParts` glue, finalizeReport continuation / bound, control-
  message registration.

## Follow-up — cold-cache native-tools race (08-31, 14:26 run)

The 14:26:05Z run regressed hard: only 5 of 15 tool calls executed, Steps 4–10
unreached, the report documenting "repeated tool-call JSON format errors".
Root cause was **not** the length-continuation work — it was a probe/agent
**ordering race** exposed by a backend restart at 14:25:58:

- `EffectiveToolCallFormat` resolves a local model's format by probing the
  serving endpoint (an HTTP round-trip, ~4s here). The probe result is cached
  on the manager's stored config (`m.models[name].ToolCallFormat = "native"`).
- The automation executor called `NewAgent` **before** `EffectiveToolCallFormat`,
  so on a cold cache (first run after restart) `buildAgentOptions →
  ApplyModelConfig` read `ToolCallFormat == ""` and locked the agent into XML
  text mode. The probe then fired *inside* the first `executeTurn` — 4s too
  late — leaving the whole run in XML mode: no `tools` array in requests, XML
  `<tool_call>` system prompt. Ornith then emitted truncated/malformed XML
  JSON (`unexpected end of JSON input`, `invalid character '\n' in string
  literal`) → parse-error ladder → forced finalization.
- The chat path was already correct (`resolveModelConfig` probes before the
  agent builder); only the automation executor had the race.

Fix (`backend/internal/core/automation/executor.go`): resolve
`EffectiveToolCallFormat` **before** `buildAgentOptions`/`NewAgent`, so the
persisted "native" result is visible to `ApplyModelConfig` and the agent is
built with native tool calling from turn one. The probe runs once per process
(later runs hit the cache). Regression tests:
`TestBuildAgentOptions_NativeToolFormatPropagates` (post-probe config → agent
`UseNativeTools=true`) and `TestBuildAgentOptions_ColdCacheDefaultsXML`
(unprobed config stays XML — the state the reorder corrects by probing first).


## Follow-up — assistant guardrail approval never reached the UI (08-31, 15:00 run)

The 15:00 assistant run (`conv_20260831155946`, Ornith, "list all files and
report") looked stuck for ~5 minutes. It was **not** stuck: the model's
command `find ./node_modules -type f | wc -l` was rejected by the terminal
allowlist (`wc` missing), the approval wait burned the full
`guardrail_approval_timeout_seconds: 300`, timed out as designed
(`guardrail approval wait ended without a decision` at 15:05:18), and the
model then adapted and delivered a complete report at 15:05:40. The user saw
the `guardrail_violation` text but **no approval banner**.

Two defects:

1. **Backend channel bug (root cause of the missing banner):**
   `NewGuardrailDecisionCallback` published `guardrail_blocked` /
   `guardrail_invalidated` without a `Channel` field. The event bus
   (`channelOf` in `automation/broadcast.go`) defaults an empty channel to
   `ChannelAutomation`, so the assistant approval event was broadcast on the
   automation channel — the assistant SSE (subscribed to `channel=assistant`)
   never received it, no banner rendered, and the wait expired. Fixed by
   passing the producer channel into `NewGuardrailDecisionCallback`
   (`ChannelAssistant` from the conversation service, `ChannelAutomation` from
   the executor) and stamping it on both events. Regression test:
   `TestGuardrailDecisionCallback_ChannelPropagation`.
2. **Frontend no-op handlers:** `AssistantChat.vue` rendered the
   `GuardrailBanner` with `@allow/@deny` stubbed to `() => {}`. The banner
   self-posts the decision, so buttons would have worked even with the stubs,
   but the parent never cleared `pendingDecision` and offered no retry.
   Fixed by exposing `submitDecision` on `useAssistant` (POST to
   `/admin/api/conversation/guardrail-decision`, mirroring the automation
   console) and wiring the banner to it.
3. **Allowlist gap:** `wc` was missing from `terminal.json` despite `ls`,
   `find`, `sort`, `grep`, `head`, `tail`, `cat`, `du` all being allowed —
   a read-only counting utility. Added, with tests asserting the manifest
   allows the full read-only inspection set and that a `find | wc -l` chain
   validates (and still fails when `wc` is not allowlisted).

## Lessons

1. Multi-line shell commands are first-class input — normalization must
   preserve line structure, and the guardrail must validate the same command
   that executes.
2. "Content-only turn" ≠ "final answer": completion gates must also reject
   *truncated tool-call intent* (dangling Action marker / unclosed tags), and
   every completion path (natural-completion gate *and* parse-error trust
   branch) must agree.
3. Tool-call format should be a capability, not a per-model manual setting —
   probe the endpoint, default to native when supported, keep XML as the
   fallback.
4. The terminal guardrail is a security boundary for untrusted model output:
   every shell syntax form the whitelist relies on must be scanned correctly
   (heredoc markers, here-strings, quotes), and malformed syntax must fail
   closed instead of being silently absorbed — otherwise a disallowed command
   rides in on the tail of a "valid" one.
5. Commands are data: never whitespace-mangle what reaches the shell (heredoc
   bodies and string literals are content), and make completion sentinels
   uncorruptible by comments (newline-separated).
