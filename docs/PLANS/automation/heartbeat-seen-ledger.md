---
status: proposed
date: 2026-10-12
last_reviewed: 2026-10-12
related_specs: [SPEC-007, SPEC-004]
constitution_references: [II.12, II.13]
related_plans: [memory-and-heartbeat-simplification.md, automation-learning-journal.md, playbook-library-management.md]
---

# Heartbeat Repeat Suppression (the seen ledger)

**Status:** proposed. A heartbeat stores nothing by default. This plan describes how to stop a heartbeat
repeating items it already told you about, if and when you want that, without storing findings in memory.

## Why this plan exists

A live heartbeat on a real host (2026-10-12, two runs, cloud model) re-reported the same five items in both
runs and left 11 permanent memory records behind. Its task file made the **model** the keeper of "what was
already reported": it saved a record per evaluated item tagged `UNREPORTED`, searched memory 7-8 times per
run, and was told to skip items whose record "status is unchanged". Three things made that unsound:

1. **The model cannot know whether a send succeeded.** Only the framework does. With delivery blocked, every
   item stayed `UNREPORTED` and was correctly "retried", so the skip rule never fired.
2. **It costs more and grows without bound.** 92 tool calls and 7m40s on the first run, 44 calls and 4m29s on the
   second; about 4-7 permanent records per run, 4 runs a day, with no reaper for `long_term` entries.
3. **It is a second mechanism.** The framework already has a seen ledger (below) that does this deterministically.

Decision (2026-10-12): a heartbeat writes **no memory by default** (its prompt does not ask for it). Repeat
suppression, when wanted, is the framework ledger, opt-in per heartbeat. `memory_mode` keeps meaning "hot
injection" only (the operator declined making "off" remove the memory tools).

## Ground truth (verified in code and on a live host, 2026-10-12)

| Fact | Source |
|---|---|
| The ledger is a per-automation JSON file of `canonical link -> {title, reported at}`; kept 60 days by default, at most 2000 entries | `models/notify.go` (`DefaultDedupDays`), `automation/digest.go` (`mergeSeen`, `capLedger`), `platform/persistence/seen.go` |
| It is opt-in: needs `notify.connector` and `notify.dedup: true`; a heartbeat has a `notify` block | `models/notify.go:28-30`, `models/heartbeat.go:28` |
| Before a run the newest 25 reported titles are appended to the task as "Already reported"; this is a hint, not the guarantee | `automation/execution.go:97-98`, `notify.go:105-112`, `prompts/templates.go:704` |
| After a run, `buildDigest` drops table rows and link bullets whose link is in the ledger; items without a link are always kept | `automation/digest.go` (`admit`) |
| An item is recorded only after a **successful** send; a failed send records nothing, so the item is retried | `automation/notify.go:183-193` |
| With no connector nothing is delivered, nothing is recorded, and the heartbeat status still reads `alert` (`alert_not_delivered` is used only when a send was attempted and failed) | `notify.go:83-85`, `notify.go:131-140` |
| A replay of the two real reports through `buildDigest` extracted all 5 items from free-form text and suppressed 4 of 5 in run 2; the 5th (GPT-5.5 retirement) came back under a different source link (`orcarouter.ai` then `trustloud.com`) and would be sent twice | scratch replay of `digest.go` on the vertex run history, 2026-10-12 |
| The digest drops rows but leaves the model's surrounding prose ("5 items found", a run-notes section) | same replay |

For comparison, OpenClaw's heartbeat keeps only the last delivered alert text and suppresses an identical one
for 24 hours, written after confirmed delivery (`src/infra/heartbeat-dispatch.ts:428-442, 505-522`). It has no
per-item dedupe, so ours is the stronger mechanism; the shared principle is that the framework, not the model,
records what was delivered.

## Design

**What counts as "reported":** an item the framework delivered through a connector. No connector means no
ledger and no repeat suppression; the heartbeat then only records its report in run history. This is stated in
the UI and docs rather than faked with a pretend "delivered" sink.

**Phases** (each independently shippable, each test-first):

1. **Contract.** Document the heartbeat reply contract for dedupe: item lines only, one source link per line, no
   run notes. Ship the guidance in the heartbeat starter comment and `docs/guides/automation-digest.md`. No code.
2. **Observability.** Log and record per delivery: items in the report, new, suppressed, kept because they had no
   link. A rising "suppressed but model re-found" count shows the hint is not being followed. Small change in
   `deliverReport`.
3. **Honest status.** Add a heartbeat result for "found something, no channel configured" so `alert` stops meaning
   both delivered and unsent (`models/heartbeat.go`, `notify.go:131-140`, status UI).
4. **Stale prose.** Decide, from live runs, whether the digest should drop framing lines ("N items found") when it
   removed rows. Only build it if the live eval shows it matters once phase 1 is in place.
5. **Cross-source repeats.** Measure first (below). Candidate fixes, in order of cost: show the model the host and
   title of recent items in the hint; add a normalised-title secondary key; model-judged matching. Pick only if
   the measured miss rate justifies the added risk of wrongly hiding a genuinely new item.

## Rejected alternatives

| Alternative | Why not |
|---|---|
| Model-written memory as the ledger (the task file's own design) | Non-deterministic; the model cannot know delivery succeeded; 8+ searches and several permanent writes per run |
| Ledger key = link + model-assigned status | The same link gets different status words on different runs, creating false repeats; it does not fix the different-link case |
| A new structured row schema (`item`, `status`, `url`, `date`) | The existing parser already extracted every item from free-form text; a schema adds a contract and regression risk for no measured gain |
| A "delivered to run history" sink so dedupe works without a connector | Marks items as seen that nobody was told about |
| Injecting the whole 60-day ledger into every prompt | Unbounded prompt growth (hundreds of entries); the link filter already covers the full window |
| Making `memory_mode: off` remove the memory tools | Operator declined (2026-10-12); it would change every automation |

## Verification

- Unit: `buildDigest` fixtures from the two real reports pinned as a test, including the known different-link miss
  (documents the limit, fails loudly if behaviour changes).
- Integration: two consecutive runs with a mock model that repeats its findings and a fake notifier: run 2 sends
  only the new item; a failed send records nothing; no connector records nothing.
- Live (needed before any phase 4/5 work): three consecutive cloud runs with a real test connector and
  `dedup: true`; count duplicates, suppressed items, and prose left behind.

## Remaining Work

- [ ] Phase 1 contract text and docs
- [ ] Phase 2 delivery counts in logs and run record
- [ ] Phase 3 honest heartbeat status
- [ ] Live three-run evaluation, then decide phases 4 and 5
