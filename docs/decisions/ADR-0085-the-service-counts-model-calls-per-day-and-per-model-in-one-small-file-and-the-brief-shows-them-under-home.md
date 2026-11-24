---
id: ADR-0085
title: The service counts model calls per day and per model in one small file, and the brief shows them under Home
status: proposed
date: 2026-11-24
subsystems: [platform, autonomy]
supersedes:
superseded_by:
---

# ADR-0085: The service counts model calls per day and per model in one small file, and the brief shows them under Home

## Context

The owner asked for a panel under Home in the brief that says how many calls the local models had today and, opened, graphs per local model and per cloud model. The service recorded token counts per response (`TokenUsage`) and a per-day Newsroom call budget, but kept no per-day, per-model tally, so nothing could be shown. Every model client is made in one place (`newClientFactory`, the only caller of `NewLLMClient`), and the code asserts optional methods (`CloseRun`, `SetDirForRun`) on the client the recorder returns.

## Decision

1. **A counter wraps the client where it is made** (`core/proxy/usagecount`), under the recorder so the recorder's methods stay on the outermost client. It counts one call when the call ends (a stream ends when its last chunk is read or the caller stops), and reads only what the response carries: the token counts the provider reported and, for llama.cpp, the speed it reported (`timings.predicted_per_second`). It never reads, keeps or logs a prompt or a reply, and passes every chunk and error on untouched. Whether a model is local is asked of the client it wraps, once.
2. **A call an outside program makes to `/v1` is counted in the handler** (`EnsureModelProxyHandler`), because it is a plain reverse proxy to the model server or the provider and never uses that client. It records the model, whether it is local, and whether the answer was an error (status 400 or more); the answer is streamed through untouched (the status-capturing writer passes flushes on), so its tokens are **not** read. A call refused before it reached a model is not counted.
3. **One small file, `model-stats.json`, in the data folder** (`platform/modelstats`): per day and per model, calls, failed calls, tokens in and out, and a speed sum and count. At most 64 models a day (the rest are "other models"), 14 days kept, model names cleaned to one short line. Written at most once a minute from inside a call (no goroutine) and when the service stops; replaced in one step (temporary file, then rename); mode 0600. A file that cannot be read starts the counts empty and is logged; the service never stops for it. The data page lists the file.
4. **One admin read, `GET /admin/api/model-stats?days=N`** (default 7, at most the days kept): the last days oldest first (a day with no call is zeros, local and cloud apart) and today's models busiest first. Signed-in only like every admin route; numbers and model names the owner configured, nothing else; nothing leaves the machine.
5. **The panel** (Model use) sits under Home in the edition's side column and the dashboard Brief card: folded to a line ("252 calls today" and a 7-day trend), opening into the week as stacked local and cloud columns and today's models with calls, tokens, a local speed and failures. The numbers are live, not part of the edition, so an old edition shows today's.

## Alternatives considered

- **Read the run records for model calls** — they cover runs only; chat, the Newsroom, summaries and stack model calls would be missing.
- **Count in each caller** — many places, and a new caller would be forgotten.
- **Wrap outside the recorder** — hides `CloseRun` and `SetDirForRun`, which the run path asserts on the client.
- **A database table** — the data is a few hundred numbers; a file is simpler to inspect and remove.
- **Cost estimates for cloud models** — no price is known to the service for most models; not shown rather than guessed.

## Consequences

- Easier: any later screen can read the same report (the Models page, a daily summary).
- Harder: counts begin on the day this ships; there is no history before it. A crash can lose up to a minute of counts.
- Tokens read 0 (the panel says "tokens not reported") for `/v1` calls and for a cloud model whose provider sends no usage in a stream; the client does not ask for it (`stream_options.include_usage` is not set, and adding it changes every provider's request, so it is left alone).
- Not done: calls by hour, per-run attribution, cost.

## Links

- `backend/internal/platform/modelstats/`, `backend/internal/core/proxy/usagecount/`, `backend/internal/app/bootstrap.go` (`newClientFactory`, `openModelStats`), `handlers/model_stats_handlers.go`; `frontend/src/components/newsroom/NewsroomEditionModelUse.vue`, `utils/modelUse.ts`; ADR-0084, ADR-0083.
