---
status: partial
date: 2026-11-24
last_reviewed: 2026-11-24
related_specs: [SPEC-013, SPEC-003]
constitution_references: [I.16, IV.6, V.2]
related_plans: [morning-paper.md, topic-watch.md]
evidence: Branch briefs-quality-and-polish, cut from origin/main at bf9a6dc4 (#303, v0.127.0) on 2026-11-24; pushed as PR #304. Checks as listed below. This file replaces newsroom-briefs-handoff-2026-11-23.md, whose work is merged (#298 to #303).
---

# Newsroom Briefs: handoff (2026-11-24, after #303)

## State
- **Branch:** `briefs-quality-and-polish`, from origin/main at `bf9a6dc4`. Five commits, pushed, PR #304 (open; merge is the owner's call). Version file says 0.128.0.
- Merged before this: edition page, Your watches, Home (#299), sections (#300 to #302), read state, dots and the dashboard Brief card (#303). Design, owner answers D1 to D10 and phases: `morning-paper.md`.
- **Live check (2026-11-24, GET only):** vertex answers `/briefs/latest` through the core. Morning issue 3 was unread. The unread dot clearing on opening the page was not checked (it needs a POST to `/read`, which would clear the owner's state).

## Built on this branch (v0.128.0, ADR-0082, SPEC-013 1.21)
- Reworded headlines fold into one candidate; outlets counted by `OutletKey`; prompts ask for one story per event; `Issue.outlets_read` and "read from N outlets" in the masthead.
- Earlier briefs picker (newest first; only the newest is marked read); a section's source problem says what to do and opens Set up.
- Print hides the sidebar, header, tab bar, Briefs toolbar and picker; one visual test looks at the edition in nine themes and in print.
- Looked at in the screenshots: retro-sand, retro-plum (desktop), print (desktop). Not looked at one by one: the other six presets (the test checks no sideways scroll in all nine), and the mobile project's screenshots.

## Checks run (all on the final tree)
Stack `GOWORK=off go test -race . ./sources ./watches ./brief`; frontend `npm test` (375 files, 3544 tests), `npm run build`, `npm run test:visual` (422 passed, 2 skipped, **16 failed: every `design.visual.spec.ts` "/design in <theme>" at both widths**, 2% of pixels, the whole page shifted). They fail identically with this branch's shell edits removed, and the branch touches nothing the /design page renders, so they look like environment drift (the baselines are from 2026-10-15); not confirmed against main, and the baselines were not updated. All Newsroom and Briefs visual tests pass. Backend `go build ./... && go test ./...` (pass), `go run ./tools/check-complexity/` (OK, no new violations), `./scripts/check-agent-harness.sh` (pass). Not run: anything that needs vertex running the new build.

## Left, in order
1. **Owner:** review ADR-0077, ADR-0081 and ADR-0082; push, merge, deploy. On vertex, press "Write it now" on Morning and look at whether the Nvidia-type event is now one story (the model must follow the new prompt line; tune `minSharedWords` and `similarShare` in `brief/similar.go` if it does not).
2. Most stories are headline only (Google News links carry no text). Add direct feeds (BBC, the Guardian and similar) to the topics from Set up: a content choice, no code.
3. **Your day** (phase 5): needs an owner decision on whether the owner's own reminder text may pass a port (ADR-0068).
4. **Telegram** (phase 6, D3/D4): a brief notice shape so the New brief can go to the phone; then providers with a key (phase 7).
5. A dot on the sidebar's Newsroom item: needs its own badge kind (see the learnings line of 2026-11-24).

## Traps
- `go test ./...` does not work in `stacks/newsroom` (workspace): name the packages, with `GOWORK=off`. Topic ids and edition ids share one counter in the store.
- Tests that mount a component using `useBriefUnread` must unmount it (module-level state). A `.component.test.ts` name gives a test the DOM.
- The mock admin API matches by path suffix: answer `/briefs/issues/5` and `/briefs/latest` separately.
- `HomeLine` must keep exactly `ID, Count, Words`.

## First command
`cd stacks/newsroom && GOWORK=off go test -race . ./brief`, then `stacks/newsroom/brief/similar.go`.
