---
id: ADR-0074
title: The watch form looks a source up before it is saved, through two keyless reads of the Newsroom stack
status: proposed
date: 2026-11-21
subsystems: [autonomy]
supersedes:
superseded_by:
---

# ADR-0074: The watch form looks a source up before it is saved, through two keyless reads of the Newsroom stack

Proposed: in force when the owner merges the pull request that carries it. It adds two routes to the Newsroom stack (SPEC-013); ADR-0071, ADR-0072 and ADR-0073 stay accepted.

## Context
- The owner asked whether the form could search public channels and topics for a source instead of needing the exact name, and whether the other kinds could do the same.
- A source is read only through the core's guarded `Fetch`: one GET, no header, no login (SPEC-013, Constitution I.1 to I.3). Telegram's channel preview (`t.me/s/<name>`) answers only for a name already known; Telegram's own channel search needs a signed-in client. Checked live on 2026-11-21: a name that is not a channel answers a redirect (a page with no messages), `mastodon.social/api/v2/search?type=hashtags` and `/api/v1/trends/tags` answer keyless, Algolia's front page answers keyless.
- The hot-topic suggestions in the 2026-11-20 handoff (item 3) were asked for and not built.

## Decision
1. **A look-up route and a suggest route**, both GET, admin only, following the module switch. `GET /admin/api/newsroom/sources/lookup?kind&target&instance` reads one source once and answers what it is (title, up to three latest items, the cleaned-up value a save would keep) or a plain-words problem. `GET .../sources/suggest?kind&instance&target` lists what can be picked while typing.
2. **The look-up reuses the adapters' own checks, URLs and parsers** (`sources/lookup.go`), so a preview shows what a run would read. It reads through its own reader, never the timer's, so it cannot use up a Telegram read slot and make the next "Check now" wait.
3. **Telegram is a name check, not a search.** The form confirms that a channel has a public preview and shows its title and latest posts. Searching channels by topic would need a login or a third-party directory, and neither fits the stack's rules; it is not built and would need a new decision.
4. **Mastodon gets real suggestions**: the server's trending tags, or its tag search once something is typed. **Feeds** get discovery: a page that is not a feed offers the https feeds it advertises. **Hacker News and a web page** get a preview only; a front page of stories would not help choose a search.
5. **Polite by construction.** The look-up is an explicit button (Mastodon's tag list follows typing after a pause). The stack remembers an answer for 10 minutes (bounded to 64, keyed by the cleaned-up value), does not remember an answer about the moment (internet off, a slow answer), and allows six Telegram look-ups a minute across all channels.
6. **A hint, never a gate.** Saving validates on its own and does not need a look-up.

## Consequences
- Easier: a wrong channel name, tag or address shows before the watch is saved; a pasted link is cleaned up where the owner can see it; a site address finds its feed.
- Harder: two more routes and a small cache to keep right; fixed sentences must keep the remote side's words out of the answer (they are not repeated).
- Not done: channel search across Telegram; Reddit and Bluesky sources; a per-source test of a saved watch.

## Alternatives rejected
- **Search every Telegram channel by topic.** Needs a login or a third party; breaks the guarded-fetch rule.
- **Look up on every keystroke.** Would break Telegram's read gap and flood small servers.
- **Put the look-up in the core.** The sources are the stack's; the core would need to know their URLs.
