---
status: reference
last_reviewed: 2026-08-18
---

# Known Performance Findings — LLM Streaming & Slow-Assistant Runs

Consolidated, discoverable record of latency/performance findings so future
sessions skip re-deriving them. Load when a run "feels slow", TTFT/streaming
questions come up, or the assistant loop appears stalled.

## 1. Slow assistant runs are provider TTFT, not our logic (2026-08-18)

Observed: `deepseek-v4-flash-0731` (NVIDIA free tier) "list files and report"
run took ~2m47s across 4 sequential LLM calls. Inter-step gaps looked like
17–40s of unexplained delay. **These are provider time-to-first-token, not
local work.**

**Mechanism (how to confirm without guessing):**
- `Client.Stream` (`backend/internal/core/proxy/client.go`) calls `doRequest`
  **synchronously**; `httpClient.Do(req)` blocks until the upstream sends
  response headers (for SSE, that == first token).
- Only after headers arrive does `Stream` return the channel, and only then
  does the `"stream request sent"` log fire (`assistant/stream.go`).
- So: gap from `tool_result` → next `"stream request sent"` log == provider
  TTFT, **not** local processing.

**Local loop is clean (verified):** `react_strategy.go` is a tight loop with no
sleep/backoff between steps; `handleToolTurn` → `processToolCalls` executes
tools in the same second (logs show `executing tool` → `completed` instantly);
guardrails / ledger / `ListTools` are all in-memory. `ResponseHeaderTimeout =
10 min`, `StreamChunkTimeout = 5 min` — no client timeout is cutting streams.
`prefill:false` (no extra local round-trip). Context was tiny (~8.5k chars), so
not context-driven.

**NVIDIA free tier** `deepseek-v4-flash-0731` is a reasoning model
(`reasoning_enabled`, emits `reasoning_content`) with slow TTFT (17–40s/call)
even on tiny prompts; ×4 sequential calls compounds it. Direct-latency probe
against the provider will reproduce the same first-byte delay.

Also documented (prior session): `assistant-liveness-heartbeat-package-split.md`
records the 26s-inside-`doRequest` wait and NVIDIA 529 overloads.

## 2. SSE reader — 1-byte-per-syscall (FIXED 2026-08-18)

`readSSELine` previously read one byte per `Read` call (one syscall/byte).
Originally flagged as `backend-audit-report.md` §1.4 (Medium, bottleneck).
**Fixed**: `Stream` wraps `resp.Body` in `bufio.NewReader`; `readSSELine` uses
`ReadBytes('\n')`.

**Gotcha (do not regress):** `bufio.Reader.ReadBytes('\n')` returns the line
**including** the trailing newline (and any `\r`), unlike the old manual
reader. That would break the `line == "data: [DONE]"` equality check and leave
a trailing `\n`/`\r` in the JSON. The reader strips it with
`bytes.TrimRight(line, "\r\n")` to preserve exact pre-refactor line semantics.
`TrimRight` treats `"\r\n"` as a cutset (agnostic to `\n`/`\r\n`/`\r`), and
never strips mid-line. Guarded by `TestClientStream_LargeAndCRLF`
(100k-char chunk + CRLF line).

**No-terminator-at-EOF:** SSE requires `\n` terminators; streams close on
`data: [DONE]\n`. EOF mid-line only happens on truncation/crash, where the
partial line is unparseable JSON — dropping it is correct (matches old
behavior). Not worth a loop restructure to surface it.

## 3. Not changed / explicitly out of scope

- Step count & reasoning budget are **not** the bottleneck and were left alone
  (user decision). Do not "optimize" these to fix slowness.
- Redundant in-memory `ListModels` calls before assistant requests are
  negligible (no network); not worth touching.
