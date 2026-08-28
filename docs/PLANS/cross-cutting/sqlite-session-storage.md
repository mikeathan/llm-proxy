---
status: proposed
last_reviewed: 2026-08-20
---

# SQLite session storage (future work — NOT part of the current bug-fix session)

**Status:** ⏳ Proposed (documented for later implementation; deliberately out of scope for the current reload bug fixes)

**Subsystem:** cross-cutting (persistence layer `WorkspaceManager`, assistant session storage)

## Why

llm-proxy currently stores each assistant session as a **JSON file on disk**
(`{sessionID}.json` under the workspace `sessions/` dir), written atomically via
`storage.WriteAtomic` (`backend/internal/platform/persistence/workspace.go`).
Hermes stores its transcript in SQLite. This plan captures the migration as a
tracked, separate effort — the current bug fixes (running-session reload +
persisted-history ceiling) deliberately kept the existing JSON storage and must
not be entangled with a storage-backend migration.

Motivation for moving to SQLite (not a fix for the truncation bug, which is
about *what* is persisted, not *where*):

1. **Scalable listing.** `ListSessions` currently reads **every** session JSON
   file in full to build the sidebar (O(sessions × file-size) per call). With
   SQLite, `SELECT id, snippet, updated_at, source ... GROUP BY session_id`
   avoids full-file reads and unbounded memory as sessions grow.
2. **Structured per-message rows** (`role`, `content`, `reasoning`, `tool_call_id`,
   `tool_calls`, timestamps) make "last N tool calls of session X" cheap to query
   without parsing whole JSON files — directly useful for the reasoning-bubble
   reconstruction and a future FTS search (Hermes adds an FTS index).
3. **Atomic multi-row transactions** for frequent checkpoints (every tool result)
   instead of temp-file+rename per file.
4. **Full history without a global char cap.** Row-based storage removes the need
   to bound file size via whole-message truncation; the LLM prompt is separately
   bounded by the existing sieve / `context_budget`.

## Constraints / current facts

- SQLite drivers are **already in `go.mod`**: `github.com/mattn/go-sqlite3` and
  `modernc.org/sqlite`. `modernc` is used by the existing memory + ledger stores
  (`orchestrator.db`). No new dependency.
- Current JSON API surface to migrate: `WriteSession`, `ReadSession`,
  `ListSessions`, `DeleteSession`, `collectSessionIDs`, `firstUserSnippet`
  (`backend/internal/platform/persistence/workspace.go`), plus the checkpoint
  path in `conversation_service.go` (`buildObserver`) and the session-dir layout
  (`SessionsDir`, `sessionOldDir`).
- `SessionBrief`/`AssistantSession` models already exist (`backend/models/`).

## Decisions to make at implementation time

1. **Separate `sessions.db`** vs. **new table(s) in the existing `orchestrator.db`**
   (which already holds memory + ledger). A separate DB isolates blast radius and
   WAL traffic; the shared DB avoids another file. Trade-off to weigh.
2. **Schema shape** (Hermes-style vs. simpler):
   - (a) **Per-message rows** (`messages` table, one row per message) — queryable,
     FTS-friendly, mirrors Hermes, larger refactor.
   - (b) **Session row with JSON `history` blob** — minimal refactor, keeps the
     current `AssistantSession` shape, no per-message querying.
3. **Migration path** for existing `.json` session files (one-time import on first
   read, or a dedicated migration command).
4. **Concurrency** — WAL mode; whether sessions share one connection or use a
   pool; interaction with the existing memory/ledger DB if shared.

## Scope guardrail

This plan is **proposed and deliberately separate**. It must NOT be implemented
as part of the running-session-reload bug-fix session. The bug fixes persist
full history in the existing JSON files (bounded by `MaxPersistedHistoryChars`)
and are storage-agnostic; SQLite is a follow-up that can build on them.

## References

- Hermes transcript storage: `hermes_state.py` `messages` table + `context_compressor.py`
  (full transcript persisted; only the prompt is compressed by summarization).
- llm-proxy current store: `backend/internal/platform/persistence/workspace.go`
  (`WriteSession`/`ReadSession`/`ListSessions`).
- Related docs: `docs/PLANS/cross-cutting/session-source-backend-driven.md`
  ("Checkpoint persistence during execution"), `docs/PLANS/cross-cutting/persist-assistant-run-state-for-reload.md`.
