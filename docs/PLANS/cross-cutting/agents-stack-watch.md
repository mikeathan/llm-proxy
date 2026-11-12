---
status: partial
date: 2026-11-12
last_reviewed: 2026-11-12
related_specs: [SPEC-006]
constitution_references: [I.1, IV.6]
evidence: Built 2026-11-12 on branch agents-stack-watch-and-isolation-close-out from origin/main c4b4f557 (v0.82.0), uncommitted. Read before building - backend/internal/core/agentwatch (old watch.go, secrets.go and their tests), app/stacks.go, app/stackhost.go, app/netaudit.go, stacks/sdk/ports.go and manifest.go, stacks/network/network.go, core/stackport, archtest/boundary_test.go, ADR-0032, 0033, 0035, 0036, 0038, 0039, Constitution IV.6. Run - `go build ./...`, `go vet ./...`, `go test ./...` (see Checks), `go run ./tools/check-complexity/`, each stack module with GOWORK=off -race. Not run on vertex.
---

# The Agents stack: the watch moves into `stacks/agents`

Owner direction (2026-11-12): "ideally we want to move all agentic logic into the stack so the main service … becomes a pluggable service, but that's a lot
of work", so **in stages, only the logical parts, watch only for now**. Decisions: [ADR-0051](../../decisions/ADR-0051-the-agents-stack-takes-the-watch-and-the-core-keeps-the-walk-the-words-and-the-gate.md).
Parent: [`stacks-as-packages-and-network-stack.md`](stacks-as-packages-and-network-stack.md).

## What moved, what stayed

| Piece | Now |
|---|---|
| The record (`agent-watch.json`: events, destinations, alert bookkeeping, secrets seen), the alert rules and the schedule of the nightly secrets pass | `stacks/agents` (`watch.go`, `secrets.go`, `record.go`) |
| `GET /admin/api/agent-watch` and its view | the stack's route (`view.go`, declared in the manifest, mounted by the core) |
| Tool-name, automation-name and host vetting; the bus tap and the activity gate | core, `core/agentwatch/feed.go` (it sees untrusted model text) |
| The workspace folder walk and `tools.SecretKindIn` | core, `core/agentwatch/scan.go`, behind the new `SecretScan` port: places and kinds only |
| The alert sentences | core, `core/agentwatch/words.go` (`Words`), from a fixed table; the stack publishes a typed `Alert` |
| Lock down (`core/lockdown`) | stays core: the run path calls it |
| The registry's two modules and their switches (`agentModules`, `agentPart`) | stay in `app/stacks.go`, held equal to the manifest by `TestAgentsStack_TheRegistryModulesMatchTheManifestSpecs` |

Removed from the core: `core/agentwatch` watch and secrets code, `persistence.AgentWatchStore`, `models.AgentWatch`, the agent-watch handlers and their route registration.

## The SDK additions

`PortSecretScan` and the `SecretScan` interface; `Observation` and the optional `Observer` interface with `Manifest.Observes`; `Alert` and `EventAlert`.
`TestAgentWatchTypes_CarryOnlyIdsAndVettedTokens` pins every text field, as `TestLookTypes_CarryNoTextFromTheNetwork` does for sensors.

## What is pinned (the byte-identical rule, ADR-0038)

- Every alert sentence, captured from the old code before it was removed: `TestWords_AreTheSentencesTheServiceAlwaysSent` (refusal, odd workspace id,
  unnamed tool, new destination, 1 secret, 2 secrets), and the same path through the real adapters: `TestAgentsStack_ARefusalOnTheBusIsTheSameAlertAndRecordAsBefore`.
- The record file: `TestRecord_AnEarlierVersionsFileReadsBackAndWritesBackWithTheSameKeys` (every kind and field, the stored kind strings), and the file name
  `agent-watch.json` and the settings key `agents` in the manifest test.
- The route's answer: `TestHandle_NewestFirstWithDestinationsAndNoInternals`, `TestHandle_AnUnreadableRecordIsAnErrorNeverAnEmptyRecord`.
- What the stack may reach: `TestStack_ManifestIsValidAndItsPortsArePinned` (exactly `config`, `events`, `log`, `datadir`, `secretscan`: no dial, no fetch, no program).
- The old behaviours kept as tests: one alert a day per key, a chat's refusal is not alerted, a week of learning then a new host, more than 8 hosts while learning
  marks a browsing automation, an unreadable record records and alerts nothing, a corrupt file is set aside, 500 events kept, the queue never blocks, an event
  that does not fit is counted and reported, the switch is read on every event.

## Checks run (2026-11-12)

- `cd backend && go build ./... && go vet ./...`: clean.
- `go test ./...` (final run, after the advisor review fixes): all packages pass. An earlier full run failed `TestNetworkStack_RunsOverTheRealAdaptersAndTheCoreSeesItsSettings` (waits 3 s for the DNS collector's first cycle) under load; it passed 8 of 8 alone and in three later full runs. It does not touch the Agents stack, so it is read as a load-dependent timing flake, not proven one.
- `-race` on `internal/app`, `internal/archtest`, `core/agentwatch`, `core/stackport`: pass.
- `go run ./tools/check-complexity/`: found `stacksdk.Manifest.Validate` at 13 after the new check; refactored to a loop of checks; now no new violations.
- Every stack module with `GOWORK=off go vet && go test -race` (agents, example, network, sdk): pass.
- The isolation fitness test failed first on `privhelper/client`, then passed ([`isolation-importers-switch.md`](isolation-importers-switch.md)).
- **Not run:** `npm test` / `npm run build` (no frontend file changed; the route and its JSON are unchanged); anything on vertex.

## Handoff

Branch `agents-stack-watch-and-isolation-close-out`, cut from origin/main `c4b4f557` (v0.82.0), **uncommitted and unpushed** (VERSION 0.83.0). It also carries the
isolation switch ([`isolation-importers-switch.md`](isolation-importers-switch.md)); they can be one PR or two. First command: `git status -sb && ./scripts/check-agent-harness.sh`;
first file to open: this note, then ADR-0051.

## Format check against the old writer (read, not assumed)

The old store was `rootFile[models.AgentWatch]` at `<RootDir>/agent-watch.json` (`persistence/rootfile.go`, `jsonfile.go`): bare `json.MarshalIndent` of the record with no
envelope, `storage.WriteAtomic` with `ClassUserContent`, temp pattern `agent-watch-*.json.tmp`, set aside as `.corrupt`, and removed with its `.corrupt` copy on a reset.
`stackport.DataDir` uses the same folder, the same bytes (the stack marshals the same keys with `MarshalIndent`, two-space indent, no envelope), the same `WriteAtomic` class and
pattern, and the same `.corrupt` suffix, so the first event after a deploy reads vertex's existing file as it is. This was read from the code on 2026-11-12; it was not
tried against vertex's real file.

## Remaining Work

1. **Owner:** approve a commit and a PR for the branch (nothing is committed or pushed); read ADR-0051 and accept or change it (it is `proposed`).
2. **Owner, on vertex after a deploy:** run `scripts/verify-deployment.py` (Agents stack 2 of 2 modules, the agent-watch route answers); trigger a refused tool
   call from an automation and check the alert text and that `agent-watch.json` kept its entries; switch "Agent activity" off in Settings and check nothing is
   recorded, then on again.
3. **Next stage, if the owner wants more of the agent logic in a stack** (not started; each its own plan): the pieces the owner could move next are the
   ones the run path does not call: the agent profile catalogue and its UI routes (ADR-0009), memory capture review (`core/memorycapture`), and the delegation
   pool. Each needs the same question answered first: can the engine reach it only through a port with plain data? The engine itself (the loop, `tool_exec`,
   guardrails, runlane) is the product and is not a candidate without a new rule (see the owner's note in the conversation of 2026-11-12 and ADR-0051).
4. `Stack.Status` reports the two parts for any caller; the registry does not read it yet (the Agents modules have no monitor id). Wire it if the modules
   screen should show "last run" for the secrets pass.

## Traps found

- **A drift test for alert ids must live in `internal/app`.** The architecture test bans a stack import from anywhere but `app/stacks.go`, test files included;
  `app/stacks.go` re-exports `agents.AlertIDs` as `agentAlertIDs` for the test in `app`.
- **`stackport.Config[T]` takes the typed store that already exists.** `agentsPolicy` satisfies `Store[models.AgentsConfig]` unchanged, so no adapter was written.
- **The bus is attached before the loops start**, so the stack's queue is created in `New()`: an observation before `Start` waits and is handled once running (`TestStack_AnObservationBeforeStartWaitsInTheQueueAndIsHandledOnceRunning`).
- **The secrets pass runs ten minutes after start on its own timer**, so it cannot be driven from an `app` test; it is tested in the stack (fake `SecretScan`) and in the core (`Scanner`, `Words`).
- **Adding a check to `Manifest.Validate` pushed it over complexity 12**; the checks are now a list.
- A `go test ./...` run under full load can time out `TestNetworkStack_RunsOverTheRealAdaptersAndTheCoreSeesItsSettings` (3 s wait); rerun it alone before suspecting a change.
