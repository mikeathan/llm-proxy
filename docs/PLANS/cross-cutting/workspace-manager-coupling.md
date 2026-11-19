# WorkspaceManager coupling — narrow the concrete type that 19 files hold

**Status:** approved (owner, 2026-11-18). P0 measured 2026-11-18 (§3a); P1 and P2 built for six of the seven files 2026-11-19 (§3b); P3 and P4 not started.
**Related Specs:** SPEC-007 (Automation Dispatcher). No SPEC names the persistence manager; checked `docs/SPECS/README.md`.
**Constitution refs:** IV.4 (no dead code), V.3 (no half-finished work). No new invariant is proposed.
**Origin:** a knowledge-graph run over `backend/` on 2026-10-27 (graphify, code structure only) showed `WorkspaceManager`
(`internal/platform/persistence/workspace.go:38`) as the strongest cross-community bridge (betweenness 0.030, 88 edges,
all `references` edges read from the code).

## 1. Problem, and what the evidence does and does not show

Evidence (verified by grep on 2026-10-27, `persistence.WorkspaceManager` in non-test files outside `platform/persistence`):
19 files in 11 packages hold the concrete type: `app/bootstrap.go`; `core/automation` (`dispatcher.go`, `executor.go`);
`core/chat` (`service.go`, `conversation_service.go`, `conversation_helpers.go`); `core/assistant` (`system_prompt.go`,
`guardrails/guardrails.go`); `core/netaudit` (`runreporter.go`, `ackservice.go`, `scantool/scantool.go`);
`core/tools/automation_journal.go`; `core/toolregistry/registry.go`; `internal/testing/mocks/assistant_service.go`;
five handler files. 26 test files also mention it.

Several of those files call only one or two methods (for example `webhook_handlers.go` reads state; `services.go` uses
`AcquireLock`, `ReadState`, `RecentRuns`, `ListWorkspaces`, `DeleteAutomationRuns`). The manager has about 56 methods across
`workspace.go` (36), `seen.go`, `journal.go`, `baseline.go`, `acks.go`, `reports.go` and `workspace_tree.go`.

**Not shown by the graph (assumption, to be tested in P0):** that the width of the type is a real cost. It may be the
intended single owner of workspace files. The graph shows sharing, not harm. Part of the bridging is test setup.

## 2. Goal and non-goals

**Goal.** Each consumer depends on the few methods it uses, declared where it is used, so a change to the manager does not
ripple across packages and tests can fake only what they need. No behaviour change.

**Non-goals.** Splitting the manager's storage, changing file layout, locking, or any public HTTP or persisted format.
`Config` (369 edges, the top god node) is a separate question; see §5.

## 3. Phases (each ends with a proof)

- **P0 — Measure.** For each of the 19 files, list the manager methods it calls (compiler-assisted: a throwaway narrow
  interface per file, or `gopls` references). Record the table in this plan. Decide per consumer: narrow interface, or leave.
  Proof: the table is committed; no code changed.
- **P1 — Interfaces where they are used.** Add small consumer-side interfaces (Go convention: define at the use site) for the
  clusters P0 finds — likely a run-history reader, a state reader/lock, and the journal/seen/baseline stores that
  `netaudit` and `tools` use. Proof: `go build ./...`; the manager satisfies each interface with a compile-time assertion.
- **P2 — Migrate one package at a time.** Change fields and constructor parameters to the interfaces, lowest-risk packages
  first (`core/tools`, `core/netaudit`), `app/bootstrap.go` last. Proof per package: `go test ./<pkg>/...`, then
  `go test ./...` and `go run ./tools/check-complexity/`.
- **P3 — Shrink the fakes.** Replace test setups that build a real manager only to satisfy a field with small fakes.
  Proof: fewer test files import `persistence`; suite time not worse.
- **P4 — Re-measure.** Re-run the graph over `backend/` and compare the manager's degree and betweenness. Proof: numbers in
  the learnings ledger, whatever they show.

## 3a. P0 result (measured 2026-11-18, compiler-assisted)

Measured with a throwaway `golang.org/x/tools/go/packages` program (kept outside the repo): every method value whose receiver is
`*persistence.WorkspaceManager`, per non-test file outside `platform/persistence`. 73 methods on the type. **Since 2026-10-27 the
`core/netaudit` and `core/tools` consumers already moved to consumer-side interfaces** (`AckStore`, `BaselineStore`, `FixStore`,
`LifeStore`, `ScanLogStore`, `ExpectationStore`, `digestStore`, `findingsview` `Baselines`, `scantool` `AckSource`, `tools`
`JournalStore`, `automation` report writer, each commented "*persistence.WorkspaceManager satisfies it"), so they no longer call the
concrete type and do not appear below.

| File | Methods called | Decision |
|---|---|---|
| `app/netaudit.go`, `app/routes.go` | ListWorkspaces | leave: wiring layer |
| `core/assistant/guardrails/remember.go` | WriteConfig | narrow: one-method `ConfigWriter` |
| `core/assistant/system_prompt.go` | ReadTaskFile | narrow: one-method `TaskFileReader` |
| `core/toolregistry/registry.go` | ReadConfig | narrow: `ConfigReader` |
| `core/chat/conversation_service.go` | ReadConfig, ReadSession, WriteSession | narrow: `SessionStore` + `ConfigReader` |
| `transport/http/handlers/assistant_handlers.go` | DeleteAllSessions, DeleteSession, ListSessions, ReadSession, WriteSession | narrow: the same `SessionStore` (5 methods) |
| `transport/http/handlers/assistant_memory_review_handlers.go` | ReadSession | narrow: `SessionStore` |
| `transport/http/handlers/webhook_handlers.go` | ReadState | narrow: `StateReader` |
| `core/automation/*` (admission, execution, heartbeat, history, journal, notify, scheduling) | TryAcquireLock, ReadState, WriteState, ReleaseLock, ReadTaskFile, ReadConfig, Read/WriteHeartbeatStatus, RecentRuns, ReadJournal, Read/WriteSeen, ListWorkspaces, MetadataRoot | leave: the dispatcher is the owner of run state and locks; an interface would mirror about 14 methods (§4 rejects &gt; 6) |
| `transport/http/handlers/services.go` | 18 methods (workspace, task-file, run, journal, lock CRUD) | leave: the admin facade over the whole manager; splitting it is the WorkspaceService question, not this plan |

**Result:** the real narrowing left is four small interfaces (`SessionStore`, `ConfigReader`, `TaskFileReader`, `StateReader`, plus
the one-method `ConfigWriter`) for seven files; the automation and admin-facade holders stay concrete on purpose. P1 starts there.

## 3b. P1/P2 result (built 2026-11-19)

Each interface is declared where it is used and the manager satisfies it (a compile-time assertion where the type is exported):
`assistant.TaskFileReader` (`LoadAgentsFile`, `BuildSystemPrompt`, `conversationSystemMessage`), `chat.ConversationStore`
(session read and write, config read, task file), `handlers.SessionStore` (the five session methods), the `WebhookHandler` field
(`ReadState`) and `newCachedConfigReader` (`ReadConfig`). Proof: `go build ./...`, `go test ./...`, `go run ./tools/check-complexity/`.

**Not narrowed on purpose: `guardrails.GuardrailEngine` (`WriteConfig`).** The engine checks `persistence == nil`, and the registry passes
`deps.Persistence`, which can be a nil `*WorkspaceManager`. Passed to an interface parameter, that becomes a non-nil interface holding
a nil pointer, the check no longer fires, and "persistence not available" turns into a panic. Narrow it only together with a nil-safe
hand-off (pass the interface only when the pointer is non-nil), and a test for the nil case.

## 4. Risks and checks

- Interfaces that mirror the whole manager add nothing. P0 must reject any cluster larger than about six methods.
- `app/bootstrap.go` wires one manager into many consumers; a missed assertion shows up as a compile error, not at run time.
- Pitfall to check first: `docs/architecture/pitfalls.md` and `docs/learnings/platform.md` for earlier decisions on this type.

## 5. Related follow-ups found by the same run (not part of this plan)

- `Config` is the top god node (369 edges); the same question applies. Decide after P4 whether it deserves its own plan.
- The report's "import cycles" all run through `internal/testing/mocks/assistant_service.go` and test files. They are likely
  an artefact of the file-level graph (Go refuses real package cycles), but this was not checked. Verify before acting.
- Several communities are weakly connected (cohesion near 0.01, for example "System Settings Tests", "Agent Loop and Sieve").
  Low cohesion in a code graph is not by itself a defect; no action proposed.

## Remaining Work

P0 done (§3a); P1/P2 built for six files (§3b). Left: the guardrail engine's `ConfigWriter` with its nil-safe hand-off, P3 (smaller fakes in tests), P4 (re-measure).
