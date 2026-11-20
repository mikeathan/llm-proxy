---
status: proposed
date: 2026-11-20
last_reviewed: 2026-11-20
related_specs: [SPEC-009, SPEC-006]
constitution_references: [I.1, III.6, V.2]
related_plans: [sources-and-more-connectors.md, ../autonomy/topic-watch.md, ../autonomy/morning-paper.md, connector-auto-reply.md]
evidence: Read 2026-11-20 - CONSTITUTION.md (secrets encrypted III.6, step-up for stored secrets, connector webhooks carry a mandatory secret), models/config.go (ConnectorTypeTelegram is the only type), core/tools/notifiers (telegram.go only), sources-and-more-connectors.md, ADR-0068 (the notify port names a channel type, never an address). SMTP and TLS behaviour below is general knowledge, not re-checked; Go's net/smtp was not read for this plan.
---

# Email connector — a send-only SMTP channel the owner configures, kept out of reach of models and agents

**Status:** proposed (2026-11-20). The owner asked for email support, configurable by the user, with top-level security, encryption and no reach for LLMs or agents.

## Why this plan exists

Telegram is the only connector. Email suits what Topic Watch and the Morning Paper produce (long, formatted, kept) and works with no public address. Because the notify port lists enabled connector types, a new type is available to every stack that sends (Reminders, Newsroom) without changing them.

## Decision: send only

**No inbound.** No IMAP, no webhook, no reply handling. The Telegram connector has an inbound webhook that a stranger could try to reach (hence its mandatory secret); an email connector with no inbound has no such surface, and no outside sender can ever put text in front of an agent through it.

## Security requirements (each is a test, see Phases)

| # | Requirement | How |
|---|---|---|
| S1 | **Encrypted in transit, always** | Implicit TLS (port 465) or STARTTLS (587), TLS 1.2 or newer, the server certificate verified against the system roots. No plain mode, no "skip verify" setting. A server that does not offer TLS is refused |
| S2 | **The password is a stored secret** | The app password lives only in the secrets store (AES-256-GCM, Constitution III.6), never in `settings.yml` or the connector's JSON. The settings API returns it masked; a masked value on save means "keep what is stored" (the existing `MaskedSecret` rule). Set, change and remove need the passphrase confirmed (step-up), as for other stored secrets; a script token cannot |
| S3 | **Out of reach of models and agents** | No tool, prompt, memory, run log, assistant answer or stack port can read the password or the connection settings. The send path receives a channel type and a notice; the host, user, password and addresses are resolved inside the connector. Errors and logs carry fixed sentences, never the library's own text (a server banner or a failed AUTH line can quote them). The config folder is outside every workspace jail. A test greps logs and errors for the secret after a failing send |
| S4 | **A model or stack cannot choose who gets mail** | The recipients, the From and any Reply-To are the owner's settings only. The notify port names a channel type, never an address (ADR-0068), so a stack cannot redirect a message. No CC or BCC from a message |
| S5 | **No header injection** | Subject and body are built in code from fixed templates; every value placed in a header has CR and LF removed and is length-bounded; the body is plain text (a rendered HTML part, if added later, is built from escaped fields by code, never from model markup) |
| S6 | **No mail bombing** | A rate limit (for example 30 an hour, 200 a day, set in code, lower only by the owner), a size cap (256 KiB), and an unbounded retry is refused: one retry with backoff, then the failure is shown |
| S7 | **Guarded egress** | The connection is dialled through the guarded dial, so the internet boundary and the operator's egress grant apply; the SMTP host needs a grant like any host (Constitution I.1). A LAN mail host works only if the operator allows the LAN |
| S8 | **Minimal logging** | One line per send: connector, recipient domain count, size, result code. Never the body, the subject or an address |
| S9 | **Test send is fixed** | A Test button sends one fixed message to the configured recipient; it needs step-up |
| S10 | **No new dependency** | Go's standard `net/smtp` over the guarded connection if it fits (its client is given an already-open TLS connection). To verify in Phase 0; a dependency is an owner decision |

## What the owner provides

SMTP host and port; security mode (implicit TLS or STARTTLS); username; an **app password** (many providers, Gmail included, require two-step verification before one can be made; use a password made for this alone); the From address; the recipient address. The password is typed into Settings, never into a chat.

## Phases

0. **Read and fit.** Read the `connector-patterns` skill and the registry switch; confirm `net/smtp` over the guarded dial; list the settings fields. *Accept:* a one-page note in this plan.
1. **Notifier** (`core/tools/notifiers/email.go`) with a fake SMTP server in tests. *Accept:* S1 (plain refused, bad certificate refused), S3 (no secret in logs or errors after a failed login), S4, S5 (CRLF in subject and name), S6 (limit and size), S7 (a host with no grant is refused before connecting).
2. **Registry and settings API**: new connector type, masked secret, step-up on change. *Accept:* the settings payload never holds the password; a save with the masked value keeps the stored one.
3. **Settings UI** (the Communication tab): one form, a Test button, a plain explanation of each field and of what the password is for. Frontend rule file first.
4. **Docs**: SPEC-009, the connector hub, an ADR if the connector contract changes, learnings.

## Risks

A provider that blocks app passwords or rate-limits a new sender (the owner sees the fixed error and the provider's limit is theirs). Sending through the owner's own account means the mail carries their identity: it only ever goes to their own address. Provider SPF and DKIM are the provider's.

## Remaining Work

All of it. First step: Phase 0.
