---
status: partial
date: 2026-11-25
last_reviewed: 2026-11-25
related_specs: [SPEC-013, SPEC-003]
constitution_references: [IV.6, V.2]
related_plans: [morning-paper.md, topic-watch.md]
evidence: Review read at 0f941c49 (v0.131.0); built on branch newsroom-reader-and-fixes (v0.132.0), PR #308. Checks listed under "Handoff". Nothing measured on vertex.
---

# Newsroom review and redesign (2026-11-25)

## Handoff

- **Merged and deployed:** #308 (v0.132.0, the reader and the review fixes) and #309 (v0.133.0, the reader's spacing). Not yet checked after the deploy:
  that the next scheduled brief is full (the pool-file change marks every topic due); an old Telegram summary's `/newsroom/briefs` link landing on the
  brief (a browser redirect, curl cannot see it); "Write a new one" showing its steps; a real Telegram send.
- **Alerts follow-up, branch `newsroom-alerts-fixes`** (from origin/main `620c8e1e`), committed and pushed, **PR open** (merge is the owner's call), v0.134.0,
  ADR-0087, SPEC-013 1.29. Built from a read of vertex's live feed (2026-11-25, GETs only: one watch "AI news", 13 alerts, 10 unread, 5 "Changed" with
  "was: unknown"):
  opening an alert's details marks it read; an unread count route and one shared poller for the Alerts tab and the sidebar dot; a changed fact must cite
  the state note or a recent title; the prompt asks the headline and reason not to make a post sound more certain or official than it is; the items the
  model left out are kept 7 days with its reason and shown under "Left out by the model" with "Should have matched"; a written brief can be deleted, or
  the older ones of its brief, from a trash button in the masthead (a deleted slot is not written again). Quiet by default (owner, 2026-11-25: "we don't want a lot of noise"): the alert row is unchanged, Alerts shows a warning line only for a watch that needs a look, the left-out count sits in the switch, and the brief's routine "written automatically, nothing to press" line is gone (shown only when the brief is not today's).
- **Checks run on the branch:** `go build ./...` and `go test ./...` in `backend` (pass); `go run ./tools/check-complexity/` (pass, after
  `keepAside` returned its note to keep `deliver` at 12); `GOWORK=off go test -race -count=1 . ./watches ./brief ./sources` in `stacks/newsroom`
  (pass); frontend `npm test` (392 files, 3617 tests, pass) and `npm run build` (pass);
  `e2e/newsroom-briefs.visual.spec.ts` (22 pass); full `npm run test:visual`: 427 pass, 17 fail = the 16 `/design` cases (failing on every branch since 2026-11-24, they render none of this code) and the dashboard "arranging on a wide screen" case (passed alone, the known load flake). Test noise unchanged by the new poller (774 connection-refused lines with and without it). Mutation checks: removing the citation rule, the left-out bound, `keepAside` or the deleted-slot
  check each fails its test. **Not run:** anything on vertex; the visual spec has no Alerts case yet.
- **Left, in order:**
  1. Owner: review and merge the PR, then deploy.
  2. Mock dropped by the owner (2026-11-25): quiet by default was built instead. Revisit the row only if headlines still overclaim on vertex.
  3. After deploy, on vertex: the next run's entries carry no "Changed" without a citation, the headlines of user posts say they are posts, the Left
     out view fills, and the sidebar dot clears when alerts are read.
  4. Owner's calls carried over: an https address for the Telegram link, direct feeds (BBC, the Guardian) so more stories carry text, dropping the
     two old redirects once no summary from before 0.132.0 is kept. Not built: teaching the filter from "Should have matched" (a prompt change).
- **Traps:** a test that mounts the sidebar must mock `NewsroomService.unreadCount` and call `resetAlertsUnread()` (done in `AppSidebar` and `BottomTabs`); reset it in `beforeEach`, not `afterEach`: `enableAutoUnmount` unmounts after an `afterEach` reset and counts the poller's consumers below zero, so the next test never loads (passed alone, failed in the full run). A view test "follows a row's automation link without opening the run" failed once while `npm run build` ran beside the suite and passed in the two full runs after (load flake, not this change). A left-out entry is unread in storage but never counted (`UnreadCount`, `Feed` filter on `LeftOut`); module-level stores need their
  `reset…` in `afterEach` (`resetAlertsUnread` now too).
- **First file to open:** `frontend/src/views/NewsroomView.vue` (Alerts), then `stacks/newsroom/watches/runner.go` (`deliver`, `keepAside`).

# The review

A review of the whole Newsroom (Feed, Watches, Briefs, the three dashboard cards and the stack) after v0.131.0, then a design proposal with
clickable mocks. Nothing is built. Each finding is marked **verified** (read in the code, with a pointer) or **assumption** (reasoned, not
measured or reproduced).

## 1. Bugs

| # | Finding | Evidence |
|---|---|---|
| B1 | **No time limit on a single fetch.** `stackport.Fetch.Get` has none, `NetworkTools` sets only the dial (30 s) and TLS (10 s) timeouts, and neither the collector nor the watch runner wraps a `Fetch` in a deadline. Every source is read from the stack's one loop (`newsroom.go:239`), and sources are read one after another (`brief/collector.go:136`, `watches/runner.go:368`). So one server that sends headers and then trickles the body holds up watches, collection, writing the edition and the Telegram summary until it stops. | verified in code; not reproduced |
| B2 | **`text-state-warning` / `border-state-warning` are not tokens** (the ledger noted it on 2026-11-22; it has spread since). The send-problem line on the brief (`NewsroomEdition.vue:67`) and the chip border on the "needs a look" chips (`NewsroomBriefChips.vue:14`) draw in the default colour, so the only "needs a look" signal looks like an ordinary chip. Also `NewsroomSourceLookup.vue:91`, the Radio files and `netaudit/AuditOverview.vue`. `npm run build` does not catch it. | verified (`tokenRegistry.ts` has no warning token) |
| B3 | **A card link does not carry the edition.** Every Brief link goes to `toNewsroom('briefs')` with no edition (`NewsroomBriefCard.vue:83,95,147`, `NewsroomTodayCard.vue:43,66`). If the owner picks edition B in the card and presses "Read the whole brief", the page opens on the first unread edition (`NewsroomBriefs.vue` `markShown`) and marks *that* one read. The Today card's section links all land at the top of the page. | verified in code; not clicked through |
| B4 | **An old poll can overwrite a newer one** in `useBriefs`: it has no "newest request wins" guard (unlike `useNewsroom`, which keeps `latest`), so a 10 s poll that started before a save can land after the save's reload and show the old list until the next poll. | verified in code; assumption that it is visible |

## 2. Performance

| # | Finding | Evidence |
|---|---|---|
| P1 | **The Briefs tab reloads five endpoints every 10 s** (`useBriefs.ts:38,99`, `POLL_INTERVAL_MS = 10000`). One of them is `GET /briefs/issues`, which returns up to 30 *full* issues (every section and story). The page needs only the ids and times for the "Earlier briefs" picker, plus the one issue on screen. | verified cadence × payload; size not measured |
| P2 | **Each card instance runs its own pollers.** `useBriefCard` (editions, 60 s), `useBriefChips` (automation activity and network alerts, 60 s), `NewsroomEditionModelUse` (60 s) and `NewsroomLatestCard` (watches and feed, 60 s) each poll per mounted copy. Only `useBriefUnread` is shared. The Newsroom board (Brief card and Latest card) runs 5 timers, about 8 requests a minute, on top of whatever the shell polls. | verified; counts from code |
| P3 | **`briefs.json` is one document** holding topics, editions, the pool (up to 3,000 stories), up to 100 issues and the shown record, and `saveLocked` re-marshals and rewrites all of it on every change (`brief/store.go:77`): each topic collected, each "mark read", each edit. | verified; file size on vertex not measured |
| P4 | **"Write a new one" is one long HTTP request**: it first collects stale topics one after another, then makes one model call per section plus the top stories. On a local model that can take minutes with nothing on screen but "Writing…", and a proxy or browser timeout would cut it. | verified flow; duration assumption |
| P5 | **Writing an edition blocks the watches.** The loop runs watches, then collection, then publishing, in turn (`newsroom.go:239`). While an edition is being written, the scheduled watches are not checked, and a manual "Check now" waits at the model gate (`modelgate.go`) for its model step. | verified; impact depends on the model's speed |

**Fixes, in order of value:** B1 a deadline per `Fetch` (for example 20 s; the core is the right place, so every stack gets it) · P1 issues list as `{id, made_at, read}` plus one `GET issue` · P2 one module-level store per data source, the way `useBriefUnread` already works · P4/P5 make "write now" a job: `202` plus a status the page polls, run off the loop · P3 split the pool into its own file (measure on vertex first).

## 3. Code design

- **The large Brief card has grown into a second reader** (tabs, top stories, a side column). ADR-0064 says cards show and link, and the
  owner's "one path per job" points the same way. This is the hinge of the design question; see section 6.
- **The same blocks are written three times.** The lead block is in `NewsroomEdition` and `NewsroomBriefCard`. The critical-line list is in
  `NewsroomBriefCard` and `NewsroomTodayCard`. "Which edition, which issue" is in `useBriefs` and `useBriefCard`. One `BriefSummary`
  (density xs/s/m) plus one `useBriefIssue` store would remove all three copies.
- **Two products, two forms.** A watch and a brief topic are both "something I follow, read from these sources", but they have two forms,
  two source editors (`NewsroomWatchEditor`, the brief chip box) and two tabs. See mock 3.
- `sections.find(...)` runs twice per tab in the card template, and the "Earlier briefs" label formats dates inline in the template.
- **The handoff was stale** (done: harvested into the Handoff above and deleted). `newsroom-briefs-handoff-2026-11-25.md` still said the PR was open; the owner says it is merged and deployed, so it
  should be harvested and deleted (`documentation-stewardship`). This review does not touch it.

## 4. Settings worth adding (keep them under Advanced)

Today these are constants. Per edition:

| Setting | Now | Where |
|---|---|---|
| Stories per section | 5 | `brief/compose.go` `StoriesPerSection` |
| Top stories | 4 | `TopStories` |
| Don't repeat a story for | 24 h | `ShownWindow` |
| Only stories newer than | 48 h | `CandidateMaxAge` |

Leave the rest as constants (pool and issue retention, prompt sizes, similarity thresholds): the owner wants a clean, simple UI, and nothing
has shown a need for them.

## 5. Features worth considering

Ordered by value for effort; each is a proposal for the owner, none is agreed.

1. **Deep links** `/newsroom/brief?edition=&section=&story=`. They fix B3 and make every card element land where it points.
2. **"Write now" as a job with progress**: "Reading 6 sources… writing World (2 of 5)…". It fixes P4.
3. **One Follow sheet** for watches and topics (mock 3), with a live preview from the existing look-up.
4. **Alerts in the brief's side column**: the feed's newest findings shown beside the news, so the two halves of the Newsroom meet.
5. **Copy as text / share**: the same plain words as the phone summary.
6. **Keyboard reading**: `j`/`k` for the next and previous story, `o` to open it.
7. **Turn a story into a watch** ("follow this story"): a watch made from a headline's words, which links the brief to alerts.
8. **Story feedback** (more like this / less like this), stored per topic as boost and skip words. It needs a design decision: it touches the
   prompt, so it is a SPEC change.
9. **OPML import and export** for "My sources".

## 6. Design proposal

Mocks: [`newsroom-redesign-mocks/newsroom.html`](newsroom-redesign-mocks/newsroom.html). Open the file in a browser; `?theme=light`,
`?view=reader|cards|follow|phone` and `?notes=0` select the variant. There are PNG renders beside it. All text is sample data.

**The question: what is the focus point?** Today the same brief is drawn three times: the Today card on Home, the big Brief card on the
Newsroom board, and the Briefs tab. Each has its own code and its own polling, and every link from a card lands on a generic page.

| Option | What it means | Against ADR-0064 | Cost |
|---|---|---|---|
| **A · The card is the reader** | Keep the tabbed big card as the place you read; the Briefs tab shrinks to setup and history. | Amend it (cards would control and read) | Low code; reading splits across two places |
| **B · Cards are front pages** | One `BriefSummary` in three densities (one line, glance, front page) on any board. No tabs in a card. Every headline, section and "needs a look" line deep-links into the reader. | Fits it as written | Medium: one component, deep links, remove the card's tabs |
| **C · The Newsroom is a reader** | B, plus the Newsroom page rebuilt as a reading app: it opens on the brief; tabs Brief · Alerts · Following · Settings; a nameplate masthead, a sticky section index, every section open, a side column in reading order, an end mark. | Fits | Highest: the page layout and the IA rename |

**Recommendation: C, built as B first.** B removes the three copies and fixes the links. C then makes the page worth landing on. A is the
cheapest, but it leaves two readers.

**Design moves in the mocks** (each is marked R1–R6, C1–C3, F1–F2, P1 in the file):

- **Information architecture:** the Newsroom opens on the Brief. "Feed" becomes **Alerts**. "Watches" and brief topics become one **Following**.
- **Nameplate masthead:** a thick and a thin rule, a big name, and one mono dateline (date, issue number, outlets, model calls, send state).
  It replaces three rows: the masthead, the status card and the send line.
- **Sticky section index** instead of folding cards and "Collapse all". On a phone it is a row of tabs that sticks under the header, scrolls
  sideways and starts with "Needs a look".
- **Halftone plates:** art generated in the browser from the headline, drawn in the theme's brand ink in the retro dither style. It works
  offline and never loads a publisher's image, which keeps the privacy rule of `NewsroomPlate`. The line-art plates stay as section marks.
- **Coverage meter:** one tick per outlet next to each story, so importance is visible without reading the outlet list.
- **Side column in reading order:** needs a look, your day, alerts, home, model use.
- **Model use without the Home switch (a proposal, not a bug):** SPEC-013 1.23 / ADR-0085 show Model use only under Home, so an edition without "Include my network and radio" never shows it. The mocks show it on its own; this changes the SPEC, so it is the owner's call.
- **End mark:** "You are up to date; the next brief is at 18:00". Square corners, hairlines and colour only for state, as the theme rules say.

**Open choices for the owner:** (D1) A, B or C. (D2) The tab rename (Alerts, Following). (D3) The halftone art, kept or dropped. (D5) Model use shown without the Home switch. (D4) A serif
display face for the nameplate: it would be a new self-hosted font file, so it is not in the mocks; they use Inter at weight 700.

## Remaining work

Nothing is built. Order when the owner chooses: B1 → B2 (with a lint rule that refuses a colour class that is not a token) → B3 and deep
links → B4 → P1/P2 → the chosen design option → P4/P5 → settings → features.
