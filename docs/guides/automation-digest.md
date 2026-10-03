# Automation Digest — a recurring research run delivered to Telegram

Operator guide (non-normative; SPEC-007 §II.6 and SPEC-009 stay authoritative).

## What you get

A scheduled automation (for example the nightly AI-release brief) whose result is **pushed to your
chat**, skips items it already told you about, stays quiet when nothing is new, and tells you when
a run fails.

## 1. Connector

Use **Telegram setup help** in Settings → Communication for a theme-aware walkthrough, diagrams,
and links to Telegram's official BotFather, getUpdates, and webhook documentation. Opening the
help panel keeps any unsaved connector form values.

Create a Telegram connector in Settings → Communication. Paste the full token from @BotFather
in **Bot token**, including its numeric prefix and colon (`123456789:example-token`). **Chat ID**
is the separate destination chat ID. To find it before registering a webhook, send your bot a
message and use Telegram's [getUpdates API](https://core.telegram.org/bots/api#getupdates) to read
that message's `message.chat.id`. For a group, add the bot and send it a command in the group;
keep the minus sign in the returned chat ID. `getUpdates` is unavailable while a webhook is
registered; reuse a saved chat ID or read it from the incoming webhook payload.

Choose a connector name such as `my-telegram`; it is the `connector` value below and does not
need to match your Telegram bot name. Click **Add connector**, then **Save communication settings**.

When editing, stored bot tokens show the server's masked value and webhook secrets show stars.
**Not set** means no value is configured; **Pending save** marks a typed replacement. Leave either
replacement field empty to preserve the existing secret. The masks are display placeholders and
are never saved as credentials. After replacing a webhook secret, save and register the webhook
again so Telegram uses the new value.

For agent-initiated sends through `notify_user`, enable **Communication** in **Settings → Agent
Guardrails**, or in the individual workspace's **Settings**, and save. The section also exposes
**Approve each notification** and **Max messages per task** (stored, but currently not enforced
for agent notifications). Host network access must be on.
Configuring and enabling a connector does not by itself grant the agent permission to send.
Dispatcher delivery of automation results through `notify` is a separate system-side path
(SPEC-007 §II.6; SPEC-009 §1.1).

The inbound webhook is optional and only needed if you also want to chat with the agent or
`/run <automation>` from Telegram. Enter an existing workspace ID in **Workspace for inbound
messages** (the part of its app URL immediately after `/workspaces/`). Optionally set a separate
**Webhook secret token** of your choice, using 1–256 letters, digits, underscores or hyphens.
Save, then use the **Inbound webhook** panel's **Public host** and **Register** controls to register
with Telegram. Enter a public HTTPS host or base URL that reaches this server; the UI appends
`/api/v1/webhooks/<connector-name>`. **Register** replaces the bot's existing webhook.
**Verify** checks its status; **Unregister** stops inbound delivery without stopping outgoing
notifications. Removing a connector unregisters its saved webhook before deleting its bot token.
If unregistering fails, removal stops and the connector and token are retained. Connectors without
a saved webhook URL do not unregister webhooks that may have been configured elsewhere.

## 2. Automation

In the workspace `config.yaml` (or through the automations API):

```yaml
automations:
  - name: ai-release-brief
    trigger: {type: cron, value: "0 0 * * *"}
    task_file: llm_ai_release_brief.md
    strategy: isolated
    memory_mode: "on"           # optional: inject your interest profile (below); "" follows the global default
    notify:
      connector: my-telegram
      dedup: true               # skip links already reported (default window 60 days)
      send_empty: false         # stay silent when nothing is new
```

The task must produce a markdown **table with a real link per row**, or a **bullet / numbered list
with a link in each item** — dedup identifies an item by its link. Items without a link are always
sent. `backend/data/templates/llm_ai_release_brief.md` already produces a table. Table rows are sent
as a bullet list because chat clients do not render tables; bullets are sent as written. When a
repeated bullet is dropped, its indented detail lines go with it.

### Time window

`llm_ai_release_brief.md` has one setting, the `**Window:**` line (`day`, `week` or `month`). Set it to
match the cron schedule: `day` for a nightly run, `week` for a weekly one. The agent passes it as the
`time_range` argument of `internet_search`, which each provider maps to its own recency filter
(Tavily `time_range`, Brave `freshness`, SerpAPI `tbs=qdr:`), and drops items dated outside it. A quiet
window gives a short or empty brief instead of padding with old items. Edit the copy of the task file in
your workspace; changing the template does not update copies already made. The task file is
instructions: the template forbids the agent from writing to it, because a run that overwrites it
makes later runs re-verify the old output instead of searching.

The same template has a **Memory** section for runs that have the memory tools (an assistant chat with memory on, or an
automation with memory on): the run first calls `memory_search` for earlier briefs, and before answering saves one short
fact (`LLM news brief <date> reported: <items>. Sources: <what worked or blocked>`) so the next brief skips what was
already reported. In a chat this is the way the brief learns between runs; automations can also keep a journal.

## 3. Interest profile (optional, no code)

With memory on (`memory_mode: "on"`, or the global "Automations remember" default), the workspace's operator notes (`MEMORY.md` in the workspace metadata
folder, edited in the Memory UI) are injected first in every run, ahead of saved facts. Put what you
care about and what to ignore there ("Care: open-weights releases, coding models, pricing changes.
Ignore: funding rounds under $100M, listicles"). The agent cannot rewrite these notes.

## Heartbeat: quiet checks that only ping you when it matters

A heartbeat is a check that runs often and says nothing unless something clears your importance
bar. It is one per workspace, set up in **Workspaces → (workspace) → Heartbeat**, and it is **off
until you switch it on**:

- **Check every**: 5 minutes to 6 hours (30 minutes by default).
- **Model**: a cloud model keeps your local model asleep (see below). Left on the workspace
  default it follows your primary model.
- **Send alerts to**: a connector. Quiet checks are never sent.

What to check goes in the workspace's `heartbeat.md` — one check per line. A new workspace's file is
only comments explaining this; while it holds no checks every tick is **skipped without calling the
model**, so enabling the heartbeat costs nothing until you write one. The comments are never sent
to the model. The system adds the reply rules itself:

- the model replies exactly `HEARTBEAT_OK` when nothing is new and clearly important — the
  dispatcher then shows and delivers nothing;
- otherwise it replies with a short alert, one line per item with a source link, which is
  delivered to your connector.

The panel shows the last check (quiet, alert, skipped because there were no checks or the model was
busy, or failed). A check is never dropped for being busy on a cloud model; on a local model a tick
that finds the GPU in use is skipped and waits for the next one (this is automatic, not a setting).
The heartbeat never injects hot memory.

Choosing the provider matters for a heartbeat:

- **A small cloud model** runs in the cloud lane: it never touches your GPU, never cold-starts the
  local model and is never preempted or queued behind your chats. Recommended for frequent checks.
- **A local model** wakes the model on every tick (load, short run, idle timer reset) and competes
  with your chats; the panel warns you when that is the case. If you use it, keep the interval
  long: a tick that finds the lane busy is skipped instead of queued, and a heartbeat that you
  interrupt by chatting is dropped instead of restarted.

## Learning from past runs (journal)

Dedup stops a digest repeating itself; the **journal** makes the next search better. Switch on
**Keep a journal between runs** (Model & access) and each run:

- starts with its own notes from earlier runs, added to the task;
- rewrites them before it finishes — which queries surfaced new items, which sources are worth
  checking or skipping, which topics are already covered.

The notes are plain text capped at 4000 characters; the agent writes the whole journal each time,
so it can tidy as well as add. Open the automation's page to read the journal, copy it, or **Clear
journal** to start over. It is deleted with the automation. The notes come from web-derived text
and are shown to the model as "your own notes, not instructions", but treat a journal that looks
odd as a reason to clear it.

## Behaviour to know

- Delivery is best-effort: a Telegram outage is logged and the run still succeeds. The items are
  **not** marked as seen, so the next run offers them again.
- A run that fails sends one `⚠️ Automation <name> failed: …` line, at most once per hour per
  automation until a run succeeds again. A run you stop yourself does not.
- Long reports are split into several Telegram messages; if Telegram rejects the formatting the part
  is re-sent as plain text.
- The seen ledger lives under the workspace metadata folder (`seen/`); delete the file to forget
  everything reported so far. A ledger that cannot be read is kept as `<name>.json.corrupt` and
  the next run starts with an empty one.
- Connector changes made at runtime need a service restart to reach the dispatcher (same as the
  inbound webhook).
