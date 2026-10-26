---
subsystem: agent-loop
tags: [delegation, research]
result: reference
status: reference
date: 2026-10-22
---

# Multi-Agent Workflows — research for a possible plan (2026-10-22)

**Status:** reference. Research for discussion only. No code was changed and no plan was written.

**Request (user's words):** "multi agent workflow … different types of agents that can work with each other depending
on the user request … investigate what hermes agent does … check what claude code does, what is the best practises …
what the benefits would be, some case studies … and how to keep it generic and configurable."

## Evidence labels

- **[code]**: opened in this repo or in `~/dev/hermes-agent` (HEAD `22063fc099`, 2026-10-17) during this research.
- **[doc]**: read in that project's own documentation.
- **[web]**: fetched from the published source linked in that section.
- **[assumption]**: my inference. Not verified.

---

## 1. Open plans in this repo

The live tracker is `docs/PLANS/README.md` → "Remaining Work". Summary:

| Status | Plans |
|---|---|
| active (7) | Prompt Profiles (eval findings + §9 items), GPU Performance, Connector Auto-Reply (Phase 3), Post-Implementation Cleanup, Search Tool Calling (manual Brave/SerpAPI check), Local/Cloud Inference Performance, Memory Gaps (P5 typed facts → P6 consolidation) |
| partial (8) | Automation Learning Journal (live checks), Frontend Redesign v2 (phone views, focus traps), CI & Versioning (P2/P3/P6), Agent Improvements (P4, P5, **P7 sub-agent wrap**), Memory Improvements (session search, skills), Liveness Heartbeat package split, Large Result Storage, Agenda — Reminders and Follow-ups |
| proposed (22) | AGENTS.md Layering, Tool-Call Grammar, Assistant Conversation Package, SQLite Sessions, Admin/API Auth, Sandbox Residuals (P2/P3), Heartbeat Repeat Suppression, Backend Hot Paths, Workbench Layout, Autonomy Roadmap + UX, Attention Policy, Cross-Channel + Remote Approvals, **Standing Goals**, **Event-Driven Triggers**, Memory Consolidation, Chat → Automation, **Cost Ledger**, **Auto Model Router**, Cloud Privacy Firewall, Playbook Library, Cloud Safety Thresholds |
| complete, pending Linux CI | Agent OS Sandboxing |

The plans in bold overlap with multi-agent work (§6).

Two existing places already reserve this idea:

- `docs/SPECS/agent-loop-strategies.md` §VIII lists "map-reduce fan-out … orchestrator-workers, Mixture-of-Agents" as
  out of scope, with this note: "Each is a follow-up plan" **[code]**. A multi-agent plan would be that follow-up.
- `docs/PLANS/agent-loop/agent-improvements.md:281-330` sketches Phase 7 "sub-agent auto-wrap", which wraps an
  `*Agent` as a tool in `NewLocalToolRegistry`. The plan skips it as "Not needed until sub-agents exist" **[code]**. A
  new plan should absorb that item, not sit beside it.

## 2. What Hermes does

Hermes has **two separate tiers**. Its docs explicitly say the two are different tools for different jobs.

### 2.1 `delegate_task`: in-run sub-agents (fork → join)

Sources: `website/docs/user-guide/features/delegation.md` **[doc]**, `tools/delegate_tool*.py` (4,464 lines) **[code]**.

- **Shape.** The parent calls one tool, giving a `goal` and a `context`, or a `tasks[]` batch. Each child is a fresh
  agent with **no knowledge of the parent's conversation**: "Subagents Know Nothing". It does get the workspace's
  AGENTS.md/CLAUDE.md. Only the child's final summary enters the parent's context.
- **Structured output.** An optional `output_schema` per task. The parent validates the result and gives the child
  exactly one correction turn. If the result still fails, the child's raw text is kept and marked `schema_valid: false`.
  The work is never discarded.
- **Blocked for children** (`delegate_tool_toolsets.py:14`) **[code]**: `delegate_task` (no recursion), `clarify` (no
  user interaction), `memory` (no shared-memory writes), `send_message` (no outbound side effects), `cronjob_manage` (no
  scheduling in the parent's name).
- **Children inherit the parent's toolsets.** The model cannot widen them per call.
- **Defaults** (`delegate_tool_config.py`) **[code]**:
  - `MAX_DEPTH = 1`, flat. A child can spawn its own children only with `role="orchestrator"` and a higher
    `max_spawn_depth`.
  - `max_concurrent_children` is 10.
  - `max_iterations` is 250 per child **[doc]**.
- **Model per child.** One global `delegation.model`/`provider` sets it, and there is no per-task model. The docs
  recommend a "frontier planner, inexpensive workers" split, because "the children are where the tokens go".
- **Lessons Hermes recorded** **[doc]**:
  - **No wall-clock timeout.** Hard caps of 300 s, then 600 s, "kept killing legitimately busy children". Hermes
    replaced them with a progress-based stall monitor: 450 s idle, 1,200 s inside a tool. A model wait counts as
    progress.
  - **Cap one-shot runs.** `oneshot_max_children` defaults to 2, because "in benchmark trajectories most of its spawns
    were 'independently review my own work' rather than parallel work". Each such child re-pays a cold system prompt.
  - **Ownership.** A child's background processes die with the child unless it hands them off explicitly.
  - **Visibility.** Children write live transcript files, and failures always reach the parent as
    `status: "failed"` plus a one-line reason.
  - **Steer and stop** individual children.
  - **Worktree isolation** for parallel children that edit the same repo is opt-in.

### 2.2 Kanban: a durable board of named agents (work queue)

Source: `website/docs/user-guide/features/kanban.md` **[doc]**.

- **Shape.**
  - Tasks are rows in SQLite, each assigned to a named **profile**. A profile is a full agent home with its own config,
    model, memory and skills.
  - Parent → child links gate the work: a dispatcher promotes a task to `ready` when all its parents are `done`.
  - Comments are the inter-agent protocol.
  - A parent's completion summary and metadata are injected into the child's context.
- **What it covers that `delegate_task` cannot:** survives restarts, human-in-the-loop (block → comment → unblock),
  retry or review by a different role, an audit trail, and a per-task model.
- **Patterns it names:** fan-out, pipeline (scout → editor → writer), voting + aggregator, long-running journal,
  human-in-the-loop, fleet work (one role across N subjects), and a triage specifier.
- **Orchestrator rules worth copying:**
  - "A well-behaved orchestrator does not do the work itself."
  - The orchestrator checks which profiles exist before assigning work to them.
  - **"Decide before you fan out"**: shared decisions such as a schema or file format are made once and written into
    every child task, because "workers cannot see sibling cards".
- **Cost.** It is large: a dispatcher, OS-process workers, a dashboard and CI-gated completion contracts.

## 3. What Claude Code does

Source: https://code.claude.com/docs/en/sub-agents **[web]**, plus the `Agent` tool contract in this session.

- **An agent type is a Markdown file with YAML frontmatter.**
  - Precedence runs managed → CLI flag → project `.claude/agents/` → user `~/.claude/agents/` → plugin.
  - Only `name` and `description` are required.
  - Optional fields include `tools` / `disallowedTools`, `model` (or `inherit`), `permissionMode`, `maxTurns`,
    `skills`, `mcpServers`, `hooks`, `memory`, `background`, `effort`, and `isolation: worktree`.
- **Routing is by `description`.** "Claude delegates based on the request, each subagent's `description`, and current
  context." The user can also name or @-mention an agent explicitly.
- **Context isolation.** Each sub-agent has its own context window and system prompt. It does not see the conversation
  history; it receives the delegation message plus CLAUDE.md. "Only the final summary returns to the main conversation,
  which keeps verbose output such as test logs or search results out of your main context."
- **Some tools are always withheld**, for example asking the user a question.
- **Model resolution:** per-call → frontmatter → environment variable → parent model.
- **Built-ins:**
  - Explore: read-only search.
  - Plan: read-only research.
  - general-purpose.
- **Background and parallel runs** are supported (up to 20 by default). `SendMessage` resumes a finished sub-agent
  with its history. Nesting is allowed up to 3 levels and can be turned off.
- **Its own guidance:**
  - Least privilege.
  - Route cheap work to faster models.
  - "Isolate high-volume output".
  - "Don't use subagents for iterative, back-and-forth work or for quick targeted changes, since a non-fork subagent
    starts without your context and has startup latency."
- **Agent teams** are a separate, heavier mechanism: coordinated sessions that message each other. This is the
  Claude Code counterpart of Hermes's second tier.

## 4. Best practice and case studies (external)

**Anthropic, "Building effective agents"** https://www.anthropic.com/engineering/building-effective-agents **[web]**

- Five workflow patterns:
  - prompt chaining
  - **routing** ("classifies an input and sends it to a specialized follow-up task")
  - parallelization (sectioning or voting)
  - **orchestrator-workers** ("when you can't predict the subtasks in advance")
  - evaluator-optimizer
- "Find the simplest solution possible, and only increase complexity when needed"; add it "only when it demonstrably
  improves outcomes".
- On frameworks: start with the API directly.

**Anthropic, multi-agent research system** https://www.anthropic.com/engineering/multi-agent-research-system **[web]**

- Results:
  - An Opus lead with Sonnet sub-agents "outperformed single-agent Claude Opus 4 by 90.2%" on their research eval.
  - Cost: "multi-agent systems use about 15× more tokens than chats", against about 4× for a single agent.
  - "Token usage by itself explains 80% of the variance."
  - Parallelism "cut research time by up to 90% for complex queries".
- Fits research: breadth-first, parallel, more information than one context window holds. Fits coding poorly: coding
  has "fewer truly parallelizable tasks than research", and agents share too much context.
- Every delegated task needs "an objective, an output format, guidance on the tools and sources to use, and clear task
  boundaries". Vague tasks led to duplicated searches.
- Scale effort to the query: a simple fact needs 1 agent and 3–10 tool calls, a comparison 2–4 sub-agents, complex
  research 10 or more.
- Failures they saw:
  - "spawning 50 subagents for simple queries"
  - endless searching
  - agents distracting each other
- Evaluation: about 20 real queries and one LLM-judge call per answer.

**Cognition, "Don't build multi-agents"** https://cognition.com/blog/dont-build-multi-agents **[web]**

The post is from June 2025. It describes an older Claude Code: today's sub-agents (§3) run in the background, in
parallel and nested.

- "Share context, and share full agent traces, not just individual messages."
- "Actions carry implicit decisions, and conflicting decisions carry bad results."
- Flappy Bird example: two parallel workers built a Mario-style background and a mismatched bird.
- Recommendation: a single-threaded agent plus history compression. Sub-agents are acceptable when they answer a
  well-defined question, do not write in parallel, and keep investigation out of the main history.

**Cemri et al., "Why Do Multi-Agent LLM Systems Fail?"** (NeurIPS 2025) https://arxiv.org/abs/2503.13657 **[web]**

- 14 failure modes in three groups:
  - specification / system design
  - inter-agent misalignment (withholding information, ignoring input, resetting the conversation)
  - task verification (premature termination, missing or incorrect verification)
- Their finding is that failures come mostly from **system design**, not from model limits.

**Where the sources agree:**

1. Delegate questions, not decisions. Make shared decisions before fanning out.
2. A child starts cold, so its task description is the whole contract.
3. Return a short summary and keep the bulk out of the parent.
4. Least-privilege tools, and no side effects from children.
5. Flat by default.
6. Expect large token multiples, and measure before claiming a win.
7. Research and breadth-first reading benefit; tightly coupled writing does not.

## 5. Fit to this repo: the constraint first

### 5.1 The local lane admits one run, so a naive child deadlocks **[code]**

- `settings.yml → scheduler.local_concurrency` defaults to 1 (`docs/SPECS/automation-dispatcher.md` §V).
- `runlane.Scheduler.ClaimInteractive` (`backend/internal/core/runlane/scheduler.go:117`) queues the caller until a
  slot frees. Before it waits, it calls `preemptForLocked` → `preemptOneLocked` (`runlane/lane.go:228-247`), which
  cancels a running **automation** in the same lane when `preempt_automations` is on (the default). A child that
  claimed the local lane would therefore:
  - under a **chat** parent: wait until the parent's context ends, so the run hangs until the user cancels it;
  - under an **automation** parent: preempt and cancel its own parent, and with it itself, since the child's context
    derives from the parent's.
- The same holds for the model gate. `WaitForModel` (`runlane/modelgate.go:135`) blocks a different local model while
  a live run holds the current one.

So a child on the local model **must run inside the parent's slot**, as a nested run in the same process. It must not
be admitted as a new run. A cloud child can claim the cloud lane (3 slots by default), but that lane can be full, so the
wait needs a bound.

### 5.2 What the benefit honestly is here

- **Parallelism exists only on the cloud lane.** On one GPU, local children run one after another **[code: limit 1]**.
  The benefit there is not speed.
- **Context isolation is the main benefit for small local models [assumption, to measure].** Two real runs fit this:
  - **The vertex 16k Qwen overflow.** JSON search results measure about 2.76 characters per token, so tool results
    fill the 16k window far sooner than the 4.0 estimate assumed (`docs/PLANS/ARCHIVE/agent-loop/request-size-estimator.md`
    §2, row 2) **[doc]**.
  - **The 141 s weather run.** A search loop, then a history rewrite that broke the cache, followed by a 41 s step
    (`docs/PLANS/agent-loop/run-efficiency-and-convergence.md` E1/E5) **[doc]**. Isolation would have kept the loop's
    results out of the parent, but it does not stop the loop. Live reruns of that question stayed at 5–7k tokens, so
    the window was not the cause (memory note, 2026-10-18).

  In the first case, a "search and read" child that returns a few hundred tokens would have kept the bulk out of the
  parent's window. This is a hypothesis to test, not a measured win.
- **Least privilege.** A child built from a narrow profile exposes fewer tool schemas, which matters at 16k: the ten
  tool manifests were about 3.8k tokens before the profiles work (`prompt-profiles.md` finding 3) **[doc]**. The
  profiles eval later cut the system prompt by about 27% (`docs/audits/2026-10-17-prompt-profiles-eval.md`).
- **Mixed local/cloud routing.**
  - A local planner can send one hard sub-question to a cloud model.
  - A cloud planner can keep private file reading on the local model.

  This overlaps the Auto Model Router and the Cloud Privacy Firewall.
- **Cost.**
  - Each child pays a cold system prompt.
  - A different profile text changes the system prompt, so the llama.cpp prefix cache is reused only up to the shared
    core rules **[assumption]**. The 2026-10-09 audit measured what cache loss costs: re-prefill took 17.3 s before its
    fix and 0.8 s after.
  - Chat preemption cancels a running automation in the same lane, taking any children with it **[code: SPEC-007 §V]**.

### 5.3 Parent/child lane combinations (added after discussion, 2026-10-22)

User's suggestion: "offer that for cloud providers only? or parent could be local but sub agents cloud? since we only
support 1 model per run in local".

The "one local model per run" part is right **[code]**. `checkSwitchLocked` (`runlane/modelgate.go:107`) refuses to
switch away from a model another key is using. A child would share the parent's key, so it would not be refused: it
would swap the model out from under its own parent.

| Parent → children | Deadlock | Parallel | Catch |
|---|---|---|---|
| local → local | Yes, if the child claims a slot; avoided only by running in the parent's slot | No | Same model only (a swap means a cold reload); each child's profile prompt breaks the cache |
| local → cloud | No (different lanes) | Up to `cloud_concurrency` (3) | The parent's local slot sits idle while it waits, so local automations queue behind it; context the parent read goes to the cloud |
| cloud → cloud | **Possible:** 3 cloud parents fill the 3 slots and their children wait forever | Yes | Children need their own budget or a bounded wait that fails cleanly |
| cloud → local | No | No | A privacy-friendly reader (only the summary leaves the machine); a child's claim pre-empts a running automation |

**Recommendation [assumption]:** children never run on the parent's local model.

1. Ship local → cloud and cloud → cloud first.
   - Local → cloud fits the 16k problem best: a local parent sends heavy reading to a large-window cloud model and gets
     back a short summary.
   - Cloud children get their own concurrency cap, separate from the lane slots.
   - The context sent is only what the parent writes in the call, never attached automatically. This is the first step
     toward the Cloud Privacy Firewall.
2. Later: cloud → local as an opt-in "private reader" type.
3. Local → local only if a measurement shows that in-slot isolation helps a 16k model.

### 5.4 What already exists to build on **[code]**

| Need | Existing piece |
|---|---|
| "Agent type" | **Prompt profiles** (`backend/internal/core/assistant/profile/`): built-ins `general`, `research`, `operator`, `builder`, `watcher`, `task-runner`. The `Profile` struct already holds `Tools` categories, `MaxSteps`, `MayAsk`, `AnswerFirst`, `Memory`/`MemorySave` and `Reminders`. User profiles are `.md` files in the config directory, read on demand, so **users can already define new types without code**. |
| Per-agent model | `models.GetModelOverride(ctx)` (`conversation_service.go:185`). `Profile` has no `model` field yet. |
| Tool scoping | `toolpolicy.ResolveForScope`, `AllowedTools`/`ExcludedTools`, tool categories (`agent_events.go:155`). |
| Loop patterns inside one agent | `LoopStrategy` registry: `react`, `plan_execute`, `evaluator_optimizer` (`loop_strategy.go`). |
| Durable multi-step pipelines | Automations + dispatcher. The proposed `after` run-chaining trigger in `autonomy/event-driven-triggers.md`. |
| Long-running goals | `autonomy/standing-goals.md` (proposed). |

## 6. A generic, configurable shape (for discussion, not a design)

Copy Hermes's two-tier split, but build each tier from what exists. Do not add a parallel mechanism.

**Tier 1: in-run delegation (one tool).**

- **Agent type = a profile**, plus two optional new fields:
  - `model`: which model it runs on.
  - `delegate_summary`: the one line the parent reads to choose it, like Claude Code's `description`.

  Built-in types ship as embedded profiles. User types are files in the config directory, as today.
- **One tool**, for example `delegate(agent, goal, context, output_schema?)`:
  - The child runs as a nested run session inside the parent's lane slot.
  - It returns a bounded summary (or schema-checked JSON).
  - Its steps appear nested in the parent's stream and run record.
  - The child's context derives from the parent's, so cancelling the parent cancels the child.
- **Policy, fixed in code** (Hermes's and Claude Code's common floor):
  - Depth 1.
  - Children per run capped.
  - The child's tools are the parent's tools intersected with the profile's tools.
  - Children never get memory writes, connectors/`notify_user`, reminders, automation creation, clarifying questions,
    or further delegation.
  - A progress-based stall bound, not a wall clock.
  - Tokens are counted against the parent run, which ties into the Cost Ledger and the Attention Policy spend cap.
- **Routing.** Who picks the agent type?
  1. The user, explicitly: "/ask research …" or a picker.
  2. The parent model, from the type summaries.
  3. A cheap classifier up front, which is "routing" in Anthropic's terms.

  Option 1 is the safest start. Option 2 is what Hermes and Claude Code do.

**Tier 2: durable pipelines (no new board).**

- Pipeline = automations chained with the proposed `after` trigger. Each automation already names its own profile,
  model and tools. The parent's final report becomes the next run's input, as Hermes does with parent results.
- Human-in-the-loop through the existing approval and remote-approval plans. A Kanban board is not justified at this
  product's size **[assumption]**.

**Overlapping plans to reconcile:** Agent Improvements P7 (absorb it), Agent Loop Strategies §VIII (this would be the
follow-up), Event-Driven Triggers (`after`), Standing Goals (goal ticks could delegate), Auto Model Router (per-child
model choice), Cost Ledger plus Attention Policy (spend cap), Cloud Privacy Firewall (what may go to a cloud child).

## 7. Candidate first use cases

1. **Research fan-out** ("compare X, Y and Z", the news automation). This is the strongest external evidence. The
   children are parallel on a cloud model and run one after another locally. Each child is the `research` profile,
   read-only, returning a summary with links.
2. **Context-isolating reader for small local models.** The parent delegates "find out what these files or pages say
   about Q" and gets 300 tokens back instead of 8k. The evidence is this repo's own 16k failures. It needs a
   before/after eval, in the style of the prompt-profiles eval.
3. **Fresh-context reviewer** (draft → independent check). Lower priority: Hermes found self-review spawns were the
   most common and the least useful in one-shot runs, and `evaluator_optimizer` already covers part of it in-run.

## 8. Decisions for the user

- **D1: first use case.** Research fan-out, the local context-isolating reader, or both on one tool?
- **D2: where children run.** §5.3 recommends per-type models restricted to cloud for now (local → cloud,
  cloud → cloud), with cloud → local as a later opt-in. Agree?
- **D2a: cloud child concurrency.** A separate child pool, or the shared cloud lane with a bounded wait? The shared
  lane can deadlock without the bound.
  - **Recommendation: a separate pool.**
    - **No deadlock by design.** The parent's lane slot covers its whole run. Sub-agents use the pool and never hold a
      lane slot, and depth is 1.
    - **The shared lane only turns the deadlock into a timeout.** It also caps fan-out below what was asked: a parent
      holding 1 of 3 slots gets at most 2 sub-agents.
    - **Neither way into the lane fits a sub-agent.** `ClaimInteractive` pre-empts cloud automations; `Submit` queues
      behind them in arrival order.
    - **Cost:** cloud requests in flight can exceed `cloud_concurrency`. Bound it with a global sub-agent cap plus a
      per-run cap, or derive the pool from the lane limit.
    - **When the pool is full:** a short wait, or an immediate "busy" result the parent can act on.
    - **Unverified:** how the backend handles a provider's 429.
- **D3: who picks the agent type.** The user explicitly, or the parent model from type summaries, or a classifier?
- **D4: tiers.** Tier 1 only for now, or also plan Tier 2 (`after` chaining) together with Event-Driven Triggers?
- **D5: limits.** Depth (1 recommended), children per run, and whether a token cap per run is required before shipping.
- **D6: approvals.** A child's terminal or network call that needs approval: surface it in the parent's approval UI, or
  deny it in children?
- **D7: success measure.** What eval proves it helped? For example, the weather and news runs: wall time, peak context,
  answer quality judged by one LLM call.

## 9. Decisions taken in discussion (2026-10-22)

The user agreed to the recommendations below.

- **D1:** web research fan-out comes first. A local-file reader comes later, as cloud → local.
- **D2/D2a:** each agent type names its own model, cloud only in v1, from a separate pool (global cap 3, 3 per turn).
- **D3:** a profile lists the types it may delegate to, and the parent model decides when. The `general` profile does
  not delegate (user: agreed).
- **D4:** in-run delegation only. The pipeline tier waits for Event-Driven Triggers.
- **D5:**
  - depth 1;
  - at most 15 steps per sub-agent;
  - a returned summary of at most ~1.5k tokens;
  - a token budget recorded per sub-agent;
  - stuck detection by lack of progress, not a fixed time limit.
- **D6:** sub-agents get only tools that never need approval. In v1 that means web search and page fetch, with **no
  file access** (user: agreed). An approval-gated call is refused at once with a result the parent can act on.
- **D7:** a before/after eval on 10–15 fixed research prompts. The parent is local Qwen 16k, with N repeats per build.
  Measures: LLM-judge quality, peak parent context, overflow and loop errors, wall time, and tokens/cost.

**Several local agents on one GPU [doc + assumption].**
- The documented server runs `--parallel 1` on a Radeon 780M iGPU (`.agents/skills/llamacpp-setup/SKILL.md:22-32`).
  llama.cpp splits `--ctx-size` across its parallel slots, and token generation shares the same memory bandwidth.
  Two or three local agents would each get a fraction of the window and run slower.
- Local → cloud stays the v1 shape.

**Dev work.** It stays a single agent: the `builder` profile in its own workspace. It may delegate look-ups such as
docs or APIs to cloud researchers, but never parallel writers (Cognition; Anthropic: coding "has fewer truly
parallelizable tasks").

**Agent definitions and the editor (user: agreed, 2026-10-22).**
- **Local read access is a per-profile permission.**
  - `files: none | read` controls whether the agent may read workspace files.
  - `cloud_files: allow` is also required when the profile's model is a cloud model.
  - Reads stay inside the workspace jail with the existing caps, and each file sent to a cloud model is logged.
  - Helpers never get write tools.
  - This replaces the "explicit attachments" idea.
- **Helpers are declared on the lead's profile**, for example
  `helpers: [{agent: web-researcher, when: on_demand}, {agent: reviewer, when: on_finish}]`.
  - `on_demand`: the model calls the helper.
  - `on_finish`: runs once, before the final answer.
  - `first`: maybe, once at the start.
  - A separate "team" concept waits until profile fields prove too narrow.
  - A free-form workflow canvas is out of scope.
- **Role rule:** sub-agents answer (research, review, summarize); the parent acts (build, run, send).
  - The reviewer is the second type: a strong cloud model, triggered explicitly or once per task.
- **Settings → Agents editor.**
  - Built-ins are read-only; the user's copies come from "Duplicate to edit".
  - The editor gives hints and autocompletion from the schema the backend publishes, so the UI and the server's
    validation share one definition (see the architecture review).
- **Order of work:**
  1. Delegation + cloud pool + researcher + eval.
  2. `helpers` + reviewer + `files` / `cloud_files`.
  3. The editor.
  4. Pipelines with Event-Driven Triggers, and "teams" only if needed.

Architecture prerequisites: [`2026-10-22-architecture-review.md`](2026-10-22-architecture-review.md).
