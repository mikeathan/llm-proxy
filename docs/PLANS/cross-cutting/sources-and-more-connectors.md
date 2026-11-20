---
status: proposed
date: 2026-11-20
last_reviewed: 2026-11-20
related_specs: [SPEC-009, SPEC-007, SPEC-006]
constitution_references: [I.1, II.2, III.2, IV.6, V.2]
related_plans: [../autonomy/topic-watch.md, ../autonomy/morning-paper.md, connector-auto-reply.md]
evidence: Read 2026-11-20 - SPEC-009, connector-patterns skill, models/config.go (ConnectorConfig), notifiers/telegram.go (allowed_updates is ["message"] at line 261), docs/guides/automation-digest.md; web searches on Telegram channel updates, Reddit Data API, Bluesky searchPosts, Telegram user-account (MTProto) access; curl tests from the dev Mac (not vertex). Reddit's own policy pages were not reachable (403); every Reddit claim here is third-party and unconfirmed.
---

# Sources and more connectors — what can be listened to, and what the owner must bring

**Status:** proposed (2026-11-20). Research plan; it feeds `../autonomy/topic-watch.md` (what it listens to) and says what a chat connector needs. Nothing built.

## The owner's request

Extend the connectors so channels can be added for **listening**; add more free sources and paid ones when the owner supplies keys; support Reddit
(and its RSS) and Bluesky; and find out how other communication channels could be added and what the owner must provide.

## What exists today (verified)

| Fact | Source |
|---|---|
| One connector type exists: `telegram` (`ConnectorTypeTelegram`); the config is generic (type, settings map, secret ref) and a new platform "needs only a new case in the registry switch" | `models/config.go`, SPEC-009 §3 |
| It **sends** (`sendMessage`) and has an **inbound webhook** for the owner's own chat, secured by a mandatory secret header | `notifiers/telegram.go`, connector-patterns skill |
| The webhook is registered with `allowed_updates: ["message"]`, so **channel posts are not delivered today** | `notifiers/telegram.go:261` |
| Connectors are rebuilt live on every settings change, so a factory must do no network work or goroutines | connector-patterns skill |
| Secrets are stored in the secrets store, not in config; a masked value means "keep what is stored" | `models/config.go` (`MaskedSecret`) |

## Two roles, kept apart

A **connector** is a conversation with the owner: it sends, and may receive the owner's messages. A **source** is something the service reads: a feed, a channel, a
search. Telegram can be both. Recommendation: one **Communication** settings area with two lists, **Connectors** (talk to me) and **Sources** (listen to
these), one secret store, one egress grant per host. A source never replies and never runs an agent turn by itself; its items go to Topic Watch.

## Telegram: three ways to listen

| Way | What the owner must do | Reach | Cost / risk |
|---|---|---|---|
| **A. The bot as a channel administrator** | Add the existing bot to a channel as an admin; use the channel id (a negative number starting -100). We re-register the webhook with `channel_post` allowed | Only channels where the owner (or a friendly admin) can add the bot | Free, push, no polling. Bot API documents that channel posts reach a bot only if it is an administrator (Bot API changelog 2.3, found by search, not re-read) |
| **B. The public web preview** (`t.me/s/<channel>`) | Name the channel | Any **public** channel with a preview | Free, no login. A page fetch, not an API; Telegram's terms were not read. Tested working 2026-11-20 |
| **C. A user account (MTProto)** | `api_id` and `api_hash` from my.telegram.org, the owner's phone number and a login code, a stored session | Anything the account can see, including private channels it joined | New heavy dependency (AGENTS.md: ask first); a stored login session is a high-value secret; third-party guides warn automation can get an account limited. Not recommended for v1 |

Recommendation: A and B in v1, C only if the owner needs a channel neither can reach.

## Keyed and keyless web sources

| Source | Needs from the owner | Verified today | Notes |
|---|---|---|---|
| RSS / Atom (any site, GitHub releases) | The feed URL | GitHub releases feed: 200 | No key |
| Hacker News search | Nothing | 200 | Keyless |
| Mastodon hashtag or account | An instance name; a token only for private data | 200 (mastodon.social) | Per instance |
| **Reddit** | Reddit developer credentials: an app (client id and secret) created at reddit.com/prefs/apps, plus the account's login for a script app, and an approval under Reddit's Responsible Builder Policy | **No.** reddit.com and `.rss` returned 403 from this host; Reddit's policy pages were unreachable | Blogs say new public API access closed 2026-10-31 and the rest ends March 2027. **Unconfirmed.** First owner step: try creating the app and tell me what Reddit says. If it is refused, Reddit stays out and RSS from vertex's address is the only try |
| **Bluesky** | A handle and an **app password** (Settings, App Passwords) | **No.** Public `searchPosts` returned 403 | Web reports: search needs a login and is flaky even with one; an account's own feed may work without search. Test from vertex with the app password before promising search |
| X | A developer account and a bearer token; usage-based charges | No | Pay-per-use per search titles; terms and price not read; ask the owner before building |
| Web search (Tavily and the others in the repo) | The existing provider key | Existing | Governed by `search_mode` and the global cap (topic-watch) |

Every source host needs the operator's egress grant, and a request goes through the guarded dial. A source that fails is shown as "failing", never as "nothing new".

## Other chat connectors

Facts below are from general knowledge of those services, **not re-checked today**; they are what to ask the owner before building any of them.

| Connector | The owner provides | Inbound reach | Public address needed? |
|---|---|---|---|
| Discord | A bot token (developer portal), the bot invited to a server, the message-content intent switched on | Gateway connection (a long-lived outbound socket) or interactions webhook | Not for the gateway; needs a lifecycle-tethered goroutine (Constitution II.2) |
| Slack | A Slack app: bot token plus either an events URL or an app-level token for Socket Mode | Events API or Socket Mode | Not for Socket Mode |
| Matrix | Homeserver address, a user and an access token | Sync polling | No |
| Email | IMAP and SMTP host, user, an app password | IMAP polling or IDLE | No |
| WhatsApp / Signal | Business API account and approved number (WhatsApp); no supported bot API (Signal) | Webhook (WhatsApp) | Yes; heavy; not recommended |

The existing Telegram webhook needs a public HTTPS address; a long-poll or socket mode would not. That choice (D3) decides whether a connector can work from a home server with no public address.

## Design

1. **`Source` interface**, parallel to `Connector`: `Name()`, `Fetch(ctx, since)` returning typed items (title, canonical link, time, source id, plain text). It lives in the watch stack
   or the core by the Topic Watch decision D1; the credentials stay in the core's secret store and cross a vault port, never into the stack's files.
2. **Registry switch**, as for connectors: a new type is one case plus a factory that does no network work.
3. **Telegram channel_post**: extend the webhook parser and the registration (`allowed_updates` gets `channel_post`), keep the secret check, ignore a channel the owner has not
   listed, and route the post as a **source item**, not as an agent chat turn.
4. **Settings UI** (Communication tab): Sources list with type, name, status (working, failing, needs credential), last item time, a **Test** button that fetches once and
   shows the first items. One form per type listing exactly what the owner must bring (tables above).
5. **Each item is untrusted text** (V.2): bounded length, cleaned, never run as an instruction, never rendered as markup.

## Decisions for the owner

| # | Decision | Recommendation |
|---|---|---|
| D1 | Sources next to connectors in one Communication area? | Yes, two lists |
| D2 | Telegram: A and B in v1, C later? | Yes |
| D3 | Add a second chat connector now? If yes, which (Discord, Slack, Matrix, email)? And may it need a public address? | None in v1; pick one after the watch works; prefer one that needs no public address |
| D4 | Reddit: will you try to register the app first? | Yes; do not build until Reddit has answered |
| D5 | Bluesky: will you create an app password for a test account? | Yes; test search from vertex |
| D6 | X or other paid sources: now or never? | Later; needs a price check |

## What the owner needs to do (in order)

1. Tell me which Telegram channels: ones you can add the bot to (way A) and public ones (way B).
2. Try registering a Reddit app and report what Reddit says (approval, refusal, or nothing).
3. Create a Bluesky app password (a throwaway account is enough) and keep it ready; it goes into Settings, not into a chat.
4. Say which second chat connector, if any, and whether the server has a public address.
5. Say how much web search you can spend a day on your free plan, so the global cap has a real number.

## Phases

0. **Telegram channel_post (way A)** behind the existing webhook, with tests for: a listed channel is accepted, an unlisted one is dropped, a missing secret is refused.
1. **`Source` interface, registry and the keyless adapters**, each with a recorded fixture. *Accept:* adapter tests; failing source shown as failing.
2. **Telegram public preview (B)**, per-channel opt-in, polite rate.
3. **Sources settings UI** with Test button.
4. **Reddit and Bluesky adapters**, only after D4 and D5, tested from vertex with the owner's credentials.
5. **A second chat connector** per D3.
6. **Docs**: SPEC-009, hub, ADR, learnings.

## Risks

Terms of service of the sources (Telegram preview, Reddit) can change or forbid use; credentials stored for a source widen what a compromised service can
do (hence sealed storage and the vault port); a long-lived listener must be tethered to the root context; a source can flood (rate and size bounds).

## Remaining Work

All of it. First owner step is the list above.
