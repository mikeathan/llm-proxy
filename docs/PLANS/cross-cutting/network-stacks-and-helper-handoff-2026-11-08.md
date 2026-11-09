---
status: partial
date: 2026-11-08
last_reviewed: 2026-11-08
related_specs: [SPEC-003, SPEC-006]
constitution_references: [I.11, I.12, II.3, IV.6]
evidence: branch network-capture-window (local, unpushed); CONSTITUTION.md I.12; docs/PLANS/cross-cutting/network-stack-p3b-design.md section 4e; docs/PLANS/cross-cutting/network-capture-window-plan.md; docs/PLANS/cross-cutting/network-nightly-udp-probes.md; docs/PLANS/cross-cutting/opensandbox-evaluation.md; docs/learnings/network-audit.md (last six entries). Replaces network-stacks-s2-handoff-2026-11-08.md (deleted; #209 and #210 are merged).
---

# Network stacks, helper, UDP checks and capture: handoff (2026-11-08, second session)

Read this, then the plan named in the item you take. Everything below is on **local, unpushed** branches.

## State

- **Branch chain (each is on top of the one before; nothing pushed):** `main` a51e920f (#210, v0.58.1) → `network-stacks-s3-opensandbox-plan` (3 commits: the OpenSandbox plan; the nightly UDP checks v0.59.0; the OS-guess note v0.59.1) → `network-capture-window` (C1, C2 backend, and the C2 frontend and docs commit; v0.60.0). The first branch's name is misleading (it carries no S3 work). **Merge order:** one PR from `network-capture-window` carries all of it, or split by cherry-picking the three groups; the owner decides. `VERSION` is 0.60.0.
- **Deleted on the remote** (owner approved): `network-privileged-helper-n1-design` (its content was in #205 to #207).
- **AGENTS.md** gained the rule that produced this file (Execution Protocol: hand over unfinished work); the stewardship skill has the matching row.

## Built, and what was checked

| Built | Checks run | **Not** run |
|---|---|---|
| Nightly UDP checks (`core/netaudit/udpchecks`, `udpfindings.go`, module `udp_probes`, Constitution I.12, SPEC-003 2.46) | backend `go test ./...`, `-race` on the touched packages, `go vet`, complexity, harness | any real SNMPv3 or SSDP reply: fixtures are RFC-shaped |
| OS guesses marked unreliable with no open port (SPEC-003 2.47) | frontend 2,728 tests, build, visual tests | none |
| `capture_window` C1 + C2 (SPEC-003 2.48; plan `network-capture-window-plan.md`) | `go test ./...`, complexity, `go vet`, `GOOS=linux` build for amd64 and arm64, a BPF interpreter test of the filter, the frontend 2,735 tests, build and visual tests | **anything that opens the packet socket** (the Linux code was only compiled), design test 6 (`isolationtest` on the helper's jail), the helper under its real unit |
| P3b section 4e (S3 cannot be built as written) | reading only | no code moved |

## Left, in order (what each needs)

1. **Done 2026-11-09:** the branch chain was merged as #211 (v0.60.0), the first-press fix as #212 (v0.60.1) and deployed. Nothing is unpushed except the branch `network-capture-window-jail-test`.
2. **Owner on vertex, capture_window:** the first real press worked on 2026-11-09 (143 frames in 60 s, 0 unreadable; ledger entry in `docs/learnings/network-audit.md`). Four windows later (v0.61.0): the card folds copies and flags addresses outside the link's range; M3 is answered yes (a VLAN leaking onto the SSID was shown). Left: the IPv6 check (0 in every window, unproven; the owner skipped the `ping -6` test) and a window while a device joins. C3 findings only if real windows keep showing no false alarms.
3. **Owner on vertex, UDP checks:** switch on the **UDP checks, nightly** module (Network, Modules); after a nightly run keep one real SNMPv3 report and one real SSDP unicast reply as test fixtures (`udpchecks_test.go` uses RFC-shaped bytes). A device that speaks only SNMP v1 or v2c is missed by design.
4. **Owner decision: a DNS finding.** The 7-day data (567 of 567 matches, 0 differ) meets Phase 3's condition. It needs a way for a stack's data to reach the core's findings (a new SDK port plus an ADR) and one deliberate test with a lying resolver. Not started.
5. **Owner decision: S3 option A** (shrunk S3). Moving the vuln and Ubuntu feed stores means the root `knownvulns.go` and `flaws.go` can no longer read `models.VulnIndex` and `UbuntuIndex`: a read port, or the matchers move too (S6). Section 4e's "about 1,900 lines" was optimistic. Then S4 (ingest port), S5 (programs), S6 (root `netaudit`, settings opaque, the capture summary store moves into the stack).
6. **Owner decision: OpenSandbox trial** (`opensandbox-evaluation.md`): run its Docker mode in a scratch directory and answer the five questions in phase 1.
7. **Open, small:** hide or demote OS guesses on a device with no open port (today only a note; decide from more real results); D9, D8 second half, N2d, N2e (see `network-privileged-additions` and the helper design); the stale `discoverModelFiles` baseline entry in `tools/check-complexity`.

## Traps found this session

- In zsh an unquoted `--include=*.go` fails with "no matches found"; quote it. `ls` is aliased to a long listing; use `command ls -1`. `command grep` avoids the ugrep alias. macOS `sed -i` needs `-i ''`.
- The commit hook prints a CRAP "FAIL (no coverage data)" list and still commits; it is informational today.
- `gofmt -l` lists `app/bootstrap.go`, `app/netaudit.go` and four handler test files before this work; do not reformat them in an unrelated change.
- The Router DNS check's Copy results button is on **Settings**, in Advanced, not on the Modules tab (the Modules card links to it inside its "What it does" fold).
- A packet socket created with `ETH_P_ALL` queues frames at once: create it with protocol 0, attach the filter, then bind. 64 bytes never reach DHCP options (they start at byte 282).
- Switching the UDP checks off drops their nightly findings from the next report (the diff shows them as resolved); the web checks behave the same way.
- Stack tests cannot import `llm-proxy/internal/...`; `stacks/network` needs `GOWORK=off` for `go test`.

## Start here

`docs/PLANS/cross-cutting/network-capture-window-plan.md` (Remaining Work) for item 2, or `network-stack-p3b-design.md` section 4e for item 5. Run `./scripts/check-agent-harness.sh` after any doc edit.
