---
id: ADR-0067
title: A stack is switched off by switching all of its modules off; it has no setting of its own
status: accepted
date: 2026-11-19
subsystems: [admin-ui, platform]
supersedes:
superseded_by:
---

# ADR-0067: A stack is switched off by switching all of its modules off; it has no setting of its own

## Context
Only modules could be switched (ADR-0028), one at a time, on each stack's Modules tab. The owner noticed (2026-11-19) that there was no way
to disable a stack. In the code a stack is already "on" exactly while one of its modules is on: the menu entry, the dashboard board and the
cards (ADR-0066, `useEnabledStacks`) all read that. Evidence: `core/stack/registry.go`, `handlers/stacks_handlers.go`,
`composables/stacks/useEnabledStacks.ts`, ADR-0028, ADR-0064.

## Decision
Switching a stack off means switching off every module of it that is on: `POST /admin/api/stacks/{stack}/off`, one button on the Modules tab
behind a confirmation. No stack-level setting is stored, so there is no second source of truth that could disagree with the modules. The
passphrase is asked (the same `reauth_required` cue as a module switch) only when a module that is on needs it, checked before anything is
changed. A failure part-way stops at that module; the ones before stay off and the answer's error names it. There is no route to switch a stack
**on**: which modules to run is the person's choice, made one by one on the same tab; a stack with nothing on says so there and on the Stacks page.
Settings and what the modules already found are kept.

## Consequences
- The route is checked in the handler like the module route, so a test walks the registry: with a passphrase-guarded module on, the route is
  refused without the passphrase and nothing changes.
- A person who wants "everything back as it was" must switch modules on again; a remembered set of modules was not built (nothing asked for it).

## Alternatives considered
- **A stored per-stack enabled flag**: a second truth next to the modules; the menu, board and registry would each have to honour it.
- **The browser switching each module in turn**: many requests, many passphrase checks, and a half-off stack on a network error.
- **A matching "switch on" that restores the last set**: more state than the request needs; left out.

## Links
- ADR-0028 (modules are the one switch), ADR-0066 (a stack that is off is hidden), SPEC-003 2.70.
