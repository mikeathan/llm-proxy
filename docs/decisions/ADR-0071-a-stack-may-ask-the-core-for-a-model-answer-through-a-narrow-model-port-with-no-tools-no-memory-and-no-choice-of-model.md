---
id: ADR-0071
title: A stack may ask the core for a model answer through a narrow model port: no tools, no memory, no choice of model
status: proposed
date: 2026-11-20
subsystems: [platform, autonomy]
supersedes:
superseded_by:
---

# ADR-0071: A stack may ask the core for a model answer through a narrow model port: no tools, no memory, no choice of model

Proposed: in force when the owner merges the pull request that carries it. It adds one SDK port, as [ADR-0068](ADR-0068-reminders-are-a-stack-with-its-own-store-a-notify-port-and-a-schedule-port.md) added notify and schedule.
The owner agreed to the port on 2026-11-20, on the condition that both local and cloud models stay allowed, and approved the build the same day (the code exists on branch `topic-watch`; the status changes when the pull request is merged).

**Resolved by the build (2026-11-20):** the call is **not** admitted through the run lane: it is background work and must not take a chat's place. It reuses the existing one-shot completion pattern (`proxy.WaitForClient` for a local model still loading, then one tool-free `Chat`, as the memory review does); the model is the owner's primary, else fallback (`SelectModels`). The daily limit is the fixed `stackport.ModelDailyLimit` (200); an owner setting for it is not built.

## Context
- The Newsroom stack's model step (`stacks/newsroom/topics/analyst.go`) asks a model whether new items are new, changed or repeats. A stack has no way to reach a model: its seam is a stack-local `Model` interface filled by a mock in tests.
- `docs/PLANS/cross-cutting/next-stack-moves-and-local-models.md` section 1 already names this: local models stay core, and a small port that lets a stack **use** a model (not host one) would also unlock `memorycapture` and `delegation`.
- Constitution IV.6: a stack is plain data across ports. ADR-0033: the sandbox is core and a stack gains no way around it.

## Decision
1. **One port, `PortModel`**: `Complete(ctx, ModelRequest{System, User string}) (ModelResponse{Text string}, error)`. Plain data both ways.
2. **The core chooses the model.** The stack names no provider, model, endpoint, header or parameter. The core uses the owner's selection for background work, local or cloud as the owner configured, through the same provider code and the same cloud-or-local classification as a run. When no model is configured or reachable the call fails with a fixed sentence and the stack shows "model unavailable"; it never falls back to something the owner did not choose.
3. **No tools, no memory, no history.** The call is one system and one user message. No memory block is injected, no tool list is sent, no conversation is attached. Injected text in a prompt therefore has nothing to act with; its worst effect is a wrong answer, which the stack checks (ADR-0036's rule: a stack does not put free model prose on the owner's phone).
4. **Bounded.** Prompt at most 64 KiB, answer at most 16 KiB, a timeout (120 s), one call in flight per stack, and a per-stack daily call cap the owner can set (default a safe number). A call over a limit is refused with a code, not truncated silently.
5. **Granted only to a stack whose manifest requires it, and only while its module is switched on** (ADR-0067). The first user is Newsroom.
6. **Logged as metadata only**: stack, size in and out, duration, model class (local or cloud) and result. Never the prompt or the answer.
7. **What it does not do:** run an agent, call tools, read the owner's files or memory, stream, or let a stack choose or add a model.

## Consequences
- Easier: a stack can use a model without owning keys or provider code; `memorycapture` and `delegation` gain the same port later.
- Harder: the SDK has one more port and the core one more adapter (`stackport.Model`), plus a round-trip test (IV.6). Cloud use sends the stack's prompt to the owner's provider: for Newsroom that is the topic name, public text and the state note, never anything the stack cannot already see.
- Open for the build, not decided here: whether a call is admitted through the run lane (a local model on a busy GPU can queue behind a chat; the note "child runs must not re-admit via runlane" applies to nested runs, and this is not one), and how a local model that must be loaded is started for a call.
- Rejected: letting a stack pass a model name or provider (a stack would choose where data goes); giving the port tools (it would become an agent path around the guardrail gate); a model call through the existing automation run (it carries tools and memory, which is the opposite of narrow).

## Links
- `docs/PLANS/ARCHIVE/autonomy/topic-watch.md` Phase 3; `stacks/newsroom/topics/analyst.go`; `docs/PLANS/cross-cutting/next-stack-moves-and-local-models.md` section 1.
