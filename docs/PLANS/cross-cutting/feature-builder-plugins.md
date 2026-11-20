---
status: proposed
date: 2026-11-20
last_reviewed: 2026-11-20
related_specs: [SPEC-006, SPEC-008, SPEC-001]
constitution_references: [I.1, II.3, III.3, IV.6, V.2]
related_plans: [mcp-in-agents-stack.md, standing-goals.md, ../autonomy/autonomy-roadmap.md]
evidence: Read 2026-11-20 - CONSTITUTION II.3 (sandbox isolation, guarded programs), III.3 (workspace jail), ADR-0006, ADR-0028, ADR-0032 (rejects in-process plugins and parks a process boundary), ADR-0051, ADR-0069 (MCP, stdio out of scope), stacks/sdk manifest, delegation.json. Nothing was run or built.
---

# Feature Builder — the service builds a feature and adds it as a plugin

**Status:** proposed (2026-11-20). **This is a research and options plan, not a build plan.** The owner said it "needs more thinking" and "a lot
of planning". Its first deliverable is a decision, not code.

## What the owner asked for

A user asks for a feature; the service builds it with whichever model is configured (local or a provider), and it is added to the running system as a
plugin, so features can be added live. Answers already given: finished work lands as a **branch and pull request for review**; requests and
progress live in a **backlog list in the UI**, started from the UI, chat or Telegram; the build host is **vertex**.

## Ground truth: why this is hard here

| Fact | Source |
|---|---|
| Agent file access is jailed to the workspace; the repo is outside it | Constitution III.3 |
| No raw `os/exec` for agent work; code runs in the sandbox. A *guarded program* is a **known binary** with arguments built in code | Constitution II.3 |
| Stacks are **compiled in**; the SDK is the only thing they import. **In-process plugins (Go `plugin`, WASM) were rejected**; a process boundary is "parked" | ADR-0032 |
| A stack can offer tools learned at run time (`DynamicTools`); MCP servers are such a source, over HTTP, trust Ask by default; **stdio (starting a program) is out of scope** | ADR-0069, `stacks/sdk/manifest.go` |
| Helper agents have no file access | ADR-0006 |
| Dashboard cards are frontend code (`frontend/src/constants/dashboard.ts`); a plugin cannot add one without a build | repo (file listed, not read in depth) |
| The local model overflowed at 16k on breadth tasks; multi-file Go work is untested | ADR-0006 evidence; **assumption** for code |
| The repo rule: no commit, push, branch or PR without the owner's approval | AGENTS.md |

## Three different things the owner may mean

| Tier | What it is | Live without a rebuild? | New constitutional ground? |
|---|---|---|---|
| **A. Declarative feature** | The service writes an automation, a task file, a playbook or a watch topic (no code) | Yes | No: uses today's create-automation path with the owner's review |
| **B. Plugin process** | The service generates a small program (a tool server) in a sandbox workspace and runs it as a separate process; the Agents stack's MCP client adds its tools live | Yes (tools); no UI of its own | **Yes.** Today only *known binaries* are guarded programs; generated code is untrusted. Needs an ADR reopening ADR-0032's parked process boundary and an amendment of II.3 |
| **C. Core change** | A real code change to llm-proxy (a new stack, a route, a card) built in a throwaway clone on vertex and proposed as a PR | No: needs review, merge, build, restart | Smaller: a guarded program runs a coding agent on a clone; needs a repo credential and a toolchain on vertex |

The owner's picture ("added as a plugin, live") is **B**. The owner's words ("build new features for llm-proxy") are **C**. **A** is the part that
needs no new trust and could ship first.

## Design sketch for B (the plugin process), to be decided

- **Shape.** One plugin = a folder of source + a manifest (name, tools, the hosts it may reach, the files it may touch). Built and tested in the sandbox,
  run as a child process under Landlock with no network unless granted, no unix sockets (II.3), a memory and CPU cap, and a private data folder.
- **Contract.** Tools are exposed over the MCP client already built (HTTP on loopback), so a plugin is "an MCP server the service started". Every call
  goes through the guardrail engine with trust **Ask** until the owner promotes it (ADR-0069 item 4).
- **Install gate.** The owner sees the source diff, the manifest and the test result, and approves once. A change to the code needs approval again.
- **Rollback.** Switch off or remove = stop the process and drop its tools; the data folder is kept until the owner deletes it.
- **Limits.** A plugin cannot add a dashboard card, a route or a setting (those are compiled code). If that matters, it needs C or a generic card type
  (a plugin returns a table or text card; the UI already renders those kinds, unverified).
- **Model quality.** Whether the local model can write a working plugin is unmeasured. A small eval (ten requests, pass/fail by tests) comes before any
  build, as the other plans did.

## Design sketch for C (PR on a clone)

A queued request makes a fresh clone on vertex; a guarded program runs the coding agent with a fixed argument table and a clean environment; tests run in
the same clone; the result is a branch plus a PR description following `pr-commit-message`. The agent never sees secrets, never pushes to the main branch, and the
owner merges. Needs: the toolchain on vertex, a scoped repository token, and the egress grant for the code host and the model provider.

## Decisions for the owner

| # | Decision | Recommendation |
|---|---|---|
| D1 | Which tier is the goal: A, B, C, or A then B then C? | A first (cheap, no new trust), then measure C, then decide B |
| D2 | May we reopen ADR-0032's parked process boundary for B? | Only after an eval shows the model can write working plugins |
| D3 | Whose model builds: the configured one, or always the cloud one? | Cloud model for code; local for the rest |
| D4 | May a plugin reach the network? | No by default; one named host at a time, with the owner's grant |
| D5 | Is a PR (C) enough, or must something go live without a restart? | PR for core changes; live only for B plugins |

## Phases (only after D1-D5)

0. **Evidence**: an eval of the configured models on ten small plugin and ten small core requests, scored by tests. *Accept:* a numbers table in `docs/audits/`.
1. **Tier A**: "build this" turns a request into a draft automation, task file or watch topic, reviewed before saving (builds on `chat-to-automation.md`).
2. **Backlog**: request list, statuses (queued, building, ready, failed), logs, cancel. *Accept:* a failed build leaves nothing running.
3. **Tier C**: clone, guarded coding run, tests, PR text. *Accept:* a run on vertex produces a PR; the main branch is untouched.
4. **Tier B**: ADR + II.3 amendment, the plugin runner, install gate, rollback. *Accept:* an escape test (a plugin tries a write outside its folder, a socket, a
   network call) fails every time.
5. **Docs**: SPEC, hub, ADRs, learnings.

## Risks

Generated code runs with the service's host as its neighbour (the sandbox is the only barrier; a flaw in it is now reachable by model output). Supply chain: a plugin
that pulls dependencies. Prompt injection: a web page that steers the builder. Cost: a coding run is long and uses cloud tokens. The service editing its own
source while running (C writes only to a clone, never the live tree).

## Remaining Work

All of it. First step is D1, then Phase 0 (the eval), which needs no code in the service.
