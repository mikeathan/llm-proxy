---
id: ADR-0075
title: The Newsroom Brief module collects headlines through the day with no model and writes an edition once
status: proposed
date: 2026-11-21
subsystems: [autonomy]
supersedes:
superseded_by:
---

# ADR-0075: The Newsroom Brief module collects headlines through the day with no model and writes an edition once

Proposed: in force when the owner merges the pull request that carries it. It adds a second module to the Newsroom stack (SPEC-013) and amends nothing accepted; ADR-0071, ADR-0072, ADR-0073 and ADR-0074 stay as they are.

## Context
- The owner asked for editions of a brief (a morning one, an evening one) made of news topics they define, what their watches found, their reminders and their network and radio findings, delivered to a card and Telegram (plan `autonomy/morning-paper.md`, "Briefs").
- The stack's model call is one in flight, 200 a day, 2000 answer tokens, and refuses a second caller (`stackport/model.go`). Hourly watches already use it.
- Reuters has no feed and AP's feed is refused; Google News RSS search is keyless but returns headlines only, with a Google page as the link.

## Decision
1. **A second module, Briefs, in the Newsroom stack**, with its own switch (`briefs_disabled`, beside `disabled`), its own file (`briefs.json`) and its own routes under `/admin/api/newsroom/briefs/`. Topics, editions and a pool of collected headlines are the stack's own data. Brief topics are not watches: a watch alerts on change, a topic feeds a ranked section.
2. **Collecting is fetch only and never calls a model.** An edition names when it is ready (a time and weekdays, read in the stack's timezone through the core's one cron grammar, the schedule port) and how it collects (every N hours for the M hours before). The stack's one loop calls the collector; a topic is read when some enabled edition that lists it is inside its window and the topic is due.
3. **One pool, three days.** A story is one item with a report per outlet, found by normalised title (a news search has no stable link) or by link; a hard cap and the retention window bound it.
4. **Writing an edition is its own step**, once per ready time and started 20 minutes early: one model call per section and one for the top stories, every line citing a story the pool holds (an entry that cites nothing shown is dropped), a headline-only story given no model summary, a story shown in the last day not shown again unless more outlets report it. A missing or spent model leaves the section ranked by outlets with the reason in a note: the edition always ships. (Built 2026-11-22.)
5. **One gate for the stack's model calls**: callers take turns instead of one being refused, so a watch and an edition never fail each other. It adds no limit of its own.
6. **One reader registry** (`sources.Registry`) serves watches and briefs. A brief topic reads only kinds with no politeness slot (feed, news search, Hacker News, Mastodon) until a slot keyed by channel exists; Telegram channels in a brief wait for it.
7. **A news-search source kind** (`newssearch`, Google News RSS) for watches and briefs: headline only, the publisher moved out of the title, no text (a summary of a bare headline would be invention).

## Consequences
- Easier: news from many publishers with one reader; a dead source is named once on its topic; a restart loses nothing.
- Harder: a second module's switch, status and reset to keep consistent; Google can change or refuse the news search (a topic then shows the source problem, nothing else breaks); the link of a news-search item is a Google page, not the article.
- Not done: the edition itself, Your watches, Your day (needs a reminders and agenda port), Home, Telegram delivery (needs a brief notice shape in the SDK), the screens, sources that need a key (how the key is sent is the owner's decision).

## Alternatives rejected
- **Reuse watches as topics.** A watch's settings and run record (alerting, channel, baseline) do not fit a ranked section, and the brief has many more settings.
- **Collect only at edition time.** Slow, sees only what is on the feeds at that moment and misses a story that broke at night.
- **Call the model on every collection.** Costs the daily limit that watches share, for text nobody reads until the edition.
- **A module-less brief inside Watches.** The two have different switches, files and costs.
