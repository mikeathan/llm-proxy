---
status: reference
date: 2026-06-03
related_specs: [SPEC-004]
---
> **Resolved (2026-08-30):** the premature-sieve root cause described here was
> the local `context_budget` using a wrong 2 chars/token (≈10924 chars for an 8192 ctx),
> which pruned history when the real window was barely a third full. The derivation now
> uses the real ratio (4 chars/token → ≈21848 chars); see
> `docs/audits/2026-08-30-llm-smoke-test-incomplete-run.md` §budget.
>
> **Later (2026-10-01):** the per-run head-system `<memory>` block, ledger and per-automation `memory_mode` in
> `docs/PLANS/memory/small-context-memory.md` supersede the injection design below. This file is the evidence for
> why the earlier approaches failed.

# Memory Injection Investigation Report

**Date:** 2026-06-03 · **Model:** Gemma 4 4B Instruct (`gemma-4-4b-it-Q4_K_M.gguf`) · **Server:** llama.cpp on remote host
`vertex` (Ubuntu, Vulkan, AMD Radeon 780M, ~21 tok/s, 8192 ctx, `--parallel 1`, port :8082) · **Proxy:** local Mac
(`go run main.go`, :4001)

## 1. Problem

The smoke-test automation (`llm-smoke-test.md`, 10 steps) contains **discovery commands** for facts already in memory
(`uname -a`, `npx tsc --version`, `npm install --save-dev typescript`). Memory held `tool_versions` (TypeScript 6.0.3)
and `system_os_info` (Darwin Mac 25.5.0), yet the model re-ran the commands, wasting 30–40 s per run.

| Aspect | Impact |
|---|---|
| Startup delay with the rewriter | 77–89 s (removed; now 8 s) |
| Extra discovery steps | 3 × 10–15 s ≈ 30–45 s |
| Total runtime | ~7 min (too slow for a smoke test); ~5.5–6 min estimated if memory skipped the steps |

Constraint: the model cannot change; work within a 4B Q4_K_M model's instruction following.

## 2. How the pieces fit (at the time)

`LLMTaskExecutor.Execute()` (`executor.go:93`) gets the LLM client, builds the prompt and calls `Agent.Execute()` (tool loop).
Memory is a SQLite FTS5 store (`memory.Store`, `memories_fts`, BM25).

| Component | File | Role |
|---|---|---|
| `buildPrompt()` | `executor.go:341` | Prepends `MemoryCheckGate` to task content, wraps with `AutomationTaskPrompt` |
| `buildAssistantPrefill()` | `executor.go:347` | Assistant-role message citing memories (added 2026-06-03) |
| `injectActiveMemory()` | `stream.go:88` | Appends `<relevant_memories>` to the system prompt every turn: finds the last user message, uses the cached automation task prompt as the query, `memStore.Search(ctx, ws, query, 5)` → top 5 |
| dispatcher | `automation/dispatcher.go` | 5-minute timeout for the whole automation |

Prompt shape after `Execute()`: `[System: rules + <relevant_memories> (tool_versions, system_os_info, dev_environment_setup)]`,
`[User: "TASK: autonomous agent in workspace…" + Memory Check Gate + the 10 steps + "Use your tools to complete every step."]`,
`[Assistant prefill: "I'll check my relevant memories before each step. <3 facts> If a memory already contains an answer a step is
trying to discover, I'll skip that step."]`. Attention: system is oldest, user mid, prefill most recent and assistant-role —
in theory the strongest, in practice the user's explicit "run X" still wins.

**Table `memories`:** `id, workspace_id, memory_type (long_term|daily|session|user_profile), title, content, source, created_at,
updated_at`; FTS5 over `title, content` (`content=memories`, `tokenize='unicode61'`).

**Retrieval:** the query is the *entire task content*. `sanitiseFTSQuery` (`store.go`) strips non-alphanumerics, splits into terms,
double-quotes each, joins with ` OR `, returns top 5 by BM25 (TypeScript-heavy tasks rank `tool_versions` first). On 2026-06-03 the
quoting was added: `typescript_version OR tool_versions` previously became `typescript OR version OR OR OR tool…`, and two
consecutive FTS5 `OR`s crashed with "SQL logic error: fts5: syntax error near 'OR'".

**Dedup (write-time):** `findOverlappingEntry()` (`memory_tools.go`) uses Jaccard similarity on normalised topic words; J ≥ 0.70
→ same content returns "already saved", different content updates in place (stops `tool_versions` vs `tools_version`).

## 3. What we tried

| # | Attempt | Result | Why |
|---|---|---|---|
| 1 | System-prompt `<relevant_memories>` only ("Review the block above before each step") | Ignored; every discovery command ran | System prompt is distant from the task; the explicit "run X" overrides general guidance |
| 2 | `MemoryCheckGate` nag prepended to the task | Marginal. Model sometimes searched (`13:02:32Z memory_search` → found) then still ran `npx tsc --version` at 13:02:47Z | General guidance loses to specific imperatives for small models (Instruction Hierarchy, Wallace et al. 2024) |
| 3 | **LLM rewriter** (`rewriteTaskWithMemories`): a separate Chat call rewrites the task with memory-check gates so the agent never sees the original commands | ✅ Skipped `npm install` and `npx tsc --version`. **Removed**: 77–89 s | Unique system prompt (`TaskRewriterSystemPrompt`) never hits llama.cpp's prompt cache; up to 2048 tokens at ~21 tok/s ≈ 97 s cold on every run; frozen UI. Timeline: 11:45:18Z start → 11:46:47Z rewritten (89 s) vs without: 12:59:25Z → 12:59:33Z first stream (8 s) |
| 4 | Fallback `RewriteFailedNag`, later renamed `MemoryCheckGate` | Same as 2 | — |
| 5 | Warn-only reasoning budget (no `return nil`; `budgetWarned` flag) | ✅ Ended the XML/non-streaming fallback chain; 200+ identical warnings per stream → once (12:49:21Z → 13:05:30Z). llama.cpp already enforces `reasoning_budget`, the proxy was double-enforcing | **Reversed 2026-06-26:** runaway joke-loop on local Qwen3.5-9B (7000+ chars, no EOS) proved the server does not always enforce; `processStream` honours `ShouldTerminate` (returns nil) with a char-cap fallback `maxTokens * 4` (`exceedsContentCharCap`); budgets unchanged. See `docs/PLANS/cross-cutting/assistant-cancel-endpoint.md` § "Fix: honor ShouldTerminate + add char cap safety net" and `docs/architecture.md` invariant #10 |
| 6 | Detach rewriter context (`context.WithoutCancel`) | Moot once the rewriter was removed | — |
| 7 | `ApplyModelOverrides` after sync events | ✅ Fixed: registry/secrets handlers and `Sync()` callers were wiping `settings.yml` overrides (`tool_call_format`, `reasoning_budget`) | — |
| 8 | **Assistant prefill** (`buildAssistantPrefill`) | Partial: first turn `content_len 635` (acknowledged), but later turns still ran `uname -a` (14:42:42Z), `npm install` (14:44:10Z), `npx tsc --version` (14:45:05Z) | It states an *intent*, not a binding rule; the next "Step N: run X" imperative overrides it for a 4B model |
| 9 | Pattern-based skip directives (`buildSkipDirectives`: hardcoded command strings) | Rejected at review | Memory can hold any fact (DB strings, regions, endpoints, paths); the pattern list is unbounded and fragile |
| 10 | `notify_user` description + prompt changes (June 7) | None worked | Root cause was not prompting — see §7 |

## 4. Industry research

| Project | Memory location | Injection | Skip instruction | Use case |
|---|---|---|---|---|
| Hermes Agent (NousResearch) | `~/.hermes/MEMORY.md` | Static block in the system prompt's "volatile" tier every turn; no auto-dedup (the agent manages the file) | None; `MEMORY_GUIDANCE`: "You have persistent memory across sessions. Save durable facts using the memory tool: user preferences, environment details, tool quirks, and stable conventions. Memory is injected into every turn, so keep it compact and focused on facts that will still matter later." | Interactive chat |
| OpenClaw | `MEMORY.md` + `memory/YYYY-MM-DD.md` + `DREAMS.md` | Bootstrap context + auto-recall snippets | Maintainers: "Make retrieval mandatory… search memory before acting"; "durable rules in files, not chat" | Interactive chat |
| Cursor / Claude Code | The file system (`.cursorrules`, `AGENTS.md`, `CLAUDE.md`, source) | File contents | None | Coding assistant |
| Ours | SQLite FTS5 | System prompt + assistant prefill | MemoryCheckGate + prefill | Automated task execution |

Hermes/OpenClaw avoid the problem: a human phrases each request, so there is no static script with explicit "run X" commands.
**No framework has solved making a 4B model prefer memory over explicit task instructions in unattended runs.**

## 5. Evidence

Recordings (`backend/testdata/recordings/gemma-4-4b-it-Q4_K_M.gguf/smoke-test/`): with rewriter (89 s, the only run where skips
worked) `20260603T114647Z_7287d5216e53f810.jsonl`; without rewriter (8 s start, prefill added)
`20260603T125925Z_b413467387fa98ae.jsonl`; latest with prefill `20260603T144109Z_626aa9bc04bae5d4.jsonl`.

Latest run (with prefill): 14:41:09Z prefill injected (3 entries) → 14:41:10Z stream, `content_len 635`, `list_directory` →
14:41:44Z `memory_update` `workspace_initial_state` (id 16; saves session state, which the prompt says not to) → write/read file →
**`uname -a` (14:42:42Z) still ran**, `date -u`, mkdir/write/chmod/sh, `npm init -y`, **`npm install --save-dev typescript` (14:44:10Z) still
ran**, write `index.ts`, `npx tsc`, **`npx tsc --version` (14:45:05Z) still ran**, node, edit, `fetch_url`, recompile, `get_network_info`
→ 14:47:55Z single budget warning (`reasoning_used 915 | budget 910`; `budgetWarned` works) → 14:48:13Z final answer.

llama.cpp: prompt processing ~500–650 tok/s cold → ~200–350 warm; generation ~20–22 tok/s steady; reasoning budget 910 tokens/turn;
first-prompt eval 6–7 s cold → 0.3–0.6 s warm. GPU-bound; each turn ~10–20 s.

## 6. Known and unknown (as of 2026-06-03)

**Known:** the rewriter is the only approach that guarantees skipping (it removes the imperative); the prefill is read but not
followed; 4B models follow specific "run X" over general "skip if known" (models up to ~7–8B struggle with conflicting
instructions at different specificity); an 80 s frozen UI is worse than 30–40 s of extra steps; FTS5 top-5 is consistently relevant;
dedup works ("already saved"); the budget-warning spam is fixed.

**Unknown (never tested):** whether prefill works on 8B+ (only gemma-4-4b available; Qwen 3.5 4B and DeepSeek-Coder-V2-Lite 16B
untried); whether prefill position matters (`[System, Assistant, User]` vs `[System, User, Assistant]`); whether a warning can be
injected just before a discovery tool call; whether putting the task in the system prompt helps; whether it is model-specific; whether
910 reasoning tokens is too low (the final-report turn consistently exceeds it).

## 7. Options considered (all on reliability / startup / maintenance)

| # | Option | Reliability | Startup | Notes |
|---|---|---|---|---|
| A | **Hash-cached rewriter**: `cacheKey := sha256(taskContent + allMemoryEntries)`; hit → use cached rewrite, miss → `rewriteWithLLM` then `Set`; in-memory (simplest) or disk-backed (~50 lines, survives restarts) | High (proven) | 80–90 s first run and on memory change, instant on repeat | Same task + memories ~90% of the time |
| B | Binding-rule prefill (a "MEMORY RULES" block: "Step 5: npx tsc --version → TypeScript 6.0.3 already known; DO NOT execute, mark complete", built from FTS5 results rather than hardcoded patterns, i.e. Attempt 9 generalised) | Unknown | 0 | Model still sees the original "run X" → same conflict |
| C | Position swap `[System, Assistant{prefill+memories}, User{task}]` | Unknown | 0 | Task becomes the last message (highest attention) — could be worse |
| D | Per-turn nag after a discovery tool call matches a memory (e.g. "I ran `uname -a` though memory has this; I'll check memory first for the remaining steps") | Unknown | ~50 tokens | Reactive; may teach the model that the reminder is a safety net |
| E | Accept the overhead (~40 s; ~7 min total, still below the rewriter's 89 s startup) | N/A | 0 | — |
| F | Route discovery-heavy automations to a model with better instruction following (7B+) | Model-dependent | 0 | — |

## 8. Implementation findings (2026-06-03)

| # | Layer | Status | Finding |
|---|---|---|---|
| 0 | FTS5 stop-word filter (`store.go`, 15 lines) | ✅ | Improves BM25 recall for task-relevant entries |
| 1 | Per-step proximity injection (`executor.go`) | ⚠️ Fixed: cap 5 + word overlap | First attempt: **46 annotations** (every task line matched some memory on words like "directory", "file", "txt") ≈ 4600 chars → sieve fired on turn 1 → steps repeated (`list_directory` ×3, `uname` ×3), worse than before. Fix: max 5 annotations + require a shared non-stop-word between the task line and the entry ("fetch data from httpbin" won't match "all checks passed"; "install TypeScript and run tsc --version" matches "tool_versions: TypeScript version 6.0.3") |
| 2 | Prior-run seeding (`executor.go`) | ⚠️ Works from run 2 | First run in a fresh session has no prior run. Now seeds from any run with events (cancelled/errored runs still have valid tool events); `payload["result"]` is `any` → `fmt.Sprintf("%v", …)` |

**Sieve fires from model verbosity, not annotations.** After the cap the sieve still fired every turn: the model writes 700–2000
chars of reasoning before each tool call (recapping all finished steps and restating the next one), adding ~1000–2500 chars per
turn; after 5–6 turns history exceeds the ~10924-char budget. Example: `15:47:17Z annotated | annotations 5` → `15:48:30Z
reasoning_len 3127, tool_calls 1 → memory_search` → `WARN critical context pressure - activating physical sieve | chars 11659`.
Model behaviour, not code. (Budget 10924 came from `--ctx-size 8192` at the wrong ratio — see the Resolved banner.)

Options then: raise `context_budget` to 16000 (≈8 turns before pruning; risk near the 8192-token limit); a verbosity-aware sieve that
keeps tool results over reasoning recap (complex); accept (~7 min, steps may re-run); hash-cached rewriter (A); prior-run seeding only
(first run still slow, doesn't help the sieve). Recommended: short-term raise the budget (config only); medium-term the cached
rewriter plus seeding if full reliability is needed; long-term an 8B+ model (fewer reasoning tokens, better hierarchy handling,
2–3× faster).

## 9. `notify_user` mis-selection and its root cause (June 7)

After memory injection was disabled for automations, the model called `notify_user` instead of producing a final answer. Changing the
`notify_user` description ("Do NOT use this to submit final results"), removing memory nudges from `AutomationTaskPrompt` /
`GetSystemPrompt()`, and adding a `DefaultRules` negative rule all failed.

**Root cause:** a trailing comma in `manifests/system.json` made Go's strict `json.Unmarshal` reject the manifest in
`LoadManifestAsTool`; `addTool` logged a warning and dropped the completion tool (and `system_error`). With `tool_choice: required`
the model fell back to the closest tool, `notify_user`, which the guardrail blocked.

> **Superseded (2026-07-22):** the dropped tool was the removed synthetic `submit_final_answer`. Completion is now natural
> (content-only final answer, no tool call). See commit `f89b2cf` and `docs/PLANS/ARCHIVE/cross-cutting/universal-agent-completion.md`.

**Durable lessons:** a trailing comma in any `manifests/*.json` silently drops the tool with only a warning, so validate manifests
after edits; negative instructions ("don't do X") are weaker than positive ones ("do Y") for small models.

## 10. Why injection worked interactively but not in automations

| Factor | Interactive | Automation |
|---|---|---|
| Search query | Short user message (1–2 sentences) | Full task prompt (hundreds of words) |
| Specificity / relevance | High, matches the question | Low, broad FTS5 matches |
| Session length | 1–5 turns | 18–35 turns |
| Attention budget | Ample | Exhausted by step ~10 |
| Model decision | "Answer the question" | "Pick a tool" (must choose one) |

To re-enable it for automations: (1) **step-aware queries** (keywords of the current step, e.g. "npm install typescript", not the whole
task); (2) **early injection** near the start of the prompt (after the system message), not the end where it displaces finalisation
instructions; (3) **relevance filtering** — inject only entries above a threshold (drop always-irrelevant ones like `system_os_info`).

Files touched by the June 7 changes: `internal/core/assistant/stream.go` (`injectActiveMemory`, `prepareMessagesForTurn`),
`session.go` (`maybeFlushMemoryBeforeTurn` automation check), `prompts/templates.go` (`AutomationTaskPrompt` simplified,
`MemoryProactiveNudge` removed), `registry.go` (`MemoryProactiveNudge` removed from `GetSystemPrompt()`),
`internal/core/automation/executor.go` (`MemoryStore` removed from `AgentOptions`), `internal/core/tools/manifests/system.json`
(trailing comma fixed).

**Key takeaway:** memory injection is not inherently broken: it works for interactive sessions (specific queries, short sessions). It
fails for automations where the query is generic, the session long and attention exhausted. Future work should be step-aware
injection, not removal. The `notify_user` bug was one trailing comma; every prompt-level fix addressed the symptom.
