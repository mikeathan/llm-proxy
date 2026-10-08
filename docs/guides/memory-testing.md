# Memory — Verify It Works, and Measure Whether It Helps

Guidance, not the contract: behaviour is specified in [SPEC-004](../SPECS/memory.md) and
[SPEC-001](../SPECS/agent-loop.md); the plan is [small-context-memory](../PLANS/memory/small-context-memory.md).
**You run it, I read it** — nothing here starts, stops or restarts your server.

- **Part A (about a minute):** confirm memory is *stored* by the agent and *used* in a later run.
- **Part B (optional, long):** does hot memory, the progress ledger and the sieve fix help a small model?

The three prompts below ship as templates (`memory-store-test`, `memory-recall-test`, `memory-ab-test`) and
are the same text; a Go test (`template_store_test.go`) fails if guide and templates drift, so edit both. Part A
prompts work the same in an assistant chat and as an automation.

## Part A — Verify it works (two short runs)

The fact is made up, so run 2 can only answer correctly if run 1 really stored it and it came back through memory.
Run 2 forbids tools, so it cannot "search its way" to the answer: a correct answer can only come from the memory block
the system put in the prompt.

### Before you start

- Memory panel: delete any earlier test fact so the store run is not "already saved".
- **Automation only:** set the automation's **Memory** to **On** (Model & access), or turn on **Automations
  remember** in Settings → Local Engine → Memory. The shipped default is off, which injects nothing into an
  automation; a chat gets memory unless the workspace or Settings turns it off.

### Run 1 — store

New chat (or the `memory-store-test` automation), paste:

```markdown
TASK: Save exactly one fact to memory, then stop.

1. Call the memory_update tool once with these exact arguments:
   - scope: "workspace"
   - mode: "always"
   - keep: "permanent"
   - content: "The incident commander is Marcus Lindqvist and the audit window opens on 2032-07-09."
2. If the tool says the fact is saved (or already saved), reply with the single word STORED.
   If it returns an error, reply with the error text.
Do not call any other tool and do not write any other text.
```

**Pass:** the reply is `STORED`, and Memory → All shows the fact tagged **Always**. (Source `agent` in a chat,
`run:<id>` in an automation.)

### Run 2 — use

A **new** chat (so there is no history) or the `memory-recall-test` automation, paste:

```markdown
TASK: Answer three questions using ONLY the saved facts you were given at the start of this conversation
(the <memory> block in your instructions).
Do not call any tools: no memory_search, no commands, no file reads.
If a fact is not in your instructions, answer UNKNOWN. Never guess.

1. Who is the incident commander?
2. When does the audit window open?
3. Who leads the pager rotation?

Reply with exactly three lines and nothing else:
1: <answer>
2: <answer>
3: <answer>
```

**Pass**

```
1: Marcus Lindqvist
2: 2032-07-09
3: UNKNOWN
```

- Line 3 is the control: that fact was never saved, so `UNKNOWN` shows the model is not guessing.
- The model made **no tool calls**. A correct answer can then only come from the injected block.

### What to read in the files afterwards

| Check | Where |
|---|---|
| The first request's system message contains the fact inside `<memory>…</memory>` | `runs/<ws>/<model>/<conversation or automation>/<run>/recording.jsonl` |
| No `tool_call` events in run 2 | the same folder, `events.jsonl` |
| The fact reads "Sent in 1 run" (counters flush every 30 s) | Memory panel, or the `memories` table in `orchestrator.db` |

### If it fails

- **All three lines `UNKNOWN`:** the fact was not injected. Chat: check the fact is tagged Always. Automation:
  check Memory resolves to on (`memory_mode: on` in the workspace `config.yaml`, or the global automation default).
- **Lines 1 and 2 right but the model called a tool:** it ignored "no tools". The injection still worked
  (the block is in the request); try a larger model for this check.
- **Run 1 says "already saved" or an error:** delete the earlier test fact first, or read the error text.

### Clean up

Delete the test fact in the Memory panel. It is **Always**, so otherwise it rides in every prompt.

## Part B — Live A/B comparison on a small local model (optional)

Replays cannot show this: recorded responses ignore the prompt, and no existing run ever pruned.
Only a live run on a small window answers "does it repeat fewer calls?".

### B1. Make the window really small

The point is to make the sieve fire, so the model must be served with a small context.

1. Serve one local model with `--ctx-size 8192` (add it to that model's llama-server arguments, wherever you configure local models).
2. Find the model's context budget (the Models/Settings page shows `context_budget`) and confirm it: for 8192 it is `(8192 − 2730) × 4 = 21,848` chars
   (SPEC-005 §II.3). That number is what memory and the sieve size themselves from.
3. **Check for a budget override.** For a **cloud** workload `settings.yml → model_overrides.<model>.context_budget` beats the
   derived value. Some entries carry `context_budget: 50000`; with that, an 8K window never prunes and
   the test measures nothing. Remove it (or set it to the real figure) for the model under test. A **local**
   workload ignores the override: its `context_budget` always comes from the serving context. That includes a
   llama.cpp server behind an OpenAI-style URL, which is local once its own `/v1/models` entry identifies it
   (SPEC-005 1.3); check `process.log` for "treating it as a local workload".
4. The server must run with `--enable-runs` (or `--record`), otherwise no `events.jsonl` /
   `recording.jsonl` are written and the scoreboard has nothing to read.

### B2. The task prompt

Do **not** use the smoke test for this. It tells the agent to *run* each command, and small models obey an
explicit "run X" over anything in memory (`docs/audits/memory-injection-investigation.md`, 8+ attempts), so it
would show no difference. This prompt asks for **goals**, not commands, and it deliberately produces enough
tool output to fill an 8K window and make the sieve fire.

It ships as the template `memory-ab-test.md` (create the task file from the template library, or save the text
below in the workspace as `memory-ab-test.md`) and is used for **both** automations, byte-identical. The template
and this text are the same prompt; if you edit one, edit the other:

```markdown
TASK: You are an autonomous agent working in this workspace. Complete every part below, then write the
final report as your reply.

Rules: be concise and call tools directly. Do not repeat a step you have already completed. If something
fails, note it and move on.

## Part A — Environment
1. Find out which Node.js version and which npm version are installed on this machine.
2. Find out which operating system and kernel this machine runs.

## Part B — Work files
For each N from 1 to 8:
- Use the write_file tool to create `scratch/part-N.txt` with 40 lines. Line K (K = 1 to 40) must read
  exactly: `line K: the quick brown fox jumps over the lazy dog`
- Use the read_file tool to read `scratch/part-N.txt` back, and note how many lines it has.
Each file needs its own write_file call and its own read_file call (no shell loops, no several files in one
command). You may send several of these calls in the same response.

## Part C — Final report
Reply with a report containing exactly:
1. The Node.js version and the npm version.
2. The operating system and kernel.
3. A table of the 8 files you created and the line count of each.
4. One line: DONE.
```

Why it is shaped this way:

- **Part A** is answerable from memory (the facts you seed below) *or* by running a command, so a
  difference between the two automations is attributable to memory.
- **Part B** forces a separate `write_file` and `read_file` per file, because a first live attempt showed a
  model will otherwise do all the files in two shell loops and never fill the window. It deliberately does
  **not** say "one at a time": the agent's own system rules say "batch related tool calls into a single response",
  and a prompt that contradicts them made a model argue with itself for about 70 seconds until the stuck
  detector fired (observed, 2026-10-01). Each file is about 1,800 characters written plus about 1,800 read back
  (tool arguments and results both count): roughly 3.6K per file, about 29K for eight, against the
  21,848-char budget. By my rough estimate the first prune lands around the fifth file, but that is an
  estimate — the `sieve` column is the evidence. Keep the volume modest: the model has to *generate* every
  written character, so run time is dominated by output speed (a remote 35B at ~30 tokens/s took ~7 minutes
  with the larger first draft).
- **Part C** needs things that were done earlier. A model that kept the progress ledger can answer
  without redoing work; one that lost track will re-run commands or re-create files, which is exactly
  what the `repeats` / `after_sieve` columns count.

Grade each run pass/fail by hand against the rubric, because the scoreboard cannot:

- the report states the same Node, npm and OS as the real machine (not invented);
- the table lists all 8 files with the correct count (40);
- it ends with `DONE`;
- you see no `scratch/part-N.txt` written twice with different content;
- the run used one tool call per file. If it batched files into a shell loop, the window never filled: discard
  that run (the `sieve` column will say `0`).

Report pass/fail per run next to the scoreboard row. A run that finishes correctly in fewer steps is the win;
a run that repeats work or stops early is the loss.

### B3. Seed memory the task would otherwise rediscover

In Workspace → Memory → **Add memory** add these short facts, each *Used: In every run*. First run the
commands once yourself to get the real values:

- `Installed: Node <output of node --version>, npm <output of npm --version>.`
- `This machine runs <output of uname -sr>.`
- `Files with N numbered lines are made with: seq -f "line %g" 1 N > <file>` (a procedure the agent would otherwise work out)

Use the values your machine actually reports, or the model will be "helped" with wrong facts and the
comparison is meaningless.

Then **Show what the model receives**, pick the model, and check the block is there and under the
model's budget. (Optional: put a one-line standing rule in **Operator notes**.)

### B4. Two automations, same model, same task

Create two automations in the same workspace (Automations → New). Everything identical except memory:

| Name | Model | Task file | Memory |
|---|---|---|---|
| `memory-ab-off` | the small-context local model | `memory-ab-test.md` | Off |
| `memory-ab-hot` | the same model | `memory-ab-test.md` | On |

Run order matters little, but keep the machine otherwise idle: the local lane runs one at a time.

### B5. Run each three times

Local models vary run to run, so one run proves nothing. Trigger `memory-ab-off` three times, then
`memory-ab-hot` three times. Wait for each to finish before the next.

### B6. Read the scoreboard

Runs land in `~/.config/llm-proxy/runs/<workspace>/<model>/<automation>/<run>/`.

```bash
scripts/memory-scoreboard.sh ~/.config/llm-proxy/runs/<ws>/"<model>"/memory-ab-off/*
scripts/memory-scoreboard.sh ~/.config/llm-proxy/runs/<ws>/"<model>"/memory-ab-hot/*
```

| Column | What to expect if it works |
|---|---|
| `mem_req` | `memory-ab-off`: `0/N`. `memory-ab-hot`: `N/N` — memory in **every** request, not only the first |
| `mem_tok` | roughly the preview's tokens (a few hundred), constant |
| `sieve` | > 0 on both if the window is really small (derived: the prompt shrank). If `0/0` the window is too big — recheck step 1 |
| `repeats` | lower on `memory-ab-hot` (it already knows the versions and the file recipe) |
| `after_sieve` | the ledger's target: should be 0 or near it on `memory-ab-hot` |
| `turn1_s` | within about ±5% between the two (memory must not slow prefill noticeably) |
| `steps` | fewer or equal on `memory-ab-hot` |

`sieve` is derived (no sieve event is recorded) and `after_sieve` is approximate; `n/a` means the
run files did not contain what the column needs.

Cross-check in the UI: each seeded fact should now read "Sent in 3 runs" after the three hot runs
(counts are flushed every 30 s and on shutdown), and the Unused filter should no longer list them.

### B7. What a good and a bad result mean

- **`mem_req` is `1/N` on the hot runs** — memory is only reaching the first request. That is the
  original defect; it should not happen now. Keep the `recording.jsonl` and tell me.
- **`sieve` 0 everywhere** — the window never filled; the comparison says nothing about the ledger.
- **Repeats equal, steps equal** — hot memory did not change behaviour on this task. That is a valid
  negative result: the plan says to ship only the opt-in switch and record it, not to widen it.
- **`turn1_s` clearly higher on hot** — the memory block is costing prefill time; lower the share
  (`hotMemoryShareLocal`) or shorten the facts.

Paste both tables back and I will fold them into
[the platform scan audit](../audits/2026-09-30-platform-scan.md) as the memory-enabled baseline.

### B8. Optional: an assistant-chat baseline

Same idea without automations: in the Assistant, ask the same three questions in a fresh
conversation with the facts hot, then again with the facts toggled off (Memory → fact → switch off).
Read the two run directories under `runs/<ws>/<model>/conv_*/` the same way.

## Part D — Evaluating the save guidance (run on each model you use)

The assistant's head prompt now tells the model when to call `memory_update` (SPEC-004 §II.8). It can only be judged on a
real model, because small models often ignore general advice. Use a **fresh conversation per script**, assistant memory on,
and read each turn's tool steps and the Memory panel afterwards. Do this on your small-context local model (the 16K Qwen is the reference) and on one cloud model.

| # | Say this | Expect |
|---|---|---|
| D1 | `We always deploy through the vertex host, and I prefer tabs in Go files. What does gofmt do?` | the answer is a normal explanation of gofmt; one or two sensible saves (deploy host, tabs), made **before** the answer; the answer is the last message |
| D2 | the news brief (paste `llm_ai_release_brief.md` and run it) | **no** saves: it is a task, not a preference |
| D3 | `My password is hunter2. What is 2+2?` | no save of the password (guardrail or the model declines); a normal answer |
| D4 | send D1's first sentence again in the same chat | "already saved" and **no** repeated calls; the run completes |
| D5 | `Actually we deploy through the new host, not vertex.` after D1 | the old fact is replaced (`old_text`) or a new one is added; note which |

Record per model: saves that should have happened, saves that should not have (D2/D3 must be zero), whether the final
answer is intact and last, and whether every run completed. **Pass** = no unwanted saves and no failed runs on both models.
If a model over-saves or loses its answer, remove the guidance (one constant, `memorySystemText`) and keep Parts A and C.

## Part C — Saving from chats (explicit capture and review)

Needs a chat in the operator UI (not a connector chat) with assistant memory on (Settings → Local Engine → Memory, and the
workspace's Memory section not set to Off). Works the same on any model; capture involves no model call.

### C1. Explicit capture (a few seconds)

1. In a new conversation send: `Remember that the staging DB runs on port 5433. From now on answer in short sentences.`
2. Under the answer the turn shows **Saved to memory** with two lines. Open Memory: the first is an ordinary on-demand
   fact, the second is tagged Always; both have source `capture`. Reload the page: the lines are still on the turn.
3. The same run already followed the standing instruction (it was in that run's memory block).
4. Things that must **not** save, one message each: `Do you remember the port?`, `Never mind, I'll do it myself.`,
   `Remember the password is hunter2`, `Always use tabs.` (a plain imperative is left to the review), a code block containing
   `remember that …`, and a pasted playbook or any long, heading/list/table-shaped text. Nothing appears under the answer.
5. Send the first message again: nothing new is announced (already saved). In a connector (Telegram) chat the same text saves nothing.

### C2. Review this chat for memories

1. After a chat that stated a preference or project convention, press **Review for memories** in the chat header.
2. A dialog lists at most five proposed facts (already-saved ones are marked). Untick what you do not want, press
   **Save selected**; the facts appear in Memory with source `operator`. Nothing is saved before that press.
3. A chat with nothing worth keeping (a pure research task) should give "No suggestions"; if a small model proposes noise, you
   simply save nothing. If the model is busy you are told to try again; the chat itself is never affected.
