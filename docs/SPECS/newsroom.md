---
id: SPEC-013
title: Newsroom
version: "1.4"
status: stable
last_updated: 2026-11-21
constitution_references: [I.1, III.6, IV.6, V.2]
related_specs: [SPEC-011, SPEC-009, SPEC-003]
supersedes:
---

# SPEC: Newsroom

## Changelog

- **1.4 (2026-11-21)** — **Source look-up** (ADR-0074): the watch form can look a source up before it is saved. Two admin reads, `GET /admin/api/newsroom/sources/lookup` and `.../sources/suggest`. Telegram is a name check (no keyless channel search exists); Mastodon lists trending or matching tags; a page that is not a feed offers the feeds it advertises. Answers are remembered 10 minutes (64 at most), Telegram is limited to six look-ups a minute, and the look-up never uses the timer's Telegram read slot.
- **1.3 (2026-11-21)** — **Strict filter:** with `look_for` set an item is shown only when the model says `relevant: true`; an item with no verdict is not shown and is named in the row; an answer with no verdicts, no model or a spent limit is `analysis_failed` (nothing shown or recorded, the next run retries). **Clear:** `POST /admin/api/newsroom/feed/clear` deletes entries for good (`ids`, `scope:"read"` or `scope:"all"`). **Screen:** the feed is a flat list of compact rows per day (newest day open, older days folded, "Show older days"), a select mode with Mark read and Clear, no "New" pill (the unread dot says it), details folded behind a chevron.
- **1.2 (2026-11-21)** — The relevance judgement is a **verdict on every item** (`verdicts: [{item, why, relevant}]`), not a list of the items to keep. Only `relevant: true` keeps an item; an item with no verdict is kept; the old `matches` list is no longer read. A Telegram entry no longer shows its text again under a headline that is the start of it.
- **1.1 (2026-11-20)** — "Topic" is now **Watch** everywhere (ADR-0073). A watch says what to look for (`look_for`, keywords, skip words); a keyword stage and a model relevance judgement filter new items; the first check shows up to five findings marked "First check". The page is Feed, Watches and Settings. Persisted names changed (`watches.json`). The unused Search, Paper and Section fields were removed.
- **1.0 (2026-11-20)** — First version. The Newsroom is a stack (`stacks/newsroom`, module `topics`) that follows topics, reads their sources on a timer, keeps what is new in a feed and, for a topic with a channel, hands the core a notice. The Morning Paper module, the email connector and Reddit and Bluesky sources are not built (plans: `autonomy/morning-paper.md`, `cross-cutting/email-connector.md`).

## Contract

**Watches.** A watch has a name, one to ten sources, an interval (default 1 h, floor 5 min, sent as a Go duration string), an on/off flag and an optional channel (a connector type; empty means the feed only). It may say what to look for: `look_for` (the owner's words, at most 300 characters), `keywords` (an item must contain one) and `skip` (an item containing one is dropped), each list at most ten words of 40 characters, lower-cased and de-duplicated. A watch made by the assistant is on and feed-only and takes the same fields. Stored in one file, `watches.json`, in the stack's data folder; a factory reset empties it.

**Sources** (guarded `Fetch` only: GET, no header, 1 MiB): RSS or Atom (also GitHub releases), Hacker News search, Mastodon tag, a web page (compared by its visible text), a public Telegram channel preview (per-channel opt-in, at most one read per 10 min). Each kind's target is checked when a watch is saved (`sources/validate.go`, mirrored in the form by `utils/newsroom/sourceCheck.ts`): an https address with a host for a feed or page, 1 to 200 characters for a search, a 4 to 32 character channel name, a bare tag and a plain host for Mastodon. Pasted forms are reduced to the bare value first: `https://t.me/s/aipost` or `@aipost` gives `aipost`, `#golang` gives `golang`, `https://Mastodon.social/` gives `mastodon.social`. `Fetch` itself stays strict. A source that cannot be read is `failing`, never "nothing new".

**A run** (one watch at a time; the timer loop ticks every minute, a manual run is refused while the module is off): read every source; items already in the seen ledger (60 days, 2000 per watch) are dropped; keywords and skip words drop more (recorded as seen, no model); the first successful read of a watch is a **baseline**: nothing is announced, but up to five of the items that match are put in the feed marked "First check" (with `look_for` set and no model, none); for new items the model step (the `model` port, ADR-0071) says which are new, changed (a was and now pair) or repeats of a shown recent title, and writes a short state note. When `look_for` is set the same call gives a verdict on every item (`why`, then `relevant` true or false); an item it called not relevant is recorded as seen and named in the row ("N did not match what you look for"). An item with no verdict is not shown (named in the row); an answer with no "verdicts" list, no model, or a spent daily limit when `look_for` is set is `analysis_failed`: nothing is shown or recorded and the run retries. The reason text is never read to decide. Code checks the answer: a repeat or verdict must cite a shown item or title. With no model chosen, or the daily limit (200 calls) spent, items go to the feed as new with the row "not compared". Any other model failure is `analysis_failed`: nothing is recorded and the run retries.

**Delivery.** The core words and sends the notice through the `notice` port (ADR-0072); the stack never sends. A refused channel shows `no_channel`; a failed send records nothing and retries. Every change is kept in the feed whether or not a message is sent.

**Feed.** Entries are kept 30 days, 200 per watch. The server groups entries by day then watch (newest entry `current`, the rest `earlier`); the screen reads a day as one list, newest first, and names the watch on a row only when more than one watch has entries. Repeats fold under the entry they repeat ("also seen"). Days are drawn in the zone the screen sends (`?zone=`), else UTC.

**Routes** (admin): `GET|POST /admin/api/newsroom/watches`, `PUT|DELETE .../watches/{id}`, `POST .../watches/{id}/run`, `GET .../feed` (`watch`, `unread`, `zone`), `POST .../feed/read`, `POST .../feed/clear` (`{ids}` | `{scope:"read"|"all"}`; deletes entries from storage for good; the seen ledger is kept, so a cleared item does not return from the same source), `GET .../sources/lookup` (`kind`, `target`, `instance`: what one source would read, as `{found, target, instance, title, sample[≤3], choices[], problem}`; nothing is saved; the value is checked first and a bad one is a problem with no request; a problem about the moment such as internet off is not remembered) and `GET .../sources/suggest` (`kind`, `instance`, `target`: Mastodon only, trending tags or tags matching what is typed; other kinds answer an empty list). Both follow the module switch and read only through the guarded `Fetch`. **Tools** (attended runs, `general` profile only, hidden and refused while the module is off): `newsroom_follow`, `newsroom_watches`.

**Module.** `newsroom.disabled` is the switch. Starting needs internet access allowed in Guardrails. No timer runs while the module is off.

**Screens** (SPEC-003): the Newsroom page, tabs Feed, Watches and Settings (the module switches), at `/newsroom/:tab?`; the watch form offers suggestions for what to look for, a guide per kind of source, and under each source a **Look up** that shows what it would read (name, latest items, the cleaned-up value) and offers feeds a page advertises or a Mastodon server's tags (a hint: saving does not need it); a dashboard card `newsroom-latest` that shows the latest headlines and a failing flag and links only (ADR-0064). All text is escaped; a link opens only if it is http or https.

## Capabilities

| Capability | Where |
|---|---|
| RSS/Atom, Hacker News, Mastodon tag, page change, Telegram preview sources | `stacks/newsroom/sources/` |
| Watches, seen ledger, baseline, runner, keyword gate | `stacks/newsroom/watches/` |
| Model step with code-checked answer and relevance judgement | `watches/analyst.go`, core `stackport/model.go` |
| Notice through the core | `stackport/notice.go` |
| Feed journal and day cards | `watches/journal.go`, `feed.go` |
| Source look-up and suggestions while filling the watch form | `stacks/newsroom/sources/lookup.go`, `sourceroutes.go`, `lookupcache.go`; `frontend/.../NewsroomSourceLookup.vue`, `useSourceLookup.ts` |
| Chat tools | `internal/app/newsroom_tools.go` |
| Newsroom page and Home card | `frontend/src/views/NewsroomView.vue`, `components/newsroom/`, `components/dashboard/cards/NewsroomLatestCard.vue` |
