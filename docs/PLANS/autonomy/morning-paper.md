---
status: partial
date: 2026-11-21
last_reviewed: 2026-11-21
related_specs: [SPEC-013, SPEC-011, SPEC-007, SPEC-003]
constitution_references: [I.1, II.13, III.2, IV.6, V.2]
related_plans: [topic-watch.md, newsroom-watches-handoff-2026-11-20.md, standing-goals.md, attention-policy.md, ../cross-cutting/reminders-stack.md, ../cross-cutting/network-radio-joined-by-tag.md]
evidence: Read 2026-11-21 - stacks/newsroom (watches, sources), stacks/sdk/ports.go (Model, Fetch, StackFinding), backend/internal/core/stackport/model.go, backend/internal/app/stack_model.go, frontend/src/styles/tokens.css; live feed tests on 2026-11-21 (curl from a Mac, then through vertex's own look-up endpoint /admin/api/newsroom/sources/lookup); owner answers of 2026-11-20 and 2026-11-21. The filename is kept for existing links; the plan is now called Briefs.
---

# Briefs: editions of news, your watches, your day and your systems (the Brief module of the Newsroom stack)

**Status:** proposed (2026-11-21). Design only; no code. Mocks: [`briefs-mocks/`](briefs-mocks/README.md) (edition page and settings, dark, light, phone).

## What the owner asked for

- A **brief** the owner can have in the morning, the evening or whenever they choose: **several editions**, each with its own time, days and sections.
- **News the owner defines**: topics and queries (a major event, economics, politics, technology, world, a region), from **as many providers as can be used for free**, plus providers that need the owner's own key (later).
- Also in the brief: **what the watches found** (the watch feed summary), **reminders and agenda**, **network and security** (Network and Radio merged in one block).
- Its **own tab**, with a lot of settings that stay **clean, professional and easy**: Simple by default, Advanced behind a switch, mobile friendly.
- A **polished report**: sections, artwork, collapsible sections.
- The owner chooses **how many hours the collection runs** before an edition.

## Earlier owner answers (2026-11-20, kept from the first version of this plan)

| Question | Answer |
|---|---|
| Sections | Watched topics; Today (reminders + agenda); Home (Network + Radio findings). **Not** held-back notices. |
| Where it is read | A dashboard card plus a Telegram message with a short summary and a link |
| When, how long | 07:00 daily, about a 5-minute read. **Changed 2026-11-21:** several editions, each with its own time and days. |
| Own searches | Round 2: the owner wants to configure news topics (for example "Technology" plus keywords); a topic can **fetch feeds, search the web, or both, within a search budget**. The free news sources above come first; **web search through the search tool** (it spends the owner's search quota) stays an optional per-topic source with a daily limit, off by default (the global daily cap of `topic-watch.md` D4 applies). |
| Presentation | Configurable and nicely presented: the owner chooses what goes in; sections, artwork, collapsible sections. |

## Concepts

| Word | Meaning |
|---|---|
| **Brief topic** | A news area the owner defines: name, category or region, queries (words, or "site:reuters.com"), a source list, stories per section, ignore words, language, age limit. It is **not** a watch (a watch alerts on change; a topic feeds a ranked section). Watches and topics share the source readers, the guarded fetch and the model gate, not their settings. |
| **Section** | One block of an edition. Types: **News topic**, **Your watches**, **Your day**, **Home**. Each can be switched off and reordered. |
| **Edition** | A named brief with a "ready by" time, days, length (short, normal, long), an ordered list of sections and where it goes (dashboard, optionally Telegram). Morning and Evening are two editions. |
| **Pool** | The collected headlines (headline, publisher, time, link, text when the source gives it), kept for 3 days and shared by every edition. |

## What an edition contains (in this order, each dropped when empty)

1. **Top stories**: the few most important items across all news sections, one lead and up to three more. Each shows how many sources reported it.
2. **News sections**, one per brief topic. Each story: headline, one or two sentences, source chips, "also reported by", posted time, link.
3. **Your watches**: what the owner's watches found since the last edition (their stored headline and summary), and each watch's health (for example "a source cannot be read", with the fix).
4. **Your day**: today's reminders and agenda; the evening edition shows tomorrow.
5. **Home**: Network and Radio findings joined by the core (new or missing devices, over-the-air sightings, patches waiting). One block; if Radio is off it says so and shows Network only.
6. A footer: how it was made (sources read, model calls, the rule that a line without a source is not shown).

Rules: **already shown** (a story shown in an edition within the last 24 hours is not shown again unless it developed, and then it is labelled "update"); **late or failed sections are labelled**, not hidden, and the edition still ships; no story appears without an item the code supplied (citation rule, Constitution V.2).

## Architecture

**Collection is fetch only and uses no model.** Every N hours during the M hours before an edition (owner's setting; for example every 2 hours for 12 hours), a collector reads the topic's sources through the guarded fetch, drops duplicates and stores them in the pool. Duplicates are found by **normalised title** (publisher suffix removed) as well as by link, because one news-search result has no stable publisher link. A source's own politeness floor holds (Telegram: one read per channel per 10 minutes).

**Watches and briefs share one reader registry (`sources` package).** The Telegram floor is held per reader instance, and a reader is cached today by kind, label, instance and target, so the same channel under another label gets a second reader and a second slot. A collector with its own readers would read a channel a watch already read and break owner decision D3; one sharing only the slot would instead starve the brief (the watch read at :00, the collector gets "too soon" and the pool misses those posts). The proper design is a slot keyed by channel, where a second reader inside the window reuses the last answer. **Until that is built, brief topics may use only sources with no politeness slot (feeds, news search, Hacker News); Telegram channels in briefs wait for it.**

**The edition pass uses the model, once per section plus once for Top stories**, and starts **15 to 20 minutes before "ready by"** so a slow or cold model cannot make it late. The code first caps each section to its best candidates (a single news-search query returns 100 items) and only then asks the model to rank, write a headline and a one- or two-sentence summary, citing item numbers. A line with no valid citation is removed. A **headline-only** item (no source text) gets no model-written summary, because a summary written from a bare headline is invention the citation rule cannot catch.

**Limits that shape this** (read in `stacks/sdk/ports.go`, `stackport/model.go`, `app/stack_model.go`): a stack's model call is **one in flight**, **200 a day** (a fixed default until the owner can set one), **2000 answer tokens**, 64 KB prompt, 16 KB answer, 120 s. It is **not** admitted through the run lane (`stackModelAsk`), so briefs and watches never take a chat's or an automation's place; they do use the owner's chosen model (a local model competes for the GPU, a cloud model does not).

- **One queue inside the stack.** Today a second caller is refused ("a call is already running") and a watch run ends as an analysis failure until its next tick. With briefs the stack waits its turn instead (bounded), so a 07:00 edition and a 07:00 watch tick do not collide.
- **Budget** (worst case, per day): watches up to 24 calls each, plus editions x (sections + 1). Example: 3 watches (72) + 2 editions x 6 (12) = 84 of 200. The settings page shows "model calls today" (mock).

**Where it lives:** the Brief module of the Newsroom stack, with its own store (editions, topics, pool, shown-ledger), written atomically in the owner's data folder, retention: pool 3 days, editions 30 days. The stack reads the watch journal directly (same stack). Reminders and agenda, and Network and Radio findings, come **only through SDK ports** (IV.6): findings by id exist (`StackFinding`, ADR-0036); **a reminders and agenda port is not in the SDK and is new work**.

## Sources

Tested 2026-11-21 **through vertex's own look-up** (the production guarded fetch, which sends no browser header) unless stated.

| Reads | Source |
|---|---|
| Works | BBC (Top, World, Business, Politics, Technology, Science), The Guardian (World, Business, Technology), Al Jazeera, NPR, NYT (World, Business), France 24, Sky News, ABC Australia, Japan Times, SCMP, Euronews, FT (headlines), CNBC, Ars Technica, The Verge, TechCrunch, Wired, Google News (top and search) |
| Does not | AP's own feed (403), The Independent (cannot be read), DW (its feed's root is `rdf:RDF`, an RSS 1.0 document the feed reader does not accept; fixable) |
| None exists | Reuters has no public feed (the old feed is gone, the agency address is 404) |

Curl with a browser header disagreed on TechCrunch and Euronews (403 and 406 there, readable through the app), so **only the app's own path counts as evidence**; the catalog must be re-tested through it whenever it changes.

**Reuters and AP, and any topic query, through Google News RSS search** (`news.google.com/rss/search?q=...`, e.g. `site:reuters.com when:1d` returned 100 current Reuters headlines). Observed limits:

- The title ends in " - Publisher" (strip it using the `<source>` element); the description repeats the title, so there is **no snippet**.
- The link is an opaque Google page (`news.google.com/rss/articles/...`). Fetching it with a script did not reach the publisher. Opening one in a fresh headless Chrome (JavaScript on, no cookies; UK region) ended on Google's **"Before you continue" consent page**, not the article. So **it is not verified that the link opens the article**: a browser that already accepted Google's consent will probably follow it, a first visit may show the consent page first. The owner should click one in their own browser before this is promised (D6).
- So these are **headline only**: shown with the publisher, no model summary, labelled "headline only" in the mock.
- The feed's own notice says it is made available "solely for the purpose of rendering Google News results within a personal feed reader for personal, non-commercial use". This is a personal, local-first install, but it is the owner's call (D6).

**Sources that need the owner's key** (news APIs) are a **later phase**: the stack's fetch is GET with no header (SPEC-013), a key in a header needs an SDK option, and a key in a query string would leak into stored targets and logs. It is a contract and secret change, so it needs the owner's decision first (D4). Provider terms are not stated here; they are unverified.

## Presentation

Matches the app's design language (tokens, fonts, hairline panels, square status tags, the offset-shadow primary button); see the mocks.

- **Edition page:** masthead (name, date, ready time, read time, how many sources and when collected), edition switch (Morning, Evening, earlier editions), Collapse all, Print; a **lead story**; **collapsible sections** (native `details`, open by default for the first two, each with its artwork plate, story count, update time and "new since last edition"); a sidebar with Your day, Home and Your watches that drops under the news on a phone. A print style keeps it readable on paper.
- **Artwork:** **bundled line art per section type** (world, economy, technology, politics, science, day, home, watches), drawn with theme colours, so it follows light and dark and works offline. **No remote images**: publisher thumbnails would make the owner's browser contact third parties about what they read (egress and privacy; the stack fetch guard does not cover a browser). Remote thumbnails, if ever wanted, are a separate owner decision (D5).
- **Settings tab "Briefs"** (SPEC-003): left, the editions list; right, the edition (name, ready by, days, **collect every N hours for M hours**, length, send to), its sections (drag to reorder with Move up and Move down buttons as the keyboard path, switch, Edit), the section editor (a news topic: name, what it covers as removable chips, sources grouped "free, no key" and "need your own key", stories), **Advanced** (ignore words, language, age limit, skip what the previous edition showed), and "Model calls today". Simple shows only what most people change. Every problem is shown where it happens with its fix (a source that cannot be read, an edition that was late). Targets are 44 px on a phone (the phone mocks do this).
- **Dashboard card:** the latest front page (lead and headlines), links only (ADR-0064).

## Decisions for the owner

**Answered 2026-11-21: the owner accepted every recommendation in the table (D1 to D10).** One reversal, with the owner's reason: **Telegram carries the Home block** (network and radio findings), "because it is important to get updates when I am not home and cannot reach vertex". The earlier recommendation (dashboard only) is withdrawn. The notice port is shaped around one watch (`NoticeRequest`: Watch, Items, Changes), so a brief summary with Home lines needs a new notice shape in the SDK (Phase 6). Telegram stays an owner choice per edition (the "Send to" switch).


| # | Decision | Recommendation |
|---|---|---|
| D1 | Collection through the day, model only at edition time | Yes |
| D2 | A quiet day: send nothing, or one Telegram line? | Dashboard card only, no push |
| D3 | The Telegram link: which address does the phone use? | Reuse the connector's public host setting if set; else text only |
| D4 | Providers that need a key: how is the key sent (an SDK header option, or a per-provider reader in the core)? | Decide before building; a later phase |
| D5 | Remote images from publishers | No; bundled art only |
| D6 | Google News search as the way to reach Reuters, AP and topic queries | Yes, labelled headline only, once the owner has clicked one link in their own browser (a consent page may show first) |
| D7 | Retention: pool 3 days, editions 30 days | Yes |
| D8 | The stack's 200 model calls a day is fixed in code; briefs need it settable | Make it an owner setting before the Brief module ships. **Done 2026-11-22:** `newsroom.model_calls_per_day` (default 200, ceiling 2000, applies at once); the screen field is under Briefs, Set up |
| D10 | Web search as a per-topic source (earlier answer, round 2) | Optional, off by default, with its own daily limit |
| D9 | Does the brief replace the attention-policy digest, or does that plan add held notices later as a section? | The second |

## Phases

0. **Contract and plumbing:** edition, section, topic and pool types and the store; one shared reader registry; the stack's model queue (wait, bounded) with tests; the already-shown rule. *Accept:* table tests; a replay gives the same edition twice.
1. **Collector and source catalog:** feed and news-search readers into the pool, title de-duplication, the catalog re-tested through the app's fetch. *Accept:* a collector run twice stores each headline once; a failing source is named once on the topic.
2. **News sections:** candidate cap, one call per section and for Top stories, citation enforcement, headline-only handling, early start, late-section labels. *Accept:* a test with an injected fabricated line removes it; a headline-only item has no summary.
3. **Edition page, dashboard card and Briefs settings** from the mocks, with Simple and Advanced. *Accept:* the checks in the frontend rule file; screenshots at desktop and phone in a light and a dark theme.
4. **Your watches** (reads the watch journal and health) and **Home** (findings by id through the core's join). *Accept:* an unread watch source shows with its fix; a finding appears once.
5. **Your day:** a reminders and agenda port in the SDK, then the section. *Accept:* the port is plain data and survives a JSON round trip (the SDK's own test).
6. **Telegram summary and link** (needs D3).
7. **Providers with a key** (needs D4).
8. **Docs:** SPEC-013 (or a new SPEC), an ADR, the hub, learnings, the capability list.

## Risks

A late or failed source leaves a stale section (each section shows its age). The model restates a fact wrongly (citation rule; no free text from stacks). A very long edition (a per-section size cap). The model's daily limit shared with watches (D8). Google can change or refuse the news-search feed (the section then says so; nothing else breaks). Two plans own "a morning digest" (D9).

## Progress (2026-11-22, later): phase 3 started

A **Briefs tab** on the Newsroom page reads the stored edition and sets up topics and editions (SPEC-013 1.11). It is plain on purpose: no masthead, lead story, artwork plates or print style yet, and the sidebar blocks wait for phase 4 and 5. Checked: component and unit tests, lint, type check and build. Looked at in headless Chromium with mocked answers (dark desktop, paper phone, the setup form); `npm run test:visual` not run (its pinned browser is not installed).

## Progress (2026-11-22): phase 2 built

The edition pass is built and tested (`brief/compose.go`, `publish.go`, `issues.go`; SPEC-013 1.10): candidate cap, one call per section plus one for Top stories, citation enforcement, headline-only handling, the already-shown rule, late/stale labels, the 20-minute early start with a 3-hour catch-up, stored editions, and the routes to make and read them. Checked 2026-11-22 against vertex's Qwen3.6 35B A3B (thinking off, as production sends it) with a six-story sample: valid JSON, every entry carried its `item`, headline-only stories got an empty summary, and a headline that said "ignore previous instructions" was left out. One sample, not a soak; the top-stories call was not replayed. The per-edition "Send to" and the Home/Watches/Day sections are phases 4 to 6.

## Progress (2026-11-21, branch newsroom-source-error-reporting)

Built, with tests: the stack's **model gate** (callers wait their turn); the shared **reader registry** (`sources.Registry`; Telegram in briefs still waits for a slot keyed by channel); the **news-search source** (`newssearch`, headline only) for watches and briefs, with the form's Region field and guide; **Phase 0 and 1 of the back end**: topics, editions, the pool, the collector, the Briefs module (second switch, status, factory reset, schedule port) and its routes. Nothing the owner can see yet: no Briefs screen, no edition. Details: ADR-0075 and SPEC-013 1.8.

## Remaining Work

In order: the rest of phase 3 (the dashboard card, the masthead, lead story, artwork and print style from the mocks); **Your watches** and **Home** (phase 4); **Your day**, which needs a reminders and agenda port in the SDK (phase 5); **Telegram** with Home lines, which needs a brief notice shape in the SDK (phase 6); providers with a key (phase 7, D4). D8 (a settable model-call limit) is done in settings.yml; its screen field is part of phase 3.
