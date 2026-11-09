---
status: partial
date: 2026-11-09
last_reviewed: 2026-11-09
related_specs: [SPEC-003, SPEC-006]
constitution_references: [I.11, I.12, II.3, IV.6]
evidence: branch network-small-items (local, not pushed, 7 commits 985814a8..bbdc22cf on top of 84b8d27a, plus one uncommitted change for v0.66.0: the router check, `upnp`); docs/decisions/ADR-0037 and ADR-0038 (accepted); docs/PLANS/cross-cutting/network-stack-s6-plan.md; docs/learnings/network-audit.md (last eight entries); docs/learnings/guardrails.md (last entry). Replaces network-stacks-and-helper-handoff-2026-11-08.md (deleted; #211 to #215 are merged and deployed).
---

# Network stacks: handoff for the next session (2026-11-09)

Read this first, then `network-stack-s6-plan.md` and ADR-0038 before touching a sensor. Then the item you take below.

## Start here

```bash
cd /Users/mikeathan/dev/llm-proxy
git status -sb && git log --oneline 84b8d27a..HEAD     # the 7 commits below; the tree should be clean
./scripts/check-agent-harness.sh                        # must PASS
```

Open `docs/decisions/ADR-0038-…md` (the rules and the "Built" notes), then `backend/internal/app/stackhost.go` (the host side of both sensors) and `stacks/network/network.go` (`Look`, `LookWeb`, `LookCertificates`).

## Branch and commit state

- **Branch `network-small-items`, local only, nothing pushed.** The seven commits below are committed; the `upnp` move (v0.66.0, SPEC-003 2.54, ADR-0038 "Third sensor") is **uncommitted** in the working tree (`git status` shows it). `VERSION` 0.66.0. SPEC-006 1.11.
- **Merge trap (found 2026-11-09):** the branch was cut from `network-stack-findings-port`, so its history holds six commits that #215 already squash-merged into `origin/main` (`41fb5e79`, `eb640844`, `585b210e`, `05710e25`, `76ff98b9`, `84b8d27a`). The tree at `84b8d27a` equals `origin/main` exactly (`git diff origin/main 84b8d27a` is empty), but a PR from this branch would re-show them and conflict in `VERSION`, the SPEC headers and `index.html`. **Recommended, needs the owner's approval (git mutation):** a fresh branch from `origin/main`, then `git cherry-pick 84b8d27a..bbdc22cf` (seven commits, no conflicts expected), then push that. Check `git grep '^<<<<<<<'` after.
- **The seven commits, oldest first:**

| Commit | What |
|---|---|
| `985814a8` | D9: a spawn with no workspace is confined (read-only system paths, nothing writable; SPEC-006 1.11) |
| `44705aec` | the lying-resolver test, end to end, through the real collector |
| `24a587ab` | OS guesses with no open port demoted; the DNS card mock sends `finding_enabled` (SPEC-003 2.51); rebuilt `index.html` |
| `8dada4ba` | `vulnindex`, `vulnmatch`, `vulnfeed`, `ubuntuoval` moved to `internal/core/vulndata/` |
| `f7e693ad` | `migrationDebt` retired (injected `VendorLookup`), ADR-0037 and ADR-0038 accepted, the S6 plan (v0.63.0) |
| `614fd555` | the UDP checks asked by the Network stack: scoped dial, `Looker`, `Manifest.Looks` (v0.64.0, SPEC-003 2.52) |
| `bbdc22cf` | the web and certificate checks asked by the stack: `WebLooker`, core words table (v0.65.0, SPEC-003 2.53) |

## Needs the owner before or with the PR

1. **Constitution IV.6 wording changed** in `f7e693ad`: the transitional clause about the shrink-only `migrationDebt` list is gone (the list no longer exists). No rule is weaker; merging is the approval (V.1).
2. **Two behaviour differences of the web checks (SPEC-003 2.53)** — keep or revert: they now honour the Guardrails local-network switch (before: declared scan scope alone; with the switch off the nightly report now says the checks could not run, and a deep check stores no web or certificate findings), and requests no longer ask for compression.
3. **Not run on vertex:** nothing from these seven commits or from the `upnp` move. After a night with the router check, compare `upnp_enabled`, `upnp_forward` and `internet_exposed_port` with the night before: none resolved, none new; the Internet view's status should read the same.
3a. **`upnp` differences to keep or revert (SPEC-003 2.54):** the report's message for a router it could not read is now one fixed sentence per reason (no network error, no router error code), and the requests no longer ask for compression. After a first night with the UDP module and the web module on, compare the `udp_service`, `web_exposure` and certificate findings with the night before: none resolved, none new.

## Owner items on vertex (unchanged from before)

- **DNS finding:** run the lying-resolver test (a local resolver answering a watched name with another address, two checks in a row), record it in the ledger, then switch on "Raise a finding when the router keeps disagreeing" (Network, Settings, Router DNS check, Advanced). The code path is proven by `TestFindings_ALyingRouterIsFoundByTheRealCollectorOnTwoChecksInARow`, not against a real router.
- **Failed logins:** wait for the first real burst to prove the `btmp` byte offset (348) and an internet source; so far `lastb -i` showed only `192.168.50.251`.
- **UDP checks:** switch on "UDP checks, nightly"; after a night keep one real SNMPv3 report and one real SSDP reply as fixtures (`stacks/network/udpchecks/udpchecks_test.go` uses RFC-shaped bytes).
- **capture_window:** the IPv6 check is unproven (0 in every window; the `ping -6` test was skipped) and a window while a device joins is still to run.
- **Data sizes recorded** (2026-11-09, `/var/lib/llm-proxy`): `vulnindex.json` 36,378,016 bytes, `ubuntu-security.json` 647,416, `nearby.json` 3,521.

## What to build next, in order (each its own PR; stop at any line)

State of S6 option B (sensors in the stack, judgement in the core): **built** the scoped dial, the `Looker`, `WebLooker` and `RouterLooker` methods, `udpchecks`, `webprobe`, `upnp`. **Left:**

1. **The ingest and exchange ports (S4)** for `routerlog`, `phonehome`, `knownbad`, `decoy` (listeners: the core keeps the sockets and pushes observations in, design D6) and `nearby` (it sends multicast, so it needs an exchange port, not ingest). **ADR-0039 is drafted, status `proposed`, nothing built:** the owner must answer its Q1 (do validated host names count as allowed text, as `StackFinding.Names` already does), Q2 (an exchange port for Nearby's multicast) and Q3 (observations pushed in). Real multicast and listener behaviour needs a run on vertex.
2. **Programs (S5):** `nmap`, `portprobe`, `scantool`, `wifilook`. The scoped dial does **not** cover a program: `nmap` opens its own sockets. Today the root's `PolicyAuthorizer` checks every target before the engine runs and the engine receives `[]netip.Addr`. The owner chooses (a) a typed method whose host checks every address, plus a check of the target arguments in the isolation adapter, or (b) keep `nmap`, `scantool` and `wifilook` in the core (ADR-0039, "Programs"). `scantool` is the agent's `network_scan` tool (it uses `core/assistant/prompts`, `toolregistry` and `models` context values), not a sensor.
3. **The ADR-0037 lookup port**, with its first caller (the flaws and known-vulns matching), optional in the manifest. No caller exists until the root `netaudit` pieces that call `vulnmatch` and `ubuntuoval` move.
4. **Settings flip and the Modules definitions** move last, with the round-trip test on a real `settings.yml`.

Small and open: the OpenSandbox trial (`opensandbox-evaluation.md`, five questions in phase 1); whether to hide OS guesses with no open port (decide from more real results; one line in `DeviceProbe.vue`); the full frontend visual suite was run on 2026-11-09 after the `upnp` move (344 passed; no frontend file changed since the earlier commits).

**Decided not to build (reasons in `docs/learnings/guardrails.md` and the helper design):** D8's second half (the service marking itself non-dumpable) and N2e (`read_auth_log`).

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

## Checks run for the seven commits

Backend `go build ./...`, full `go test ./...` (no FAIL), `-race` on `internal/app`, `internal/core/netaudit`, `internal/core/stackport`; `go run ./tools/check-complexity/` OK (49 baseline entries, no new); `stacks/sdk`, `stacks/network` (with its sub-packages) and `stacks/example` under `GOWORK=off go test -race`; Linux container for the sandbox, tools, shell and progexec packages; the DNS-card and devices Playwright specs (54 passed); Vitest 2,740; `./scripts/check-agent-harness.sh` PASS. **Not run:** the full Playwright visual suite after the last frontend change; anything on vertex.
