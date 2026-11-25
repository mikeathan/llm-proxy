---
status: partial
date: 2026-11-25
last_reviewed: 2026-11-25
related_specs: [SPEC-013, SPEC-003]
constitution_references: [I.16, IV.6, V.2]
related_plans: [morning-paper.md, topic-watch.md]
evidence: Branch briefs-finish-remaining (from origin/main 9aad8e53, v0.131.0), uncommitted, 2026-11-25. Checks as listed below.
---

# Newsroom Briefs: handoff (2026-11-25, after #306)

## State
- **Branch:** `briefs-finish-remaining`, from origin/main at `9aad8e53` (v0.130.0 merged and deployed), committed, pushed and opened as a pull request on 2026-11-25 (merge is the owner's call; the PR number is on GitHub). Version 0.131.0. `frontend_dist/index.html` is rebuilt from the final tree.

## Built (v0.131.0, SPEC-013 1.24, amends ADR-0083 and ADR-0084)
- **A refused or failed Telegram summary is said.** `Issue.send_state` (`refused` / `failed` / `gave_up`, a fixed word, never connector text; written only when it changes; a send that went clears it). The edition page shows it under the masthead (a refusal links to Settings, Connectors) or "Summary sent to Telegram at HH:MM". The old code comment that promised this notice is corrected.
- **Two send-state rules:** an issue with nothing to say is marked `nothing` (never `sent_at`, so the page does not claim a send), and a refusal is kept after the catch-up (only a failing send becomes `gave_up`).
- **A critical Home fact dots its page.** `GET …/latest` rows carry `critical` (ids); while the newest issue is unread the sidebar and phone More sheet dot Radio (`radio_*`) or Network. Same single poller as the Newsroom dot.

## Checks run
`go build ./...`, `go test ./...` (no failure), `go run ./tools/check-complexity/` (no new violation); `GOWORK=off go test -race ./...` in `stacks/newsroom` and `go test` in `stacks/sdk` (pass); frontend `npm test` (382 files, 3594 tests, pass) and `npm run build` (pass); the Brief and dashboard Playwright specs pass with the card changes (the arranging test on the host board failed once under load and passed alone). Harness: PASS. `npm run test:visual`: 426 passed, 2 skipped, 16 failed: the `/design` cases, which also failed on the two earlier branches. `DesignView` imports none of the changed components (grep), so they are failing and not from this diff; the cause is not found. **Not run:** a real Telegram send, anything on vertex.

## Built (same branch): the dashboard Brief card redesigned, and a Today card on Home
- **Brief card (Newsroom board):** the section chips and "Show every section" are gone (both only expanded the edition at the bottom of the card, off-screen). A real tab list (Top stories and one tab per section, arrow keys, `aria-selected`) switches the body in place, with a fixed minimum height on a wide screen; a critical fact has its own line above the tabs; a side column holds Your day, one Home line ("Home: N to check", a link), Model use and watches. One link out: "Read the whole brief". Smaller sizes unchanged.
- **Today card (Home board, `newsroom-today`, half width):** what needs a look, the next three reminders, the lead and one more headline, a link into each section. It shows and links only (ADR-0064/0081). Added to Home's shipped layout; an existing saved Home gets the "New on this board" banner (found in `DashboardView.vue`, not exercised on an old saved layout).
- **Shared code:** `composables/newsroom/useBriefCard.ts` holds the loading both cards need.
- Chosen by the owner from three mocks (tabs in place, show-and-link, a Home Today card); the mock files were deleted at the owner's request.

## Built (same branch, later): the leftovers
Critical set now includes `radio_rogue_twin` (owner approved). A Home fact deep-links (`homeLineRoute`; Radio findings via `?finding=<id>` on the Radio board card, which now focuses and marks it). Telegram uses short radio sentences (`radiodigest.ShortWords`; listed in the final message for approval). `model-stats` serves `hours` and the open Model use block draws them. A live chip row (failed runs, alerts no connector took) on both cards. **Not built, owner's choice:** cloud cost, a quiet-day message.

## Left, in order
1. **Owner:** review and merge, deploy; on vertex switch the Telegram summary on while Telegram is off and check the refusal line after the next scheduled issue; check the dots, the tabs card and the Today card on a phone; open Model use after a few calls to see the hours.
2. **Owner:** approve the short radio sentences in `backend/internal/core/radiodigest/radiodigest.go` (`shortWords`).
3. **Telegram needs an https address** for a link in the message; until then it is text only.
4. Add direct feeds (BBC, the Guardian) in Set up: content, no code, so more stories carry text.
5. The Dashboard view test prints connection-refused noise from an unmocked poll (not from this branch's code; it passes).

## Traps
- `go test ./...` does not work in `stacks/newsroom`: name the packages, with `GOWORK=off`. `HomeLine` must keep exactly `ID, Count, Words, Level`.
- A fake of `stacksdk.Notice` in a stack test needs `Brief`. `editionJoining` carries every edition flag by field.
- A component that calls `useBriefUnread` must be unmounted in its test; a test of the shell mocks `BriefService.latest`.

## First command
`cd stacks/newsroom && GOWORK=off go test -race . ./brief`, then `stacks/newsroom/brief/send.go`.
