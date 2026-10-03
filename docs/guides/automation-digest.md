# Automation Digest — a recurring research run delivered to Telegram

Operator guide (non-normative; SPEC-007 §II.6 and SPEC-009 stay authoritative).

## What you get

A scheduled automation (for example the nightly AI-release brief) whose result is **pushed to your
chat**, skips items it already told you about, stays quiet when nothing is new, and tells you when
a run fails.

## 1. Connector

Create a Telegram connector in Settings → Communication (bot token from @BotFather, your `chat_id`).
Note its name — it is the `connector` value below. The inbound webhook (`scripts/telegram-bot.sh`)
is optional and only needed if you also want to chat with the agent or `/run <automation>` from
Telegram.

## 2. Automation

In the workspace `config.yaml` (or through the automations API):

```yaml
automations:
  - name: ai-release-brief
    trigger: {type: cron, value: "0 0 * * *"}
    task_file: llm_ai_release_brief.md
    strategy: isolated
    memory_mode: hot            # optional: inject your interest profile (below)
    notify:
      connector: my-telegram
      dedup: true               # skip links already reported (default window 60 days)
      send_empty: false         # stay silent when nothing is new
```

The task must produce a markdown **table with a real link per row** — dedup identifies an item by
its link. `backend/data/templates/llm_ai_release_brief.md` already does. Table rows are sent as a
bullet list because chat clients do not render tables.

## 3. Interest profile (optional, no code)

With `memory_mode: hot`, the workspace's operator notes (`MEMORY.md` in the workspace metadata
folder, edited in the Memory UI) are injected first in every run, ahead of saved facts. Put what you
care about and what to ignore there ("Care: open-weights releases, coding models, pricing changes.
Ignore: funding rounds under $100M, listicles"). The agent cannot rewrite these notes.

## Heartbeat: quiet checks that only ping you when it matters

A heartbeat is an ordinary automation that runs often and says nothing unless something clears
your importance bar. New workspaces ship a starter `heartbeat.md` with the contract:

- the agent replies exactly `HEARTBEAT_OK` when there is nothing to report — the dispatcher then
  shows and delivers nothing;
- otherwise it replies with a short alert and links, which is delivered through `notify`.

Set it up in **Automations → New**: task file `heartbeat.md`, a schedule of every 1–3 hours, a
connection, **Send results to** your connector with **Skip items already reported** on (so the same
alert never repeats), and **Skip a run if the model is busy** on.

Choosing the provider matters for a heartbeat:

- **A small cloud model** runs in the cloud lane: it never touches your GPU, never cold-starts the
  local model and is never preempted or queued behind your chats. Recommended for frequent checks.
- **A local model** wakes the model on every tick (load, short run, idle timer reset) and competes
  with your chats. If you use it, keep the interval long and leave **Skip a run if the model is
  busy** on: a tick that finds the lane busy is skipped instead of queued, and a heartbeat that you
  interrupt by chatting is dropped instead of restarted.

## Behaviour to know

- Delivery is best-effort: a Telegram outage is logged and the run still succeeds. The items are
  **not** marked as seen, so the next run offers them again.
- A run that fails sends one `⚠️ Automation <name> failed: …` line. A run you stop yourself does not.
- Long reports are split into several Telegram messages; if Telegram rejects the formatting the part
  is re-sent as plain text.
- The seen ledger lives under the workspace metadata folder (`seen/`); delete the file to forget
  everything reported so far.
- Connector changes made at runtime need a service restart to reach the dispatcher (same as the
  inbound webhook).
