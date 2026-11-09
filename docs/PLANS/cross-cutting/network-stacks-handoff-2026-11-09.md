---
status: partial
date: 2026-11-09
last_reviewed: 2026-11-09
related_specs: [SPEC-003, SPEC-006]
constitution_references: [I.11, I.12, II.3, IV.6]
evidence: PR #216 merged (c4cf7b9b, v0.66.0); branch network-ingest-exchange-ports (docs only); docs/decisions/ADR-0037 and ADR-0038 (accepted); docs/PLANS/cross-cutting/network-stack-s6-plan.md; docs/learnings/network-audit.md (last eight entries); docs/learnings/guardrails.md (last entry). Replaces network-stacks-and-helper-handoff-2026-11-08.md (deleted; #211 to #215 are merged and deployed).
---

# Network stacks: handoff for the next session (2026-11-09, second)

Read this, then `network-stack-s6-plan.md` and ADR-0038/0039. #216 (v0.66.0) is merged and deployed by the owner.

## Start here

```bash
cd /Users/mikeathan/dev/llm-proxy
git fetch origin && git status -sb          # branch network-ingest-exchange-ports (docs only), or its PR
./scripts/check-agent-harness.sh            # must PASS
```

## Branch and state

- Branch `network-ingest-exchange-ports`, cut from origin/main `c4cf7b9b`. **Docs only, no Go or frontend change:** ADR-0039 accepted, ADR-0038 rule 3 amended (validated domain names and IP addresses may cross a port, never in a finding's wording except the core's fixed sentence), the S6 plan's order for step 4 and 5 rewritten, a ledger entry, the READMEs, this note. `VERSION` stays 0.66.0.
- Checks run: `./scripts/check-agent-harness.sh` PASS. Nothing else (no code changed).

## Decisions taken by the owner on 2026-11-09 (ADR-0039)

Q1 yes (validated names); Q2 yes (an exchange port for Nearby's multicast, raw datagrams in, the stack parses); Q3 yes (the core owns every socket and pushes typed observations); programs: `nmap`, `scantool`, `wifilook` stay in the core. **Open:** `portprobe` was not named; ask before moving it.

## Why no code was written (read this before starting)

The first consumer must be `decoy`, but moving it is not "add a push port":

1. `models.DecoyTouch` mixes sensing facts (source, port, first, last, count) with alert state (`Notified`). The alert state is judgement and stays in the core.
2. Twelve core files read the touches: `core/netaudit/decoyfindings.go`, `incident.go`, `attackpaths.go`, `overview.go`, `activity.go`, `fixguide.go`, `remediation.go`, `runreporter.go`, `app/netaudit.go`, `app/netaudit_alerts.go`, and the handlers `netaudit_decoy_handlers.go`, `netaudit_overview_handlers.go`. A stack keeping the table needs a read-back method too (a typed list), which the SDK does not have. `Findings` by id is not enough.
3. State handover: vertex has a persisted touch file. A stack starting empty changes the first night's decoy findings (rule 1 of ADR-0038), so the plan must be one of: the core pushes the existing touches once as observations with counts, or the stack reads nothing and the core keeps the table and the stack only judges. Decide with the owner.
4. The listener callback must never wait on the stack: the host's observe is non-blocking and bounded (drop and count when full), with a test that a stalled stack does not stall the listener.

## What to build next, in order (each its own PR)

1. **Observation port + `decoy`:** SDK `Observer` (plain data: kind id, source address, port, time), `Manifest.Observes`, the manifest check and a drift test; the host's bounded non-blocking push; the stack's decoy module; a typed read-back; golden test of the touch table and the findings pinned first (`decoyfindings_test.go`, `decoy_test.go`); the state-handover choice above. A run on vertex is required (ADR-0039 Consequences).
2. `routerlog` counters and the DNS-query leg (syslog parsers stay in the core; names validated at the port).
3. `phonehome`, only after splitting `Told` from the names; then `knownbad` with the ADR-0037 feed.
4. Nearby through the exchange port (raw datagrams; the stack parses; delete the core's `ParseSSDP` copy then).
5. The ADR-0037 lookup port and the settings flip stay last.

## Owner items on vertex (unchanged from before)

- **DNS finding:** run the lying-resolver test (a local resolver answering a watched name with another address, two checks in a row), record it in the ledger, then switch on "Raise a finding when the router keeps disagreeing" (Network, Settings, Router DNS check, Advanced). The code path is proven by `TestFindings_ALyingRouterIsFoundByTheRealCollectorOnTwoChecksInARow`, not against a real router.
- **Failed logins:** wait for the first real burst to prove the `btmp` byte offset (348) and an internet source; so far `lastb -i` showed only `192.168.50.251`.
- **UDP checks:** switch on "UDP checks, nightly"; after a night keep one real SNMPv3 report and one real SSDP reply as fixtures (`stacks/network/udpchecks/udpchecks_test.go` uses RFC-shaped bytes).
- **capture_window:** the IPv6 check is unproven (0 in every window; the `ping -6` test was skipped) and a window while a device joins is still to run.
- **Data sizes recorded** (2026-11-09, `/var/lib/llm-proxy`): `vulnindex.json` 36,378,016 bytes, `ubuntu-security.json` 647,416, `nearby.json` 3,521.

## The rules every sensor move must meet (ADR-0038)

1. **Findings byte-identical.** Pin the current findings with an exact test before moving anything (precedents: `TestUDPChecks_TheFindingsOfAnAnswerAreByteIdentical`, `TestWebFindings_AreByteIdenticalToWhatTheyWereBeforeTheMove`), then show the same result through the whole new path (`TestNetworkStack_AnAnswerThroughTheSensorIsTheSameFindingAsBefore`, `…AWebPageThroughTheSensor…`). Acks, fixes and the baseline diff are keyed on the finding's fields.
2. **A stack never probes wider than the core.** Use `Host.ScopedGuard` only, never `Host.Guard`; keep a test that an out-of-scope address is never dialled, and run it once with the scope check removed to see it fail (precedents: `TestNetworkStack_TheUDPSensorNeverDialsAnAddressOutsideTheDeclaredScope`, `TestNetworkStack_TheWebSensorNeverDialsAnAddressOutsideTheDeclaredScope`).
3. **No text from the network across a port.** Ids and numbers only; the core owns words and severity and drops what it does not know. `TestLookTypes_CarryNoTextFromTheNetwork` and `TestWebTypes_…` list the only allowed text fields.

The recipe that worked twice: pin goldens → SDK types and manifest check (test first) → core types and words → move the package with its tests → stack method → host method with its limit and bounds → the core's adapter doing the checks the old code did for itself (so a forbidden run is an error the report states, never "nothing found") → wire in `app/netaudit.go` → drift test between the core's table and the stack's list → docs, version, commit.

## Traps found (so you do not repeat them)

- **A stack's `Guard` does not apply the declared scan scope**; only `ScopedGuard` does. A refused address and a silent device look the same to the caller, so a sensor that cannot look must be treated as "unknown", never "clean" (`UDPResult.unknownEverywhere`).
- **The stack architecture test bans `net/http.Transport`** (and any dialling outside the SDK). `webprobe` writes one request on the scoped-dial connection and reads one response instead.
- **Test imports count** in the architecture test; only `internal/app` may import a stack, test files included.
- **`stacksdk.Errorf` takes a fixed message, not a format.** That is deliberate.
- **Stack tests need `GOWORK=off`** (`cd stacks/network && GOWORK=off go test -race ./...`); a stack cannot import `llm-proxy/internal/...`.
- **A fixed host wait cuts a long run.** The web look waits the request's own limit plus a grace (`webLookGrace`); the UDP look has a fixed 3 minutes (`lookTimeout`).
- **The prober paces 200 ms between requests**, so a unit test that reaches a real stack runs for seconds; give a refused target a one-second limit.
- **Go's DNS client adds an EDNS record** to its query; a test responder must echo only the header and the question.
- **Complexity limit 12 is enforced for new code** (`go run ./tools/check-complexity/`); my first `UDPChecks.Findings` was 13 and was split.
- **Pre-existing `gofmt -l` entries:** `app/bootstrap.go`, `app/netaudit.go` (the one-line `routerLogSource.Snapshot`) and four handler test files. Do not reformat them in an unrelated change; if you touch the imports of `app/netaudit.go`, fix only the import block.
- **The commit hook prints a CRAP "FAIL (no coverage data)" list and still commits;** it is informational.
- **Git:** the repo rules make git read-only unless the owner approves a mutation. I created the branch with `git checkout -b` before reading that rule; ask first. Moving a directory: use plain `mv` and let git see the rename. To commit a mixed file in two commits, build the intermediate blob with `git hash-object -w` and `git update-index --cacheinfo`, then check the index builds by exporting it with `git checkout-index -a --prefix=/tmp/x/`.
- **`core/netaudit/upnp` is not dead:** Nearby still uses `ParseSSDP` and `Identify`; the stack holds a copy of `ParseSSDP`. Delete the core's copy when Nearby moves with the ingest port.
- **Linux-only code on this Mac:** the colima VM runs it. `cd backend && docker run --rm --security-opt seccomp=unconfined -v "$PWD":/src -w /src golang:1.26 go test ./internal/platform/sandbox/ ./internal/core/tools/ ./internal/shell/ ./internal/platform/progexec/ -count=1` ran with Landlock available; the first run downloads modules.
- **zsh:** quote globs (`'*.md'`), `ls` is aliased to a long listing (use `command ls -1`), `command grep` avoids the ugrep alias, and macOS `sed -i` needs `-i ''`.

## Traps (still valid)

- A stack's `Guard` does not apply the declared scan scope; only `ScopedGuard` does. A sensor that cannot look is "unknown", never "clean".
- The stack architecture test bans `net/http.Transport`; only `internal/app` may import a stack (test files count). Stack tests need `GOWORK=off`.
- `stacksdk.Errorf` takes a fixed message. Complexity limit 12 for new code. Pre-existing `gofmt -l` entries: `app/bootstrap.go`, `app/netaudit.go` (one line), four handler tests; do not reformat them.
- The commit hook prints a CRAP list and still commits. Git is read-only unless the owner approves; plain `mv` for moves. zsh: quote globs, `command ls -1`, `sed -i ''`.
- Linux-only code runs in the colima container (see the previous handoff's command in git history, `c4cf7b9b`).
- `core/netaudit/upnp` is not dead: Nearby still uses `ParseSSDP` and `Identify`.

