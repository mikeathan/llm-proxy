---
status: built
date: 2026-11-02
last_reviewed: 2026-11-02
related_specs: [SPEC-003]
constitution_references: []
evidence: Recon of 2026-11-02 (§2) and the owner-approved mockup Option A ("Quiet"; the three HTML mockups were deleted after the build, so the shipped UI is the reference). Built and tested on 2026-11-02; not yet merged or deployed.
---

# Modules group and a calmer Network page

**Status:** built (2026-11-02), branch `modules-group-network-redesign`, uncommitted. The owner chose mockup Option A, the group
name **Modules**, and moving the network scan settings onto the Network page, then approved the branch, the SPEC-003 change
(2.24, ADR-0022) and status groups (§6). What differs from the plan is in *What was built*.
**Related plans:** [`network-guard-module.md`](network-guard-module.md) (built the Network page this reworks).

## 1. Goal and non-goals

**Goal.** (a) A generic **Modules** group in the navigation that Network lives under now and later modules (smart home,
messaging, memory views, …) can join. (b) A Network page that stays readable with many devices and on a phone: grouped,
searchable device list; neutral finding cards; one place for the page's settings; a Simple/Advanced split that means the same
on every tab.

**Not in this plan:** new scans, new backend endpoints, a new colour palette (§3), moving Messaging or Memory into Modules, the
Integrations/Messaging placeholders as pages. Backend is untouched.

## 2. Recon (verified in the repo)

- Navigation is one list, `frontend/src/utils/navigation.ts` (`SPECS`, `buildNavigation`), read by `AppSidebar.vue` and
  `BottomTabs.vue` (phone: four tabs + a More sheet from the entries with `tab: false`). Types: `NavId`, `Destination`,
  `NavItem` in `types/routes.ts`.
- Network page: `views/NetworkView.vue`, tabs from `NETWORK_TABS = ['devices','findings']` (`types/routes.ts:52`), overview is the
  page without a tab; route `/network/:tab(...)?` in `router/index.ts:62`.
- The scan settings live in Settings → Safety → Network scan: `components/settings/NetworkScanSettings.vue`, tab id
  `network-scan` (`domain/settings.ts`, `constants/settings.ts`, `types/admin.ts`, `views/SettingsView.vue:227`). Devices-page
  cards (`DevicesCard`, `DecoysCard`, `RouterLogCard`, `OutsideLookupCard`, `DnsCheckCard`) are already on the Network page.
- Simple/Advanced is `useNetauditViewMode`, shown by `NetauditToolbar.vue`; SPEC-003 §II.6 already defines it.
- SPEC-003 fixes the navigation (eight entries, bottom tab bar) and says Settings → Network scan keeps what may be scanned
  (changelog 2.21): both change, so this is a SPEC-level change (SPEC-change-management).

## 3. Decisions

1. **Group name: Modules**, expandable in the sidebar; on a phone it is a heading in the More sheet. Network is its first
   child. No placeholder children are shipped (nothing is "coming soon" in the product).
2. **URLs stay** (`/network/...`). No `/modules` page until there are several children.
3. **Colour: existing tokens only.** The mockup's teal is a mock; the frontend rule bans new palettes and raw colours
   (`styles/tokens.css`, `no-palette-classes`). The calm look comes from structure: neutral surfaces, state colour only as a
   small dot or text, no thick coloured left borders (today `SEVERITY_CARD_CLASS` / `border-l-4` on every card).
4. **Settings move into the Network page** as a Settings tab; the Settings → Network scan tab is removed, not redirected (the
   product is new; no compatibility shims). Every link that points to it (`toSettings('network-scan')`, the `#scan-time`
   anchor) is repointed.
5. **Simple / Advanced** (one rule for every tab):
   - Simple: names not addresses, one verdict, one button per decision, only settings that matter; nothing is lost, only folded.
   - Advanced adds in place: MAC and IPv6, open-port lists, presence interval (Devices); full flaw list, compared software,
     fix commands, Accept with note and expiry (Findings); run history with durations (Audits); scan time, confinement,
     extra ranges, Add another audit (Settings).
   - One remembered choice, one toggle in the page header, same page layout in both.

## 4. Phases (each ends with its proof; the risky one first)

| # | Step | Verify |
|---|---|---|
| 1 | **Navigation model**: a group entry (`Modules`) with children in `utils/navigation.ts`; sidebar renders it expandable and remembers it (`usePersistedState`); More sheet shows the heading; `current` is true for the group when a child is. Update `navigation.test.ts`, add sidebar/bottom-tab tests | `npm test` · `npm run build` |
| 2 | **Network shell**: tabs Overview, Devices, Findings, Audits, Settings (`NETWORK_TABS`, route, `NetworkTabs`); header holds the Simple/Advanced toggle and Help; Audits tab = `AuditList` + run controls; Settings tab = the content of `NetworkScanSettings` plus the Devices-page switches (presence, names, IPv6, decoys, router log, outside lookup, DNS check, weekly summary, browser alerts) grouped as in the mockup; remove the Settings tab and repoint links | `npm test` · build · component tests for the tabs and the repointed links |
| 3 | **Devices**: search, filter chips (all / needs a look / new / online / offline / unnamed), groups by type (collapsible), one row per device that expands in place; phone: row shows name, address, status | component tests (filter, search, expand, new device never hidden) · `test:visual` |
| 4 | **Findings and Overview**: neutral cards with a small severity marker; verdict card with one next action; sparklines kept | tests + visual |
| 5 | **Simple/Advanced pass** against §3.5, a test per tab proving Advanced only adds and Simple loses nothing | tests |
| 6 | **Mobile and visual pass** at 390 px (44 px targets), visual baselines updated, SPEC-003 changelog 2.24, hub and README row, ADR for the Modules group (settled choice) | `npm run test:visual` · `./scripts/check-agent-harness.sh` |

Device types (computer, phone, TV, …) and the brand column in the mockup have no source. **Verified:** `NetworkDevice`
(`frontend/src/types/netaudit.ts:341`) has mac, ip, ipv6, name, suggested_name, first_seen, last_seen, approved, randomized, new,
open_ports, ports_scanned_at: no type, no vendor. So Phase 3 groups by status (*Needs a look / Online / Offline*) and shows
name, address and last seen; a type or vendor field is a separate backend change that needs its own approval.

## 5. Risks

- The mockup shows device type and brand; the app has neither (verified, §4). The built page will look plainer than the mockup there.
- Moving 60 KB of Settings UI (`NetworkScanSettings`, ScanSettingsFields) risks losing the `autosave` behaviour; keep the
  components, move only where they are mounted.
- The visual-regression baselines for Network and Settings change on purpose.

## 6. Needs approval before code

1. A branch for this work (git mutation; not yet created).
2. SPEC-003 change: navigation gains a group, the Network page gains Audits and Settings tabs, Settings → Network scan is removed.
3. Decision: accept status groups instead of type groups (§4, verified: no type or vendor is stored), or approve a separate backend change for them.

## What was built (2026-11-02) and how it differs from the plan

All six phases, frontend only (the backend is untouched). Gates run: `npm test` (2528 tests), `npm run build` (lint, type check, theme contrast), `npm run test:visual` (222 checks, layout and touch-target assertions; the `/design` baselines are unchanged).

1. **Navigation** — `groupNavigation` in `utils/navigation.ts`, `SidebarLink.vue`, a group button in `AppSidebar.vue` (open by default, fold persisted as `sidebar-closed-groups`, forced open while a child is current, a collapsed rail lists children flat), a heading in the More sheet. New icon `nav-modules.svg`.
2. **Network shell** — five tabs, one Simple/Advanced toggle and Help in the page header, `AuditsPanel.vue`, `NetworkSettings.vue` (composes `ScanScopeSettings.vue`, `DeviceCheckSettings.vue`, and the decoy, router-log, internet-view and DNS cards, the weekly summary and the browser notices). Settings → Network scan removed; `toRequirementFix` maps the server's `network-scan` to the Settings tab. `NetworkScanSettings.vue` was moved (not copied) to `components/netaudit/ScanScopeSettings.vue`.
3. **Devices** — `DevicesCard.vue` (search, chips, status groups), `DeviceRow.vue`, pure logic in `utils/netaudit/devices.ts` (`deviceStatus`, `groupDevices`, `filterCounts`, `matchesQuery`).
4. **Calm** — one `FINDING_CARD_CLASS` (neutral surface) replaces `SEVERITY_CARD_CLASS` and `PLAIN_CARD_CLASS`; no thick coloured edges on findings, audits, the scan banner or deep-check rows; the verdict card has one next step (`nextAction`).
5. **Simple/Advanced** — a test per tab that Advanced only adds (Devices, Findings, Audits, Settings, Overview); Advanced adds the recent runs with durations to the Audits list.
6. **Docs** — SPEC-003 2.24, ADR-0022, the hub, this row, the ledger.

**What the mockup showed (Option A "Quiet"; the HTML files were deleted after the build, so this is the record).** The Settings page was four titled cards, the same in Simple and Advanced, each setting one row of title, one-line hint and a switch: *What may be scanned*; *Network audit*; *Watching for changes* (new devices, device names, IPv6, weekly summary); *Optional extras* (decoys, router log, internet view, DNS check); then *Vulnerability data*; and in Advanced a fifth card, *Advanced* (time allowed per device, confinement). The Settings tab is built in that order, with *Watching for changes* and *Optional extras* as group headings and *Advanced* as a heading over the scan limits; nothing is folded in Simple, as in the mockup. The Devices page was a search box, chips, groups with a count and an online count, a row of avatar, name, brand, address, ports and a tag, and an opened row with address, MAC, open ports, a "needs a look" reason and the buttons *It's mine*, *Rename*, *Expected ports*, *Remove*.

Differences from the plan, each deliberate:
- **Five filter chips, not six.** The server's `new` already means "not approved and seen in the last week" (`newDeviceIsNews`), which is what Needs a look is, so a separate New chip would show the same list.
- **"Online" is a fixed 30-minute window** (`DEVICE_ONLINE_WINDOW_MS`): the server keeps when a device was last seen, not whether it is there. With the new-device check off the list says it is only as fresh as the last scan. An unapproved device not seen for a week is Offline, not "Needs a look", as on the server.
- **The IPv4/IPv6 tabs are gone**: an IPv6-only device is listed by its IPv6 address (all of them in Advanced).
- **Add another audit is on the Audits tab**, next to the list it extends, not on Settings (one place per job).
- **The presence interval is next to its switch on the Settings tab (Advanced)**, not on the Devices tab: the interval changes the check, and the Devices tab holds no control that changes how the network is watched.
- **The mockup's "Network audit" card (nightly audit, known-flaw check, safe checks) is not on the Settings tab.** Those are the audit's own settings, set in the ordinary automation form that the Audits tab opens; copying them would give two places for one setting.
- **Device rows show name, address and status; the open ports are inside the opened row**, on desktop too (the mockup had a ports column and a brand line; the brand has no source, see below).

## Remaining Work

- The owner's look at the built pages on a real server (nothing here was run against vertex), then commit, PR and deploy. The built `backend/internal/transport/http/frontend_dist/` is rebuilt and uncommitted.
- `VERSION` is still 0.38.0; a minor bump (as past network PRs did) waits for the owner's yes.
- Backend text still says "(Settings, Network scan)" in the report note at `backend/internal/core/netaudit/knownvulns.go:183`; changing it is a backend edit, so it was left.
- A device type or vendor (the mockup's grouping and brand column) needs a backend source; not started.
