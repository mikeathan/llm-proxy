---
id: ADR-0076
title: The Briefs topic form offers a built-in list of well-known sources and the owner's own list, and copies the pick into the topic
status: proposed
date: 2026-11-22
subsystems: [autonomy]
supersedes:
superseded_by:
---

# ADR-0076: The Briefs topic form offers a built-in list of well-known sources and the owner's own list, and copies the pick into the topic

Proposed: in force when the owner merges the pull request that carries it. It adds to the Brief module (ADR-0075) and the source look-up (ADR-0074) and amends neither.

## Context
- "Follow a website" was a text box for a feed link. Few owners know a site's feed address, and a topic showed only a count of feeds.
- Reuters has no public feed and the AP's is refused (ADR-0075), so some publishers can only be reached as a news search (`site:reuters.com`).
- Searched first (AGENTS.md): the subsystem hub, the capability list, ADR-0073 to ADR-0075 and the code. No curated or user-kept source list existed, and none was rejected.

## Decision
1. **A built-in catalog** of about 40 sources in Go (`sources/catalog.go`): feeds in News, Technology, Business and finance, Science and health and Sport, and publishers that have no feed as news searches. Every feed address was fetched and parsed as RSS or Atom when the list was written; a test with no network checks that each entry passes the checks a topic applies on save, that labels and targets are unique and that each category is listed.
2. **The owner's own list** is part of the Brief store file (`briefs.json`, `my_sources`, at most 100), not the module's settings document, which holds one number and would be overwritten by a save. Erasing the module erases it. An entry is a name, a kind (feed or news search) and a target, checked as a topic checks it, and refused when it repeats one already in either list.
3. **One route pair**, `GET|POST /admin/api/newsroom/briefs/sources` and `DELETE .../sources/{id}`: GET returns the built-in entries then the owner's, each marked `builtin`. Built-in entries cannot be changed.
4. **A topic copies the pick** (`{kind, label, target}`) and never stores a reference. Removing an entry, or a later change to the catalog, alters no topic. The label is kept so a topic can show "BBC News" and not an address.
5. **The form.** The feeds field is a chip box that suggests sources while typing (name, address or group; Arrow keys, Enter or a click), accepts a pasted feed link (`http(s)://…`) and refuses other text rather than turn a half-typed word into a feed. Under it, "Browse well-known sources" lists the groups, the one matching the topic's name first; a press adds or removes a feed, or adds a publisher search to the topic's searches. The owner's list is managed on Set up, under "My sources". A pasted link that is in neither list is read once with the watch form's look-up (ADR-0074) and shown under the field: what it gives, or the problem in words, or, for a page that is not a feed, the feeds it advertises (a pick replaces the link); a found feed can be kept in "My sources" from there, under the name the feed gives itself. The check is a hint and never stops the topic from saving.
6. **The look-up routes serve both forms**, so they refuse only when Watches and Briefs are both switched off (ADR-0074 tied them to Watches alone).
7. **"Write it now" reads first.** It collects the edition's topics that were never collected or are older than the edition's interval (`Collector.CollectStale`), so an edition written outside the collection window is not made from an empty pool. A section that was never read says so, not "nothing new".

## Consequences
- Easier: following a known site is a pick, not a search for its feed address; a topic names its sources.
- Harder: the list can go stale (a publisher moves its feed): the topic then shows the source problem as for any feed, and the list is edited in code. The owner's own list is only as good as what they paste.
- Not done: an assistant tool that reads the catalog (the route exists, nothing calls it), and offering the list in the watch form. A pasted link is checked on its own, not on every keystroke, and each distinct link is one look-up while the form is open (the stack remembers answers for ten minutes).

## Alternatives rejected
- **A list kept in the screen only.** An assistant tool could not read it. (No tool reads the catalog yet: see Not done.)
- **A reference to a list entry in the topic.** Removing an entry would break topics.
- **The owner's list in the module settings.** A settings save writes the whole document.
- **A fourth Briefs sub-tab for sources.** One more place to look for one job; the list is a card on Set up.
