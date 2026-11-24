---
status: partial
date: 2026-11-24
last_reviewed: 2026-11-24
related_specs: [SPEC-013, SPEC-003]
constitution_references: [I.16, IV.6, V.2]
related_plans: [morning-paper.md, topic-watch.md]
evidence: Branch briefs-day-telegram-card-home, cut from origin/main at 4a9251f7 (#304, v0.128.0) on 2026-11-24, uncommitted. Checks as listed below. This file replaces the handoff of the same day that described #304, whose work is merged.
---

# Newsroom Briefs: handoff (2026-11-24, after #304)

## State
- **Branch:** `briefs-day-telegram-card-home`, from origin/main at `4a9251f7`, committed, pushed and opened as a pull request on 2026-11-24 (merge is the owner's call; the PR number is on GitHub). Version file says 0.129.0. `frontend_dist/index.html` is rebuilt and committed with the frontend change.
- **Live check on vertex (2026-11-24, GET only):** the owner's new Morning brief (issue 4) was written at 18:24 from 317 outlets, with 4 top stories and a Home block of 3 lines. Its three topics make **two sections by design** (two share the category Technology). The Evening edition had no issue yet (created 18:22, due 20:00). Not checked: anything of this branch, which vertex does not run yet.

## Built (v0.129.0, ADR-0083, SPEC-013 1.22)
- **Dashboard Brief card:** shows the Home, Watches and Your day blocks beside the news on the large size; the fold control moved from after the open edition into the card header.
- **Sidebar dot:** `NavItem.dot` / `NavGroup.dot` (a plain dot with no count, apart from the run-notification `badge`) on Newsroom while a brief is unread, in the sidebar, the folded Stacks group, the phone tab bar's More and its sheet. It reuses the one brief poller.
- **Your day:** read-only `briefday` port (`stackport.Day` over the Reminders stack's list), `Issue.day`, an edition switch, the block on the page and the card; reminders only (the product has no calendar or agenda source). Titles are not in a prompt and not in Telegram.
- **Telegram summary:** `Notice.Brief` (the core words it: name, up to five headlines with links, Home facts re-worded by id and count), an edition `send_to` switch, `Publisher.Deliver` (once per scheduled issue, marked only after the connector took it, retried for 3 hours, never for a hand-made issue). The link is the origin of the connector's stored **https** webhook address, else text only.
- **`/design` visual baselines** regenerated (16 PNGs under `frontend/e2e/__screenshots__`, the only visual cases that failed, with the same 16 before this branch's edits). Not looked at one by one.

## Checks run (on the final tree)
Backend `go build ./... && go test ./...` (pass); `go run ./tools/check-complexity/` (no new violation; two functions I wrote were split first); stack `GOWORK=off go test -race . ./sources ./watches ./brief` (pass) and `stacks/sdk` tests (pass). Frontend `npm test` (377 files, 3562 tests), `npm run build` (pass), `npm run test:visual`: 422 passed, 2 skipped and the 16 `/design` failures **before** regenerating them; after `playwright test e2e/design.visual.spec.ts --update-snapshots` those 20 cases pass. **Not run:** anything against a running vertex build; a real Telegram send; `./scripts/check-agent-harness.sh` is run at the end of this change (see the final message).

## Open question: vertex memory (owner reported 3% "used" at idle, rising over about a week)
- **What the 3% is (read 2026-11-24, GET `/admin/api/metrics`):** `mem_used_mb` 1,439 of 47,950 MB, which is the **whole machine** (total minus available), not the service. One sample shows no growth and does not say llm-proxy is the cause.
- **Review of this PR for memory:** it adds no long-lived state (`Deliver` and `Unsent` copy at most 3 hours of issues a minute, which is freed; the Day port reads the reminders list once per edition; the sidebar poll runs in the browser). Not the likely cause.
- **Bounds read in the Newsroom stack (all bounded, small):** pool 3,000 items for 3 days with `Text` clipped to 1,000 runes (`MaxTextRune`); issues 100 for 30 days; watches journal 200 per watch for 30 days; look-up cache 64 entries for 10 minutes; fetch bodies read through a limit. **One unbounded thing, minor:** `sources.Registry` keeps a reader for every distinct source spec and never evicts, so an edited source leaves its old reader. The store saves its whole JSON document on every write (a short peak, not a leak). Not read: Radio, the Network stack, the core's own stores.
- **Owner to collect on vertex, now and again a few hours later:** `ps aux --sort=-rss | head -15`; `systemctl status <llm-proxy unit>` (Memory and peak lines; the unit name was not looked up); `grep -E 'MemAvailable|AnonPages|Shmem|Slab' /proc/meminfo`. Steady climb of the service's RSS means a leak; a plateau after a spike means a high-water mark (a nightly audit, a capture window); a big non-llm-proxy process or tmpfs means it is not the service.
- **Decided against (2026-11-24, owner):** adding Go heap and process memory to the metrics payload. If the numbers point at llm-proxy, first read Radio and Network state for caps, then reconsider it.

## Left, in order
0. **Owner:** send the memory numbers above; then read the next step from them.
1. **Owner:** review ADR-0083 (it amends ADR-0077), merge, deploy. Then on vertex: switch **My day** on for Morning and Evening and press "Write a new one now"; check the block shows today's (Morning) and tomorrow's (Evening) reminders.
2. **Telegram needs an https address.** The owner is on http and a Telegram webhook needs https, so the connector's stored webhook is empty or http and the message goes **text only** until that changes. To try the summary: switch "Send a short summary to Telegram" on for an edition with a scheduled time; it goes when the next scheduled issue is written (a hand-made one is never sent).
3. **Not built:** the Briefs page says nothing when Telegram refused a summary (the connector is not enabled); a refusal is silent, a failure is a warning in the stack log. A quiet-day line (D2) was not re-answered, so nothing is sent for an issue with nothing to say.
4. Most stories are still headline only (Google News links carry no text). Add direct feeds (BBC, the Guardian and similar) in Set up: content, no code. The Nvidia-type event should now be one story after #304; the model must follow the prompt line.
5. Phase 7 (providers with a key) is dropped (D4 "not now").

## Traps
- `go test ./...` does not work in `stacks/newsroom` (workspace): name the packages, with `GOWORK=off`. `HomeLine` must keep exactly `ID, Count, Words`.
- `Notice` has two methods now: a fake of `stacksdk.Notice` in a stack test needs `Brief`. `editionJoining` rebuilds an edition edit by field: a new flag must be carried there (a test pins `day` and `send_to`).
- A component that calls `useBriefUnread` must be unmounted in its test, and a test of the shell mocks `BriefService.latest` (the sidebar polls).

## First command
`cd stacks/newsroom && GOWORK=off go test -race . ./brief`, then `stacks/newsroom/brief/send.go` and `backend/internal/core/stackport/day.go`.
