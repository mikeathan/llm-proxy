---
status: proposed
date: 2026-11-20
last_reviewed: 2026-11-20
related_specs: [SPEC-011, SPEC-007, SPEC-003]
constitution_references: [I.1, II.13, III.2, IV.6]
related_plans: [../cross-cutting/sources-and-more-connectors.md, topic-watch.md, standing-goals.md, attention-policy.md, ../cross-cutting/reminders-stack.md]
evidence: Read 2026-11-20 - attention-policy.md (its digest), autonomy hub, reminders-stack.md, ADR-0036/0064/0066/0068, docs/guides/automation-digest.md, backend/data/templates/llm_ai_release_brief.md (name only), memory notes on the Oct-12 delivery audit and saved reports.
---

# Morning Paper — one newspaper-style edition each morning (the Paper module of the Newsroom stack)

**Status:** proposed (2026-11-20). Depends on `topic-watch.md` for its news section; the other two sections can ship first.

## Why this plan exists

The owner wants one morning report, in the style of a newspaper, instead of separate pings. The pieces already exist in separate places: what
the watches found, today's reminders and agenda, and the overnight Network and Radio findings. Nothing composes them.

## Owner answers (2026-11-20)

| Question | Answer |
|---|---|
| Sections | Watched topics (from `topic-watch.md`); Today (reminders + agenda); Home (Network + Radio findings). **Not** held-back notices. |
| Where it is read | A dashboard card plus a Telegram message with a short summary and a link |
| When, how long | 07:00 daily, about a 5-minute read |
| Own searches? | ~~Compose only~~ **Changed (round 2):** the owner also wants to configure news topics for the paper (for example "Technology" plus one or more keywords); if selected, a web search gets the latest news for it |

**Second round (2026-11-20):** the paper must be **configurable** and **nicely presented**: the owner chooses what goes in. Topics are chosen per section
(a category such as technology, plus keywords); a topic can fetch feeds, search the web, or both, within a search budget.

## Ground truth

| Fact | Source |
|---|---|
| The attention-policy plan also describes a morning digest, but of held-back notices only, and is still `proposed` (D1-D4 unanswered) | `autonomy/attention-policy.md` |
| Reminders hold the due items and the agenda in their own store, reached through a port | ADR-0068 |
| A stack offers findings by id and the core words them; the core never lets a stack's free text reach another stack | ADR-0036 |
| The dashboard is boards of cards; a stack's card follows the stack; the page keeps only control | ADR-0064, ADR-0065, ADR-0066 |
| Automation results can be saved as files under `reports/<automation>/` and delivered through `notify` | SPEC-007 §II.6; saved-final-reports work (#109) |
| Telegram text is capped, so a "newspaper" layout needs a page; a link from a phone needs a host the phone can reach | assumption to confirm (D3) |

## Design (recommended)

1. **An edition is data first.** At 07:00 a scheduled job gathers typed section inputs through ports: watch changes since the last edition, today's
   reminders and agenda, new findings and new devices. Sections with nothing are dropped, not padded.
2. **One cheap model pass lays it out.** It ranks the items, writes headlines and a one-line standfirst per section, and may not add facts: every
   line cites an item id the code supplied, and a line without one is removed (output is untrusted, Constitution V.2).
3. **Stored as an edition** (date, sections, items, generated text) in the owner's data folder, written atomically. The dashboard card shows
   the latest front page; a page lists past editions.
   **Presentation.** The edition page is a newspaper layout drawn from stored data: a masthead with the date and a standfirst, one lead story with a
   large headline, then columns of shorter items under section headings, a sidebar for Today and Home, source names and links on every item, a "last updated"
   note per section. It follows the theme (light, dark) and reflows to one column on a phone; a print stylesheet keeps it readable on paper. All
   text is escaped (Constitution V.2); a link opens the source, never an app route the model named.
4. **Telegram** gets the headline list and a link to the edition. An edition with no news is still a short "quiet morning" card; no Telegram push if
   nothing changed (D2).
5. **Topics are Topic Watch topics.** There is one topic list (`topic-watch.md`). A topic has a name, a category (a preset such as Technology, World, Business,
   Science, Local, or the owner's own), one or more keywords, its sources, a `search_mode` (off, limited to N a day, every run) and a flag "in the
   paper". The paper never searches by itself; at 06:30 it asks the watch to bring each flagged topic up to date within that topic's budget, and
   at 07:00 it reads what is stored. A topic set to search at most once a day is searched once, before the paper. If a topic failed or
   its budget is spent, the edition says so in one line.
6. **Where it lives (decided 2026-11-20).** The **Paper** module of the **Newsroom** stack, beside the **Topics** module (`topic-watch.md`), in one stack with
   one store, so the paper reads topics and their stored items directly, with no port between them. For the other two sections it reaches the
   service only through ports (ADR-0035), never other stacks' files: findings by id (ADR-0036) for Network and Radio, and **a reminders and agenda port,
   which I have not found in the SDK and which is likely new work** (to confirm in Phase 0).

## Decisions for the owner

| # | Decision | Recommendation |
|---|---|---|
| D1 | ~~Own stack or a core module?~~ **Decided 2026-11-20:** the Paper module of the Newsroom stack | done |
| D2 | A quiet morning: send nothing, or a one-line Telegram? | Dashboard card only; no push |
| D3 | The link in Telegram: which address does the phone use? (LAN name, VPN or public host; sign-in is compulsory) | Reuse the connector's public host setting if one is set; otherwise send text only |
| D4 | Does the paper replace the attention-policy digest, or does that plan add held notices later as one more section? | The second; one paper, more sections |
| D5 | Edition retention | 30 days |
| D6 | Presets and defaults for news topics | Ship six presets (Technology, World, Business, Science, Local, Sport); each preset only fills keyword suggestions the owner edits; nothing is searched until the owner turns a topic on |
| D7 | Which settings does the owner control? | Time and days, sections on or off and their order, topics and keywords, items per section, edition length, language, a theme (light, dark, print) |

## Phases

0. **Edition contract**: section types, item ids, the citation rule, the empty-section rule; confirm which ports exist for reminders and findings. *Accept:* table tests; a list of missing ports.
1. **Today section** (reminders + agenda) with no model call; store, card, 07:00 schedule. *Accept:* a replay gives the same edition twice.
2. **Home section** from findings by id (ADR-0036 words). *Accept:* a finding appears once, in the owner's words, with its link.
3. **Layout pass** with a mock model; uncited line removed. *Accept:* test with an injected fabricated line.
4. **News section** from the watch port (needs Topic Watch Phase 2); **Paper settings page**: sections, topics and keywords, presets, time, length, theme. *Accept:* changing a keyword changes the next edition's topic list; a topic with a spent budget is labelled.
5. **Telegram summary + link** (needs D3); edition page.
6. **Docs**: SPEC, hub, ADR, learnings, capability list.

## Risks

A late or failed watch leaves a stale section (the paper labels its age). The model rewrites facts (citation rule, no free text from stacks).
A long edition (size cap per section). Search quota: the paper plus the watches share one global daily cap (topic-watch D4). Two plans own "a morning digest" (D4).

## Remaining Work

All of it. Phases 1 and 2 are useful alone and do not need the watch.
