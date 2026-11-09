---
status: partial
date: 2026-11-08
last_reviewed: 2026-11-09
related_specs: [SPEC-003, SPEC-006]
constitution_references: [I.11, I.12, II.3, IV.6]
evidence: #211 to #215 merged (v0.60.0 to v0.62.0); branch network-small-items (local, uncommitted, v0.63.0); CONSTITUTION.md I.12; docs/PLANS/cross-cutting/network-stack-p3b-design.md section 4e; docs/PLANS/cross-cutting/network-capture-window-plan.md; docs/PLANS/cross-cutting/network-nightly-udp-probes.md; docs/PLANS/cross-cutting/opensandbox-evaluation.md; docs/learnings/network-audit.md (last six entries). Replaces network-stacks-s2-handoff-2026-11-08.md (deleted; #209 and #210 are merged).
---

# Network stacks, helper, UDP checks and capture: handoff (2026-11-08, refreshed 2026-11-09)

Read this, then the plan named in the item you take. Everything up to v0.62.0 (#211 to #215) is merged and deployed; the small items of 2026-11-09 are on the local, uncommitted branch `network-small-items` (see "Added 2026-11-09, small items").

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
4. **DNS finding: built (2026-11-09, v0.62.0, ADR-0036), off by default.** Owner on vertex: run the lying-resolver test (a local resolver answering a watched name with another address, two checks in a row), record it in the ledger, then switch on *Raise a finding when the router keeps disagreeing* (Network, Settings, Router DNS check, Advanced).
5. **Decided 2026-11-09: ADR-0037 option C (the feeds stay core reference data); the package move is built (v0.63.0).** What is left of S3 is only the lookup port, added with its first caller. **S6 is planned in [`network-stack-s6-plan.md`](network-stack-s6-plan.md)**: `migrationDebt` is deleted (v0.63.0); moving the rest needs the owner's choice A, B or C there. Reading the ports found that option A below needs a large-data push-in port as well (Fetch 1 MiB, route body 4 MiB, import up to 256 MiB). Earlier text: S3 option A (shrunk S3). Moving the vuln and Ubuntu feed stores means the root `knownvulns.go` and `flaws.go` can no longer read `models.VulnIndex` and `UbuntuIndex`: a read port, or the matchers move too (S6). Section 4e's "about 1,900 lines" was optimistic. Then S4 (ingest port), S5 (programs), S6 (root `netaudit`, settings opaque, the capture summary store moves into the stack).
6. **Owner decision: OpenSandbox trial** (`opensandbox-evaluation.md`): run its Docker mode in a scratch directory and answer the five questions in phase 1.
7. **Small items: done 2026-11-09 on `network-small-items`** (OS guesses demoted, D9 built, the lying-resolver test, the Playwright mock). N2d is built (`capture_window`). **Decided not to build:** D8's second half (non-dumpable service) and N2e (`read_auth_log`); reasons in `docs/learnings/guardrails.md` and the helper design. Still open and small: nothing that does not need the owner's vertex.

## Added 2026-11-09 (branch `network-stack-findings-port`, local, unpushed, uncommitted)

- **Built:** ADR-0036 (accepted): `ModuleSpec.Finds`, `Stack.Findings`, `stacksdk.StackFinding`, `core/netaudit/stackfindings.go`, `stackHost.Offered`, `dnscheck.Persistent`, the `dns_check_finding_enabled` switch and its Advanced checkbox. Failed-login findings say where from (btmp `ut_addr_v6`; `platform/loginlog.Attempts`). VERSION 0.62.0, SPEC-003 2.50.
- **Checks run:** backend `go test ./...`, `go vet`, complexity; `GOWORK=off go test -race ./...` in `stacks/network`; `stacks/sdk` tests; frontend 2,740 tests, build, the DNS-card visual spec; harness. **Not run:** the full frontend visual suite, anything on vertex.
- **Constitution I.10 is amended in this branch (proposed):** it said no address could be kept. Merging the PR is the owner's approval (V.1). Do not deploy before that.
- **auth.log helper action (`read_auth_log`, N2e):** not built and now not needed for "where from". Confirmed on vertex 2026-11-09: `sudo lastb -i | head` shows `192.168.50.251` for all three failures, so sshd fills the address. Still to see: the first real burst (proves the byte offset) and an internet source.
- **Not done:** a lying-resolver end-to-end test through the real collector (only `Persistent` and `Findings` are tested); the Playwright mock was not given `finding_enabled` (the field is optional).

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

## Added 2026-11-09, small items (branch `network-small-items`, local, uncommitted, v0.63.0)

- **Built:** (a) `TestFindings_ALyingRouterIsFoundByTheRealCollectorOnTwoChecksInARow` (`stacks/network/network_test.go`): a UDP responder that lies, the real collector, two checks, then the finding; (b) the Playwright mock carries `finding_enabled` and the DNS card spec checks the Advanced switch; (c) OS guesses on a device with no open port are muted and have no percentage, the warning first (SPEC-003 2.51); (d) D9: a spawn with no workspace is confined (Linux provider no longer returns early; `DefaultProfile("")` is read-only system paths; SPEC-006 1.11).
- **Checks run:** `GOWORK=off go test -race ./...` in `stacks/network` (3 runs) and `go vet`; the DNS-card Playwright spec (32 passed); `deviceProbe` Vitest; in a Linux container (colima, Landlock) the sandbox, tools, shell and progexec packages, including the new rootless probe. See the final message of the session for the full-suite results that were run last.
- **Not run:** anything on vertex; the real lying-resolver test against a router is still the owner's (item 4 above).
- **Trap:** the first draft of the lying-resolver responder echoed the whole query; Go's resolver adds an EDNS record, so the reply was malformed and the check logged `router_no_answer` (ledger entry).

