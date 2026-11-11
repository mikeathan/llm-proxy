---
status: partial
date: 2026-11-12
last_reviewed: 2026-11-12
related_specs: [SPEC-003]
constitution_references: [I.8, I.14, I.15]
evidence: PR #230 (v0.77.0); docs/PLANS/cross-cutting/network-traffic-by-device-redesign.md; ADR-0045 to ADR-0048; Constitution I.15; the owner's decisions of 2026-11-12 (option D; stored history yes; nightly read yes, with no way for an agent to breach the security layer).
---

# Network · Traffic by device and the Router connections module (handoff)

Replaces `network-traffic-more-handoff-2026-11-11.md` (trace any address, other devices' lookups, the router's table): all of it is built, and what was left
of it is in "What is left" below. Read this file first, then `network-traffic-by-device-redesign.md` for the design and the owner's mockup choice.

## State

- **Branch and PR:** `traffic-by-device-and-router-watch`, **PR #230**, two commits on top of `main` (9c1aa6cf, #229). Pushed. The owner merges and deploys it.
  Nothing else depends on it and nothing waits behind it. Merging is the owner's approval of the **Constitution I.15 amendment** (read the I.15 text again: it
  changed in the second commit).
- **Version:** `VERSION` is 0.77.0. `backend/internal/transport/http/frontend_dist/index.html` is committed with the built bundle, as the owner asks.
- **Built (v0.76.0, frontend only):** the Traffic page by device (option D): `TrafficDeviceRail`, `TrafficDevicePane`, `TrafficPlaceGroups`, `TrafficGroupPlaces`,
  `TrafficDeviceNames`, `TrafficRouterSetup`, `TrafficHeadline`; the shared `PlaceRow` model in `frontend/src/utils/netaudit/places.ts`. The map now draws the
  router's places with no look at this computer; a route opens under a place. `TrafficOtherDevices`, `TrafficRouterConn`, `TrafficLookups`, `mapDots` are gone.
- **Built (v0.77.0):** the module **Router connections** (step-up, off by default, ready only when the router's setup is finished): `core/netaudit/routerhistory`
  (bounded history of companies and countries, `router-history.json`), `RunReporter.addRouterConn` (the audit's own code reads the router once, held to **one read per
  three hours** by `readGate`), findings `router_new_company` / `router_new_country`, "New today" on the page, `MonitorRouterConn` on the Modules tab. ADR-0048,
  SPEC-003 2.65. `TestOnlyTheServiceCodeReachesTheRouter` (`internal/archtest`) keeps agents' packages from importing `routerssh`, `routerconn` and `routerhistory`.
- **Checks run on the PR's code:** `go test ./...`, `go vet ./...`, `GOOS=linux go build ./...`, `go run ./tools/check-complexity/`, `-race` on `routerhistory`,
  `routerconn`, `netaudit`; `npm run build` (lint, contrast, `vue-tsc`), `npx vitest run` (2,987), `npm run test:visual` (368), `./scripts/check-agent-harness.sh`.
  After the last commit only the app, archtest and netaudit Go packages and the harness were re-run; the full Go suite was last run before it.
- **Not run:** anything against the real router or on vertex. No CI result was read.

## What is left, in order

1. **Merge and deploy.** After the deploy, hard-reload the Network page (Cmd+Shift+R): a page opened before keeps the old bundle. Check the version:
   `curl -s -H "Authorization: Bearer $LLM_PROXY_TOKEN" http://vertex.local:4001/admin/api/version` (the token is the admin token; the UI is under `/admin/`).
2. **Look at Network → Traffic on vertex** (the first real run of the page and of the owner's router read). *Owner:* if not done, set up the router (Traffic tab, "The router"
   card: address, user, a key made for this alone, check and trust the router's fingerprint), press **Read the router now**. Expect the router's devices in the list, a map and
   company groups for the busiest one, and a Route under a place. If the router refuses an ed25519 key, try RSA (`ssh-keygen -t rsa -b 3072`). Also press **Listen now** once to see this
   computer's page. Note anything that reads wrong; the first thing to check is a hard reload.
3. **Switch on Router connections** (Modules tab; asks the passphrase; it says "Finish the router's setup" until the setup is done). Press Read now. A device's first day only
   learns, so *New today* and the two findings can appear only after about 24 hours and an audit run that made a scan. *Owner:* judge whether the 24-hour learning day is right and
   how noisy the findings are; both are constants in `routerhistory` (`LearnFor`, `NewFor`) and the read gap is `routerConnReadGap` in `internal/app/netaudit_routerconn.go`.
4. **Decide on limiting the read to runs under the auditor profile.** Today the reporter reads at the end of *any* automation run that made a scan, held to one per three hours.
   Limiting it to the auditor profile needs the profile passed through `ExecuteRequest` to `Augment`. Defence in depth, not a known hole.
5. **Optional, only if the owner wants them:** make the agent see tonight's router data during its own run (read and build the router findings at the start of the run, in code, not
   through a tool); a one-line footprint per device and an assistant tool for it; per-hop latency timeline; byte counts (needs `nf_conntrack_acct` on, which the stock firmware loses
   on reboot); the ~260 KB spikes every 10 s seen in looks (measure which page poll it is before deciding).

## Traps

- **A new module needs five places that tests pin:** the layer map and `modulesWithNoOldRoute` in `internal/app/stacks_test.go`, `stacks_about.go` (one plain sentence of at most 220
  characters per field), `moduleFinds`, and `newTestStacks`.
- **The composables `useRouterConnections`, `useDeviceNames`, `useDeviceLookups` make new state on every call.** `TrafficTab` calls each once and provides the router one
  (`ROUTER_CONN_INJECTION`); a part that called its own would never agree with the list.
- **Component tests run with `matchMedia` false**, so the pane is in its phone layout (`isMobile`); a wide-layout test stubs `window.matchMedia` for the `1024` query.
- **Known-bad matching by address is impossible:** the abuse.ch list holds names only (its host file maps names to loopback). Recorded in ADR-0048; do not propose it again.
- **`Augment` returns early when the run made no scan**, so a night with no scan makes no read and no router findings.
- **Anything that asks "is this address mine?" must use `Scope.LocalAddrs`** (pitfall 65); `net.InterfaceAddrs` fails on vertex.
- **The city table at full point precision costs 134 MB to build;** keep points at 0.1°. A broadcast to a subnet address is known only by its hardware destination.
- **Colima's user-mode NAT drops ICMP answers past hop 1:** test traces on vertex, not in Docker on the Mac.
- **`ls` is aliased to eza here:** never use `$(ls ...)` in a script. `grep --include=*.ts` fails in zsh unless the glob is quoted.

## First step

Open PR #230's description, then `docs/PLANS/cross-cutting/network-traffic-by-device-redesign.md` (the status block at the top), then run step 1 above.
