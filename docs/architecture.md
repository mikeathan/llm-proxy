# Architecture Reference

How the code is organized **now** and the cross-cutting rules that keep it working. Behavioral contracts per
subsystem are in [`SPECS/`](SPECS/README.md); why things are this way is in [`decisions/`](decisions/README.md);
one page per subsystem linking everything is in [`subsystems/`](subsystems/README.md).

Load only the part your task needs:

| File | Read it when |
|------|--------------|
| [`architecture/map.md`](architecture/map.md) | You need to find which package or directory owns something. |
| [`architecture/contracts.md`](architecture/contracts.md) | Your change touches interfaces, events, prompts, persistence or anything shared across subsystems. |
| [`architecture/conventions.md`](architecture/conventions.md) | You are writing tests or code and want the repo's patterns and standards. |
| [`architecture/checklists.md`](architecture/checklists.md) | You are adding a model field, tool, tool category, connector, settings tab or endpoint — follow the ordered file list. |
| [`architecture/network-audit.md`](architecture/network-audit.md) | You work on the network audit: tool catalog, guarded programs, netaudit domain, report, deep check, vulnerability data. |
| [`architecture/pitfalls.md`](architecture/pitfalls.md) | Before finishing a change in an area with known traps. It is long (~45 KB): search it (`grep -n -i <keyword> docs/architecture/pitfalls.md`) rather than loading it whole; cite pitfalls by their stable number (#N). |

Keep these current: a package move updates `map.md`; a new recurring trap appends a numbered pitfall; a new
change type gets a checklist.
