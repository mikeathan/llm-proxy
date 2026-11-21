---
id: SPEC-013
title: Newsroom
version: "1.2"
status: stable
last_updated: 2026-11-21
constitution_references: [I.1, III.6, IV.6, V.2]
related_specs: [SPEC-011, SPEC-009, SPEC-003]
supersedes:
---

# SPEC: Newsroom

## Changelog

- **1.2 (2026-11-21)** — The relevance judgement is a **verdict on every item** (`verdicts: [{item, why, relevant}]`), not a list of the items to keep. Only `relevant: true` keeps an item; an item with no verdict is kept; the old `matches` list is no longer read. A Telegram entry no longer shows its text again under a headline that is the start of it.
- **1.1 (2026-11-20)** — "Topic" is now **Watch** everywhere (ADR-0073). A watch says what to look for (`look_for`, keywords, skip words); a keyword stage and a model relevance judgement filter new items; the first check shows up to five findings marked "First check". The page is Feed, Watches and Settings. Persisted names changed (`watches.json`). The unused Search, Paper and Section fields were removed.
- **1.0 (2026-11-20)** — First version. The Newsroom is a stack (`stacks/newsroom`, module `topics`) that follows topics, reads their sources on a timer, keeps what is new in a feed and, for a topic with a channel, hands the core a notice. The Morning Paper module, the email connector and Reddit and Bluesky sources are not built (plans: `autonomy/morning-paper.md`, `cross-cutting/email-connector.md`).

## Contract

**Watches.** A watch has a name, one to ten sources, an interval (default 1 h, floor 5 min, sent as a Go duration string), an on/off flag and an optional channel (a connector type; empty means the feed only). It may say what to look for: `look_for` (the owner's words, at most 300 characters), `keywords` (an item must contain one) and `skip` (an item containing one is dropped), each list at most ten words of 40 characters, lower-cased and de-duplicated. A watch made by the assistant is on and feed-only and takes the same fields. Stored in one file, `watches.json`, in the stack's data folder; a factory reset empties it.

**Sources** (guarded `Fetch` only: GET, no header, 1 MiB): RSS or Atom (also GitHub releases), Hacker News search, Mastodon tag, a web page (compared by its visible text), a public Telegram channel preview (per-channel opt-in, at most one read per 10 min). Each kind's target is checked when a watch is saved (`sources/validate.go`, mirrored in the form by `utils/newsroom/sourceCheck.ts`): an https address with a host for a feed or page, 1 to 200 characters for a search, a 4 to 32 character channel name, a bare tag and a plain host for Mastodon. Pasted forms are reduced to the bare value first: `https://t.me/s/aipost` or `@aipost` gives `aipost`, `#golang` gives `golang`, `https://Mastodon.social/` gives `mastodon.social`. `Fetch` itself stays strict. A source that cannot be read is `failing`, never "nothing new".

**A run** (one watch at a time; the timer loop ticks every minute, a manual run is refused while the module is off): read every source; items already in the seen ledger (60 days, 2000 per watch) are dropped; keywords and skip words drop more (recorded as seen, no model); the first successful read of a watch is a **baseline**: nothing is announced, but up to five of the items that match are put in the feed marked "First check" (with `look_for` set and no model, none); for new items the model step (the `model` port, ADR-0071) says which are new, changed (a was and now pair) or repeats of a shown recent title, and writes a short state note. When `look_for` is set the same call gives a verdict on every item (`why`, then `relevant` true or false); an item it called not relevant is recorded as seen and named in the row ("N did not match what you look for"). An item with no verdict, or an answer with no "verdicts" list, keeps every item; the reason text is never read to decide. Code checks the answer: a repeat or verdict must cite a shown item or title. With no model chosen, or the daily limit (200 calls) spent, items go to the feed as new with the row "not compared". Any other model failure is `analysis_failed`: nothing is recorded and the run retries.

**Delivery.** The core words and sends the notice through the `notice` port (ADR-0072); the stack never sends. A refused channel shows `no_channel`; a failed send records nothing and retries. Every change is kept in the feed whether or not a message is sent.

**Feed.** Entries are kept 30 days, 200 per watch. The view is one card per watch per day: the newest entry current, every earlier entry of the day beneath it; repeats fold under the entry they repeat ("also seen"). Days are drawn in the zone the screen sends (`?zone=`), else UTC.

**Routes** (admin): `GET|POST /admin/api/newsroom/watches`, `PUT|DELETE .../watches/{id}`, `POST .../watches/{id}/run`, `GET .../feed` (`watch`, `unread`, `zone`), `POST .../feed/read`. **Tools** (attended runs, `general` profile only, hidden and refused while the module is off): `newsroom_follow`, `newsroom_watches`.

**Module.** `newsroom.disabled` is the switch. Starting needs internet access allowed in Guardrails. No timer runs while the module is off.

**Screens** (SPEC-003): the Newsroom page, tabs Feed, Watches and Settings (the module switches), at `/newsroom/:tab?`; the watch form offers suggestions for what to look for and a guide per kind of source; a dashboard card `newsroom-latest` that shows the latest headlines and a failing flag and links only (ADR-0064). All text is escaped; a link opens only if it is http or https.

## Capabilities

| Capability | Where |
|---|---|
| RSS/Atom, Hacker News, Mastodon tag, page change, Telegram preview sources | `stacks/newsroom/sources/` |
| Watches, seen ledger, baseline, runner, keyword gate | `stacks/newsroom/watches/` |
| Model step with code-checked answer and relevance judgement | `watches/analyst.go`, core `stackport/model.go` |
| Notice through the core | `stackport/notice.go` |
| Feed journal and day cards | `watches/journal.go`, `feed.go` |
| Chat tools | `internal/app/newsroom_tools.go` |
| Newsroom page and Home card | `frontend/src/views/NewsroomView.vue`, `components/newsroom/`, `components/dashboard/cards/NewsroomLatestCard.vue` |
