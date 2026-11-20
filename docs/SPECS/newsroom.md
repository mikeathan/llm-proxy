---
id: SPEC-013
title: Newsroom
version: "1.0"
status: stable
last_updated: 2026-11-20
constitution_references: [I.1, III.6, IV.6, V.2]
related_specs: [SPEC-011, SPEC-009, SPEC-003]
supersedes:
---

# SPEC: Newsroom

## Changelog

- **1.0 (2026-11-20)** — First version. The Newsroom is a stack (`stacks/newsroom`, module `topics`) that follows topics, reads their sources on a timer, keeps what is new in a feed and, for a topic with a channel, hands the core a notice. The Morning Paper module, the email connector and Reddit and Bluesky sources are not built (plans: `autonomy/morning-paper.md`, `cross-cutting/email-connector.md`).

## Contract

**Topics.** A topic has a name, one to ten sources, an interval (default 1 h, floor 5 min, sent as a Go duration string), an on/off flag and an optional channel (a connector type; empty means the feed only). A topic made by the assistant is on and feed-only. Stored in one file, `topics.json`, in the stack's data folder; a factory reset empties it.

**Sources** (guarded `Fetch` only: GET, no header, 1 MiB): RSS or Atom (also GitHub releases), Hacker News search, Mastodon tag, a web page (compared by its visible text), a public Telegram channel preview (per-channel opt-in, at most one read per 10 min). Each kind's target is checked when a topic is saved (`sources/validate.go`, mirrored in the form by `utils/newsroom/sourceCheck.ts`): an https address with a host for a feed or page, 1 to 200 characters for a search, a 4 to 32 character channel name, a bare tag and a plain host for Mastodon. Pasted forms are reduced to the bare value first: `https://t.me/s/aipost` or `@aipost` gives `aipost`, `#golang` gives `golang`, `https://Mastodon.social/` gives `mastodon.social`. `Fetch` itself stays strict. A source that cannot be read is `failing`, never "nothing new".

**A run** (one topic at a time; the timer loop ticks every minute, a manual run is refused while the module is off): read every source; items already in the seen ledger (60 days, 2000 per topic) are dropped; the first successful read of a topic is a silent **baseline**; for new items the model step (the `model` port, ADR-0071) says which are new, changed (a was and now pair) or repeats of a shown recent title, and writes a short state note. Code checks the answer: a repeat must cite a shown title; a wrong answer can only lengthen a notice. With no model chosen, or the daily limit (200 calls) spent, items go to the feed as new with the row "not compared". Any other model failure is `analysis_failed`: nothing is recorded and the run retries.

**Delivery.** The core words and sends the notice through the `notice` port (ADR-0072); the stack never sends. A refused channel shows `no_channel`; a failed send records nothing and retries. Every change is kept in the feed whether or not a message is sent.

**Feed.** Entries are kept 30 days, 200 per topic. The view is one card per topic per day: the newest entry current, every earlier entry of the day beneath it; repeats fold under the entry they repeat ("also seen"). Days are drawn in the zone the screen sends (`?zone=`), else UTC.

**Routes** (admin): `GET|POST /admin/api/newsroom/topics`, `PUT|DELETE .../topics/{id}`, `POST .../topics/{id}/run`, `GET .../feed` (`topic`, `unread`, `zone`), `POST .../feed/read`. **Tools** (attended runs, `general` profile only, hidden and refused while the module is off): `newsroom_follow`, `newsroom_topics`.

**Module.** `newsroom.disabled` is the switch. Starting needs internet access allowed in Guardrails. No timer runs while the module is off.

**Screens** (SPEC-003): the Newsroom page, tabs Feed, Topics, Sources and Settings (the module switches), at `/newsroom/:tab?`; a dashboard card `newsroom-latest` that shows the latest headlines and a failing flag and links only (ADR-0064). All text is escaped; a link opens only if it is http or https.

## Capabilities

| Capability | Where |
|---|---|
| RSS/Atom, Hacker News, Mastodon tag, page change, Telegram preview sources | `stacks/newsroom/sources/` |
| Topics, seen ledger, baseline, runner | `stacks/newsroom/topics/` |
| Model step with code-checked answer | `topics/analyst.go`, core `stackport/model.go` |
| Notice through the core | `stackport/notice.go` |
| Feed journal and day cards | `topics/journal.go`, `feed.go` |
| Chat tools | `internal/app/newsroom_tools.go` |
| Newsroom page and Home card | `frontend/src/views/NewsroomView.vue`, `components/newsroom/`, `components/dashboard/cards/NewsroomLatestCard.vue` |
