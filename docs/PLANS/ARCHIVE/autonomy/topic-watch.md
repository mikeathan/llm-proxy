---
status: complete
date: 2026-11-20
last_reviewed: 2026-11-20
related_specs: [SPEC-007, SPEC-009, SPEC-006]
constitution_references: [I.1, II.2, II.13, III.2, IV.6, V.2]
related_plans: [../cross-cutting/sources-and-more-connectors.md, autonomy-roadmap.md, standing-goals.md, event-driven-triggers.md, ../automation/heartbeat-seen-ledger.md, morning-paper.md]
evidence: Read 2026-11-20 - autonomy and automation hubs, standing-goals, event-driven-triggers, heartbeat-seen-ledger, automation-learning-journal, learnings/automation.md, ADR-0006/0036/0051/0068/0069, tools/manifests/search.json; live curl tests of free sources from the dev Mac on 2026-11-20 (not from vertex).
---

# Topic Watch — follow a topic on a schedule and say only what changed (the Topics module of the Newsroom stack)

**Status:** complete (archived 2026-11-25). Built as the Newsroom stack's watches (Alerts); SPEC-013 is the contract. What was still open moved to `../../autonomy/newsroom-handoff-2026-11-25.md`.

## Why this plan exists

The owner wants to follow a topic across the web and social media every few hours and be told only when something changed. Today a recurring
"find news" automation does this by running an LLM with `internet_search` on every tick. That costs search quota (the owner is on a free
plan with limits) and model time, and it repeats itself: the 2026-10-12 vertex audit showed the same five items reported twice, and a story
returning under a new source URL slips past the URL ledger. It also delivered nothing, because no notify connector was set.

## Owner answers (2026-11-20, asked once)

| Question | Answer |
|---|---|
| Sources | **Not constant web search** (free-plan limits). Wants free sources; asked whether the Telegram connection can be searched. |
| What counts as changed | New item **or** a changed fact. |
| Cadence and model | Per-topic interval, default 6 h, cloud model. |
| Delivery | Silent unless something changed, through the Telegram connector. |

**Second round (2026-11-20):**

| Question | Answer |
|---|---|
| Listening to channels | Extend the connectors so channels can be added for listening. Add more free sources, and paid ones when the owner supplies keys. |
| Search use | A setting per topic: web search **every run**, or fetch only, or search a **limited number of times**. |
| Reddit, Reddit RSS, Bluesky | Wanted. |
| Morning paper | Topics also drive the paper (see `morning-paper.md`); one topic list serves both. |
| (2026-11-21) | **Superseded:** briefs have their own topics, editions and store, separate from watches; they share the source readers and the model gate. See `morning-paper.md` (now "Briefs"). |

How sources and connectors are added, and what the owner must provide for each, is in [`sources-and-more-connectors.md`](../cross-cutting/sources-and-more-connectors.md).

## Ground truth

| Fact | Source |
|---|---|
| The search tool takes a query and an optional `time_range`; each call spends provider quota | `backend/internal/core/tools/manifests/search.json` |
| Helper agents research in the cloud, depth 1, cap 3 | ADR-0006 |
| The seen ledger drops table rows and link bullets whose canonical link was already sent; items with no link are always kept; recorded only after a successful send | `automation/digest.go`, `automation/notify.go` (heartbeat-seen-ledger.md) |
| URL-keyed identity misses the same event under a different source URL | vertex replay, 2026-10-12 (heartbeat-seen-ledger.md) |
| With no `notify.connector` nothing is sent and the status still reads `alert` | `automation/notify.go:83-85, 131-140` |
| The Telegram connector in the repo is a **sender** plus an inbound webhook for the owner's own chat | `core/tools/notifiers/telegram.go`; the Bot API gives a bot no way to read a channel it does not administer (Telegram's documented behaviour, **not re-checked today**) |
| Event triggers (file, watch-for-change by hash, after) are proposed, not built; minimum watch interval proposed at 5 min | event-driven-triggers.md D2 |
| A stack keeps its own store and reaches the service through named ports (notify, schedule, guard, vault) | ADR-0035, ADR-0068 |

## Free sources, tested 2026-11-20 from the dev Mac (not vertex)

| Source | Result | Notes |
|---|---|---|
| Public Telegram channels, `https://t.me/s/<channel>` | 200, message text present | No key, no login; only channels with a public preview; reading is a page fetch, not the Bot API |
| Hacker News search (Algolia) | 200, JSON | Keyless; `search_by_date` gives newest first |
| Mastodon hashtag timeline (`/api/v1/timelines/tag/<tag>`) | 200, JSON | Per instance; keyless on the instance tested |
| GitHub releases Atom feed | 200 | Good for "has project X released" |
| Bluesky `searchPosts` (public host) | **403** | Web reports (not Bluesky's own docs) say search is refused without a login and the host is unreliable even with one; test with an app password from vertex |
| Reddit `.rss` and `reddit.com` | **403** | From this address; Reddit's own page says to register developer credentials. Third-party blogs say new API access closed on 2026-10-31; **not confirmed from Reddit** |
| X API | not tested | Search titles say it moved to pay-per-use in 2026; terms not read |

Reddit, Bluesky and X need the owner's credentials; what to bring and how each is stored is in `sources-and-more-connectors.md`.

Any source added later must be re-tested from vertex, because the host's address and the egress grant decide.

## Design (recommended)

1. **A fetch layer in code, no LLM and no search quota.** Source adapters, each a small typed reader: RSS/Atom, HN search, Mastodon hashtag, Telegram
   channel (two modes), GitHub releases, a plain page (fetch + hash), and the keyed ones: Reddit, Bluesky, X. Sources are configured in one place, next
   to the connectors (`sources-and-more-connectors.md`); a topic picks sources from that list. Each returns typed items (title, canonical link, time, source). Every request goes
   through the guarded dial and the operator's egress grant; a new host needs a grant, never a silent allow.
2. **Delta first.** Items already in the seen ledger are dropped in code. Only the remainder reaches the model.
3. **Model only on the delta.** One cloud call summarises new items and compares them with the topic's **last known state** (a short note kept per topic,
   rewritten each run by the framework, not by model-chosen memory writes; the failure in heartbeat-seen-ledger.md). It answers "new", "changed
   fact: was X now Y" or "nothing".
4. **Web search is a per-topic setting** (`search_mode`): `off` (feeds only, the default), `limited` (at most N searches a day, N set by the owner; the counter
   is kept by the framework and a spent budget is shown on the row), or `every_run`. A search result goes through the same delta step as a feed
   item. A global daily cap across topics protects the free plan even when each topic is within its own.
5. **Delivery.** Silent on "nothing". A change goes to the Telegram connector and to the morning paper (`morning-paper.md`). A watch with no
   connector shows a visible warning on its row, not a quiet `alert`. **Open conflict, found 2026-11-20 (blocks Phase 4, not Phase 3):** ADR-0068 gives the notify port to the Reminders stack only, for the owner's own text sent unchanged, and ADR-0036 says the core words anything a stack finds. A notice built from scraped web text and sent by a stack does not fit either. Options for the owner: (a) the core words and sends it from a structured offer, as findings are (the notice is already built from fixed wording plus titles and links, so this is small), or (b) amend ADR-0068 to give Newsroom a notify port limited to a fixed notice shape. **Decided 2026-11-20: (a)** — the core words and sends a notice from the structured offer; the stack never sends. The core's send step (a notice offer pulled or handed to the core, worded from fixed wording plus titles and links, sent to the topic's chosen channel) is core work and is **not built yet**.
6. **Where it lives (decided 2026-11-20).** One new stack, **Newsroom** (`stacks/newsroom`), with two modules: **Topics** (this plan: sources, topics, budget,
   seen items, delta) and **Paper** (`morning-paper.md`). They share one store and one search budget. It has its own store and the schedule port, as Reminders
   (ADR-0068), and the same module switch and Stacks entry; how it sends is the open conflict in item 5, and how it reaches a model is the missing model port (Phase 3). Layers: Sources, then Topics, then Paper. The core automation package is not changed.

## Decisions for the owner

**Answered 2026-11-20.** D3 was answered "yes if we are allowed"; the terms check below gave no clear yes, and the owner then decided to read the public pages anyway.

| # | Decision | Answer |
|---|---|---|
| D1 | Own stack or dispatcher feature | **Newsroom** stack, modules Topics and Paper |
| D2 | Sources in v1 | The whole keyless set (RSS/Atom, HN, Mastodon tag, GitHub releases, plain page) **plus Telegram channels**; the source list is open and grows during the work. Keyed sources (Reddit, Bluesky, X) join when the owner brings credentials |
| D3 | Public Telegram channels through the web preview | **Decided 2026-11-20 by the owner: read public `t.me/s/` pages**, knowing the terms check below is unclear for a personal reader. Per-channel opt-in (the owner lists each channel), polite rate, no login, public-preview channels only; the owner accepts the terms risk |
| D4 | Search | Default is **fetch**; web search only to **confirm** a detected change when needed (a bounded budget). A later card module may use search as a one-off. Modes: `off`, `confirm` (default), `limited`, `every_run`; the global daily cap stays |
| D5 | Interval | Default **1 h**; floor 5 min (event-driven-triggers D2) |
| D6 | Creating a topic | **Both**: a form in the stack page and a chat request the assistant proposes and the owner confirms, one store |

### D3 evidence (read 2026-11-20 from telegram.org)

- `telegram.org/tos/content-licensing`: "Telegram firmly prohibits the scraping, indexing, harvesting, aggregation or use of data obtained from its platform to train, fine-tune, validate or otherwise engage in the development, enhancement, benchmarking or deployment of artificial intelligence, machine learning models and similar technologies", with exceptions only where all relevant users give explicit consent.
- `telegram.org/tos`: scraping is prohibited "as part of its Content Licensing and AI Scraping Terms, which apply to all users, businesses, and third-party services".
- Not answered by either page: whether a person reading public `t.me/s/` pages for themselves, then having a model summarise them, counts. Feeding channel text to a model could be read as "use ... in the deployment of AI". The plan treats it as **not cleared**; the owner's decision to proceed is recorded in D3 and the risk stays with them. Safer routes: the owner's own channels/bot-admin channels via the Bot API, or the channel's own RSS if one exists.

## Decisions of 2026-11-20 (third round)

| Question | Answer |
|---|---|
| Model access | Yes to a model port, **local and cloud models both allowed**: [ADR-0071](../../decisions/ADR-0071-a-stack-may-ask-the-core-for-a-model-answer-through-a-narrow-model-port-with-no-tools-no-memory-and-no-choice-of-model.md) (proposed). The stack never names a model; the core uses the owner's background-work choice |
| Delivery | A topic's notice goes to the **communication option chosen for that topic in the UI**, and every change is also kept in the Newsroom feed (below), so with no connector the owner still sees it |
| Where changes are read | A **dedicated Newsroom tab** (it is a stack), plus a Home card. Not only a message |
| Email | Yes: a send-only SMTP connector, security first; its own plan [`email-connector.md`](../cross-cutting/email-connector.md). Once it exists it appears in the channel list for Topics, Reminders and the Paper |

## The Newsroom tab: how five updates a day are grouped and shown

An hourly topic does **not** make an hourly report. A run that finds nothing new leaves no entry; it only updates the topic's "last checked" time. What the owner reads is a feed of **changes**.

- **The feed** (first sub-tab, as Network's sub-tabs, ADR-0070). Newest first, grouped by **day** (Today, Yesterday, then dates; older days collapsed). Inside a day, entries are grouped by **topic**; a topic block reads "LLMs, 5 updates today, last at 14:00". **One card per topic per day, with the newest finding on top as the current one and every earlier finding of that day still visible beneath it** (owner, 2026-11-20): five hourly finds read as one card with five entries, and an hour that found nothing leaves no trace. Each entry is a small card in the style of a social timeline: source name and icon, time, headline, one line of text, the link, and chips: **New**, **Changed** (the was and now pair), **Also seen at** (a repeat folded under the entry it repeats, so a repeat is visible but not noise). Filters: topic, source, unread; "mark day read".
- **Other sub-tabs:** Topics (create and edit, with the communication option, interval, search mode, "in the paper"), Sources (the list, status working or failing, a Test button), Settings (the module switches).
- **Home cards** (ADR-0064: show and link, never act): a **Newsroom** card with the latest headlines and a failing-source flag; the **Morning Paper** card from the paper plan, a rendered edition with **expanders** per section (Today, Home, each news topic), opening the full edition page.
- **What the store must keep** (new, Phase 4b): an **entry journal** per topic, not only the seen ledger: title, link, source, time, the changed pair, the run it came from, read flag; bounded (30 days or 500 entries). The notice, the feed and the paper all read it. This is also the answer to "no connector": the feed is always the record.
- **UI rules:** all text escaped (Constitution V.2); a link opens the source, never an app route a model named; the tab works on a phone (one column); frontend rule file and the `frontend-vue-engineer` rules apply before any code.

## Phases

0. **Characterise** (no behaviour change): pin what the ledger and `buildDigest` do on two vertex-shaped reports. *Accept:* tests green. **Done 2026-11-20:** `backend/internal/core/automation/digest_characterisation_test.go`. The real run history is not in the repo, so the reports are reconstructed from the audit's description; the test says so. It pins: 4 of 5 repeats suppressed, the same event under a new URL sent again, framing prose surviving, and an unrecorded run retried whole. A failed send recording nothing was already covered in `notify_test.go` ("failed send").
1. **Fetch layer + adapters** with fixtures per source (a recorded response each), canonical links, size and rate bounds. *Accept:* adapter tests; `go test -race`. **Built 2026-11-20 (uncommitted):** `stacks/newsroom/sources` (module `llm-proxy-stacks/newsroom`, SDK only, stdlib only): RSS/Atom (also GitHub releases), Hacker News, Mastodon tag, plain page (hash of visible text), public Telegram channel preview (10 min floor per channel). Recorded responses in `sources/testdata/` (README there). `go test -race -cover` passes (87.5%). **Not done:** the Newsroom stack itself (manifest, store, wiring), the CI `stacks` matrix line for `newsroom` (a CI change: owner approval asked), Reddit and Bluesky (need credentials, tested from vertex), and a live run from vertex through the real guarded port. The fixtures were recorded from the dev Mac, not vertex.
2. **Store + schedule + delta** (seen items, last known state, per-topic interval). *Accept:* a replay of three ticks yields one notice and then nothing. **Built 2026-11-20 (uncommitted), `stacks/newsroom/topics`:** topic model with validation (interval default 1 h, floor 5 min, search mode `confirm` by default), one file `topics.json` through the stack's DataDir (atomic write, corrupt file set aside), a per-topic seen ledger (60 days, 2000 entries, item identity = canonical link plus a page's content hash), `Runner.Tick` (one pass, no goroutine; the timer loop belongs to the stack's `Start`), a `Notifier` seam for Phases 3 and 4. Tests pin: three ticks give one notice then nothing; a failed send records nothing and retries; no channel shows on the row; an unreadable source is `failing`, never `nothing_new`; one failing source does not hide another; a Telegram source waiting out its floor is not a failure; the ledger and topic count are bounded. Mutation checks (record-before-send, floor-as-failure) fail the tests. **Decided (owner delegated the call to the agent, 2026-11-20):** a topic's first successful read is a silent **baseline** (what is already there is recorded, not announced), so a new topic does not send its whole backlog. `State` (last known state) is stored but nothing writes it until Phase 3.
3. **Model step + changed-fact compare** with a mock client. *Accept:* "same event, new URL" is not sent twice. **Built 2026-11-20 (uncommitted), `topics/analyst.go`:** a stack-local `Model` seam (filled since 2026-11-20 over the SDK's `PortModel`, ADR-0071; see `../cross-cutting/next-stack-moves-and-local-models.md` section 1), an `Analyst` that asks for **structured** JSON (changed facts as short was/now pairs, repeats that cite a numbered recent title, a new state note) and a `Notice` whose text is built in code from fixed wording, item titles and links. Rules the tests pin: untrusted text and the state note are fenced in the prompt (a forged end marker is neutralised); the framework keeps only changes on shown items and repeats that cite a shown recent title, so a wrong or injected answer can only lengthen a notice, never hide an item; a judged repeat is recorded as seen and **named on the row** ("1 judged repeats: X repeats Y"); the state note is cleaned, bounded, and rewritten only after the notice was delivered; a failed or unusable model answer is `analysis_failed`, records nothing and retries; a run with nothing new makes no model call; one call sees at most 30 items and the rest wait for the next run; the first (baseline) read makes one call to write the first state note. Residual risk: a model that cites a real recent title for an item that is a different event hides it from the notice; it is shown on the row, not recorded as sent. Mutation checks (record or state before send, no citation check, analysis error ignored, floor as failure) each fail a test.
4. **Delivery** through the notify port; visible no-connector warning. *Accept:* a failed send retries and records nothing.
4b. **Entry journal** in the store (see "The Newsroom tab"): every delivered or unsent change is kept, bounded, with a read flag. *Accept:* three ticks with changes give the entries grouped by day and topic; a repeat is folded under its original. **Built 2026-11-20 (uncommitted), `topics/journal.go` and `topics/feed.go`:** the journal (30 days, newest 200 per topic, text cut to 300 runes, kept in `topics.json`), one entry per finding and no duplicates when a send is retried, the changed was/now pair, a judged repeat folded under the entry it repeats ("also seen"), a sent flag, a read flag, and `Store.Feed`, the view model: day cards in the owner's time zone with Today and Yesterday labels, topics inside a day newest first, `Current` plus `Earlier`, filters by topic, source and unread. A topic has a `Channel` (empty = feed only, status `in_feed`); the feed records every change whether or not a message can be sent. Tests: five hourly finds in a day are one card with the newest on top and four earlier visible; no-connector and failed-send changes are still in the feed once; a time zone moves a late entry to the next day. Mutation checks (no dedupe, split cards, no fold, feed-only marked sent) each fail a test. **Not built:** the HTTP routes, the Newsroom tab and cards (frontend rule file first), and the core's send step.
5. **UI**: the Newsroom tab (Feed, Topics, Sources, Settings), the Home card, the per-topic communication option; **chat proposal** per D6.
   Each topic also has "include in the morning paper" and a section name (`morning-paper.md`).
6. **Docs**: SPEC, hub, ADR, learnings, capability list.

## Risks

Source pages change shape and break an adapter (fixtures catch it; the watch shows "source failing" rather than "nothing new"). Scraped text is untrusted
input to the model (Constitution V.2); it is cut, cleaned and never treated as instructions. Telegram or a site may rate-limit or block the host.

## Remaining Work

All of it. D1-D6 all decided 2026-11-20. Phases 0-3 built (uncommitted); the CI matrix line for `newsroom` is added. Owner decisions made: the model port (ADR-0071 proposed), the core words and sends notices, the email connector (own plan), the feed design. Built in the stack: sources, topics, delta, model step, feed journal. **Next, in core (public-interface changes, SDK and adapters):** the model port, the notice offer and the core's send step; then the stack manifest and `Start` loop, routes, and the Newsroom tab and cards. Then Phase 4 delivery, the stack manifest, the `Start` loop and wiring, the UI.
