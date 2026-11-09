---
id: ADR-0036
title: A stack offers findings by id and the core words them
status: accepted
date: 2026-11-09
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0036: A stack offers findings by id and the core words them

## Context

The router DNS check (`stacks/network/dnscheck`) has run hourly since 2026-11-01: 567 comparisons, 567 matches, 0 differ, 0 cannot compare
(`docs/learnings/network-audit.md`, 2026-11-08). Roadmap Phase 3's condition for a finding (a week with no false alarms) is met. The check
keeps only a log of comparisons; a stack has no way to put a finding into the audit's report, which is built in the core (`core/netaudit`,
`models.ScanFinding`). The SDK has `Events` (a message the core may route) and `DataDir` (the stack's own files); neither is a finding
(`stacks/sdk/ports.go`). The matching problem will repeat: every later stack module that should raise a finding meets the same gap, and S6 of
`network-stack-p3b-design.md` moves more modules into the stack.

Constraints: a stack is plain data across ports (Constitution IV.6, ADR-0035); text that came from the network is never put into a finding
(ADR-0025: timeline and finding text from typed fields only); a stack cannot widen what it may say.

## Decision

We will let a stack **offer findings by id**, and the core words and places them.

- A module declares the finding ids it may offer in `ModuleSpec.Finds` (the field the Modules screen already uses for its "found by" line);
  the manifest check refuses a bad or doubly claimed id, and the host passes on only declared ids.
- A new method on `Stack`, `Findings(ctx) ([]StackFinding, error)`, is **pulled** by the core when it builds the nightly report. The stack holds
  no connection to the report and pushes nothing. `StackFinding` is plain data: `ID`, `Subject` (a parsed `netip.Addr`), `Count`, `Names` (host names the
  operator chose) and `Since`. There is no map and no free-text field; a test fails if a text field other than `ID` and `Names` is added.
- The core checks the id against its own table, the subject (a private address of a device this run scanned), the count (1 to 10,000) and each
  name (a plain host name, at most five), and **builds the severity and wording from its own table**. Text a device or resolver sent never
  reaches the report. What fails a check is dropped, never reworded.
- A finding that is not offered in the next pull is gone (resolved), as a module switched off already behaves for the UDP and web checks.

The first user is `dns_router_differs`: raised when the latest comparisons of a configured name differ with no shared address, on at least two
informative checks in a row (a constant, `dnscheck.DifferRun`), newest within 24 hours, at medium severity. `cannot_compare` is never a finding; it stays a module
problem phrase. It sits behind the stored setting `dns_check_finding_enabled`, **off by default**; the operator switches it on (Advanced, on the check's card)
after one deliberate test with a resolver that lies (a local resolver answering a chosen name with a different address) has been run and its
result recorded in the ledger. That live test is still the owner's.

## Consequences

- Easier: later modules raise findings without a core change beyond one wording row and a manifest entry; the nightly report stays the only place
  findings are assembled.
- Harder: `StackFinding` and `FindingSpec` join `PortTypes()` and the IV.6 round-trip test; the report builder gains a pull step that must not
  block the audit (a timeout, and a stack that errors adds nothing and says so in its module status).
- Accepted cost: the wording table lives in the core, so a stack author edits two places. That is the point; it keeps untrusted text out.
- Must stay true: no free-text field is ever added to `StackFinding`; a test fails the build if one is (reflect over the struct, as the port
  test does for other types).

## Alternatives considered

- **Push through `Events`.** The core would need to turn an event into a finding anyway, and an event carries `Data` as raw JSON, so the
  "no free text" rule could not be enforced by type. Pull with a fixed struct can.
- **The core reads the stack's `DataDir` file.** It couples the core to the stack's file format, which ADR-0035 deliberately keeps private.
- **Leave the check as a record only.** Cheapest, but the check then never protects anyone: the owner has to read a log to notice a hijack.
- **Move the DNS check back to the core.** Undoes S2 for one finding and does not help the next module.

## Links

- `docs/PLANS/cross-cutting/network-security-roadmap.md` Phase 3; `docs/PLANS/cross-cutting/network-stacks-and-helper-handoff-2026-11-08.md` item 4.
- ADR-0025, ADR-0032, ADR-0035; `stacks/sdk/ports.go`; `stacks/network/dnscheck/`.
