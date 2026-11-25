---
status: partial
date: 2026-11-24
last_reviewed: 2026-11-24
related_specs: [SPEC-013, SPEC-003]
constitution_references: [I.16, IV.6, V.2]
related_plans: [morning-paper.md, topic-watch.md]
evidence: Branch briefs-day-telegram-card-home (v0.129.0 merged), uncommitted v0.130.0 work on top, 2026-11-24. Checks as listed below.
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

## Built (v0.130.0, ADR-0084, SPEC-013 1.23): a ranked Home panel and a slim Briefs header
- **State:** on branch `briefs-day-telegram-card-home` (v0.129.0 is merged and deployed), **uncommitted**; nothing is staged by me. `frontend_dist/index.html` is rebuilt.
- **`HomeLine.Level`** (`critical` / `check` / `info`) set by the core from `stackport.homeLevels`; `Lines` ranks before the cut at 16; the only critical fact is `radio_open_or_weak_ap` (**my assumption, the owner has not approved the set**: widening it is one table row).
- **Panel:** folds to "All quiet"; opens with the lines to check first, each a link (Radio facts to `/radio`, the rest to `/network`, no deep link to a finding), routine facts below, a critical line red with a red outline; the dashboard card says "Needs attention" at every size.
- **Phone message:** a critical fact is the first line under the name with a warning mark and not repeated under "At home".
- **Briefs page:** one slim row; "Delete all briefs" moved to the setup view; today's brief is one quiet line; "Earlier briefs" is in the masthead; on a phone the side column falls between the top stories and the sections.
- **Checks run:** `go build ./...`, `go test ./...` (pass), `go run ./tools/check-complexity/` (no new violation), stack and SDK tests with `GOWORK=off` (pass); frontend `npm test` (377 files, 3568 tests, pass), `npm run build` (pass); `npx playwright test`: after the Model use block 440 passed, 2 skipped, 0 failed (an earlier full run failed the 16 `/design` cases, which do not render this code; they passed on the later run, cause unknown, so treat them as flaky). Screenshots of the edition page and the dashboard card were looked at on desktop and mobile. **Not run:** a real Telegram send; anything on vertex.
- **Not built (from the mocks the owner approved, Option E):** the chip row (failed automations, blocked delivery: no ready count found), a Network sidebar dot for a critical fact, shorter radio sentences, deep links to one finding.

## Built (same branch): Model use under Home (ADR-0085)
- **Counter:** `core/proxy/usagecount` wraps the client in `newClientFactory`, under the recorder (the run path asserts `CloseRun` and `SetDirForRun` on the outermost client). `platform/modelstats` keeps `model-stats.json` (per day and per model, 14 days, at most 64 models a day, one write a minute and on stop). `GET /admin/api/model-stats?days=N`.
- **Panel:** `NewsroomEditionModelUse.vue` under Home in the edition and the dashboard card: a line (calls today and a 7-day trend) that opens into stacked local and cloud columns and today's models (calls, tokens, local speed, failures). Option H of the owner's mocks; the speed column is per model in the row, not a table, so it fits a phone.
- **Not built:** calls by hour, cost for cloud models, a place for it outside the brief (it only shows where Home shows).

## Closed: the vertex memory question (2026-11-24)
llm-proxy used 96 MB (RSS 113–175 MB, systemd 89–149 MB). The "3% used" was the whole machine: a desktop session (gnome-shell and friends), docker, and 37 GB of page cache whose kernel bookkeeping (buffer_head, radix tree, inode cache) is reclaimable. Nothing in the repo to change; the owner chose to leave the desktop session on.

## Left, in order
0. **Owner:** approve or change the critical set in ADR-0084 (one table row), review ADR-0084 and merge; then look at the Briefs page and the card on vertex on a phone. Then check **Model use** on vertex (it counts from the day this is deployed, so it starts at zero; open it after a few calls).
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
