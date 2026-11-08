---
status: partial
date: 2026-11-08
last_reviewed: 2026-11-08
related_specs: [SPEC-003, SPEC-006]
constitution_references: [I.7, I.9, I.11, II.3, IV.6]
evidence: PR #209 (branch network-flaky-test-isolation-switch-dns-s2, v0.58.0); docs/decisions/ADR-0035-a-stack-reaches-the-service-only-through-named-ports-and-keeps-only-the-files-it-lists.md; docs/PLANS/cross-cutting/network-stack-p3b-design.md (section 4d and its S2 notes); docs/PLANS/cross-cutting/network-nightly-udp-probes.md; docs/PLANS/cross-cutting/network-capture-window-plan.md; docs/learnings/network-audit.md (last three entries)
---

# Network stacks, S2 and the helper work: handoff for the next session (2026-11-08)

**Update (same day, v0.58.1):** #209 is merged and deployed. The port 53 label and the router's #207 results are resolved (items 2 and 3 below); a small follow-up PR carries the label change.

This replaces `network-helper-and-stacks-handoff-2026-11-08.md` (deleted; its owner items and left-over items are carried below). Read this, then ADR-0035,
then section 4 of `network-stack-p3b-design.md`. Start a new branch from `main` once #209 is merged (check with `git log origin/main`).

## State

- **#208 (v0.57.0) is merged and deployed** (Clear for kept helper scan results; the two new plans).
- **#209 (v0.58.0) is open**, branch `network-flaky-test-isolation-switch-dns-s2`, one commit on top of #208. It carries four things:
  1. **P3b S2: the router DNS check is a stack.** `stacks/network` (`network.go`, `dnscheck/`), the SDK ports `Fetch`, `Gateway`, named-file `DataDir`,
     `Stack.Status`, `Manifest.Storage`; the core adapters in `backend/internal/core/stackport/`; the host in `backend/internal/app/stackhost.go`;
     the only stack import in `backend/internal/app/stacks.go` (`networkStack`, `newNetworkHost`). Route JSON, settings key and
     `network-dns-checks.json` unchanged, so no frontend change. Old code deleted from `internal/`.
  2. **`Config.Save` race fixed** (decoded into a shallow copy of the stored settings; found by the race detector; ADR-0035).
  3. **Callers use `platform/isolation`** (13 files; `nmap`, `wifilook`, `app/netaudit.go` take `isolation.Runner`).
  4. **`routerlog` test flake fixed** (UDP burst then the asserted datagram; the reader now drains first).
- Checks run on the branch: `go build`, `go vet`, `go test ./...`, `-race` on `stackport`/`app`/`stacks/network`, complexity, harness, the three stack
  modules with `GOWORK=off`, the settings round-trip test, and the section 4b figures (in the plan). Frontend untouched, so not run.
  **Not run on vertex.**

## Do next (needs the owner)

1. **Review and merge #209**; then deploy to vertex and press the DNS check card once (Router DNS check: Settings, the summary, the Copy results button), to
   see the stack run for real. The owner's `go.work` does not list `stacks/network`; that is fine (the backend reaches it through a `replace`), but
   `go test` inside `stacks/network` needs `GOWORK=off`.
2. **The router's port 53 label: decided and built (v0.58.1, PR after #209).** The owner ran `dig @192.168.50.1 version.bind chaos txt`: `status: NOTIMP`, so nmap's
   "Cloudflare public DNS" was its guess from a reply header (`nmap-service-probes` line 12811). A DNS product with no version is now shown as "(guess)"
   (`Port.ProductGuessed`, `nmap/parse.go`, `finding.go`, `portchecks.go`; the auditor prompt line 17). Nothing left except seeing it in the next real audit
   report: the router's port 53 should read "Cloudflare public DNS (guess)" and the model should not call it Cloudflare.
3. **The #207 helper results: partly answered.** On the router (2026-11-08) both parsed: UDP 53, 67, 1900, 5353 open, 69 `open|filtered`; OS detection "Linux 3.10 - 4.11
   (100%)". **Still needed:** one result from a device with none of ports 22, 80, 443 open (a phone or smart TV): does OS detection say "No guess" plainly? The 2026-11-08
   vertex audit was a checks-mode run and did not cover this.
4. **Approve (or change) the nightly UDP probe list and Constitution item 12** (`network-nightly-udp-probes.md`); then build its slices 1 to 3.
5. **Run `capture_window` measurements M1 to M3** on vertex and decide go or no-go (`network-capture-window-plan.md`).
6. **Delete the stray remote branch** `network-privileged-helper-n1-design`.
7. **Phase 3 (Router DNS check) data** was due about 2026-11-08: bring back "Copy the results"; a finding only if there were no false alarms, else drop the phase.

## Next phases of the stacks work (each its own branch and PR; re-measure section 4b each time)

- **S3:** the pure data leaves with handlers: `vulnindex`, `vulnfeed`, `vulnmatch`, `ubuntuoval`, `upnp`, `webprobe`, `dnswire`, `names`, `nearby`. Reuse the
  S2 ports; add one only with a reason written in the PR. Needs the `Config` port to stay merge-by-fresh-value (do not decode into the stored value).
- **S4:** ingest-fed leaves (`routerlog`, `knownbad`, `phonehome`, then `mcast` users): needs an `Ingest(datagram)` port; the core keeps the sockets (D6).
- **S5:** programs (`nmap`, `portprobe`, `scantool`, `wifilook`): the isolation port and tools in the manifest; the helper work must not be combined with it.
- **S6:** the root `netaudit` and `decoy`; delete `migrationDebt`; make the settings document opaque and schema-checked (closes the "a stack may name any
  field" limit in ADR-0035); move the registry's Network modules into the manifest (today `app/stacks.go` still defines them; a test pins DNS to the manifest).
- **P4 harvest** (architecture map, hubs, SPEC-006, pitfalls) after S6. The README rows for #209 and the ADR index were updated in this PR.

## Left on purpose in #209

- `outsidelookup` stays in the core (its client reads `core/tools` types and the root `netaudit` still uses it); the stack has its own eight-line https URL check.
  No new `migrationDebt` pair was added.
- No settings cache: the stack reads the document when needed, so `ConfigChanged` is a no-op and no save path notifies it.
- A switched-off DNS collector looks at its settings again only every 30 s (`idlePoll`), as before.
- The 13 non-test importers outside `platform/` moved to `isolation`; `platform/privhelper/client` and the `progexec`, `isolation` and `archtest` files still import the lower packages on purpose.

## Still open from earlier

- OS detection port list: decide from real results (a code change that passes review).
- D9 (a spawn with no workspace gets a deny-all profile, `landlock_provider_linux.go:163`; SPEC-006 update), D8 second half (service non-dumpable), N2d/N2e: unchanged.
  nuclei through the helper: not proposed, needs the owner to accept relaxing the memory rule for the helper's own unit (ADR-0029).
- The registry still holds a stale baseline entry for `discoverModelFiles` in `tools/check-complexity` (it says "remove after refactor"); unrelated to this work.

## Traps (all also in `docs/learnings/network-audit.md`)

- Do not stash, reset or restore to compare files: AGENTS.md forbids it.
- The new nmap text on `models.NetworkDevice` must never reach a prompt; re-check if any consumer serialises a whole device.
- Inventory findings are restricted to devices scanned that night; `ProbeFindings` deliberately is not. A UDP port with no reply is `open|filtered`, never `open`.
- A shell command with `cat > file` and no heredoc waits on stdin and stalls the tool.
- A stack's tests cannot import `llm-proxy/internal/...`; give them fakes of the ports (`stacks/network/network_test.go` is the pattern).
- The DNS cycle a setting refuses (`lan_off`, `internet_off`) logs no row by design.
- `gofmt -l` lists `backend/internal/app/bootstrap.go`, `netaudit.go` and four handler test files as unformatted before this work; do not reformat them in an unrelated change.
