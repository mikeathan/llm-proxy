# Decisions (ADRs)

Architecture Decision Records: **why** the system is the way it is. One short file per decision.

- **Immutable once `accepted`.** Do not rewrite an accepted ADR to change the decision — write a new ADR that
  `supersedes` it and set `superseded_by` on the old one. Fixing a typo or a dead link is fine.
- **Small.** Context, decision, consequences, alternatives, links — one screen. Detail stays in the plan, PR or
  audit the ADR links to.
- **When to write one:** a choice between real alternatives that a later contributor would otherwise re-open —
  an architecture or contract choice, a "we will not do X", a tooling or process rule. Plans record decisions
  while work is in flight; when the plan's phase merges, the decision moves here (see the harvest rule in
  [`../PLANS/README.md`](../PLANS/README.md)).
- **Naming:** `ADR-NNNN-<kebab-slug>.md`, next free number, from [`TEMPLATE.md`](TEMPLATE.md); add a row below.
  `subsystems:` uses the slugs in [`../subsystems/README.md`](../subsystems/README.md).

| ADR | Title | Status | Date | Subsystems |
|-----|-------|--------|------|------------|
| [ADR-0001](ADR-0001-harness-neutral-agent-instructions.md) | Harness-neutral agent instructions | accepted | 2026-10-26 | agent-harness |
| [ADR-0002](ADR-0002-docs-split-by-lifecycle.md) | Docs split by lifecycle, linked by subsystem | accepted | 2026-10-26 | agent-harness |
| [ADR-0003](ADR-0003-natural-completion.md) | Completion is a content-only final answer | accepted | 2026-07-21 | agent-loop |
| [ADR-0004](ADR-0004-explicit-primary-and-fallback-model.md) | No model auto-bootstrap; explicit primary and fallback | accepted | 2026-08-14 | orchestrator, admin-ui |
| [ADR-0005](ADR-0005-no-llm-rewrite-on-the-run-path.md) | No extra LLM call on the run path to apply memory | accepted | 2026-06-03 | memory |
| [ADR-0006](ADR-0006-cloud-helper-sub-agents.md) | Helper sub-agents run in the cloud, depth 1, from their own pool | accepted | 2026-10-22 | agent-loop |
| [ADR-0007](ADR-0007-no-nmap-in-ci.md) | No nmap in CI; verify network scans by hand | accepted | 2026-10-26 | network-audit |
| [ADR-0008](ADR-0008-terminal-blocks-a-curated-set.md) | The terminal guard blocks a curated sensitive set, not every dot-directory | accepted | 2026-08-20 | guardrails |
| [ADR-0009](ADR-0009-prompt-profiles-over-a-global-agents-layer.md) | Prompt profiles instead of a global AGENTS.md prompt layer | accepted | 2026-10-17 | prompt-profiles |
| [ADR-0010](ADR-0010-no-level-2-assisted-fixes.md) | Assisted fixes are guided, not executed by the app | accepted | 2026-10-29 | network-audit |
| [ADR-0011](ADR-0011-nvd-kev-local-vulnerability-data.md) | Known-vulnerability matching uses local NVD and KEV data, never an online lookup per scan | accepted | 2026-10-29 | network-audit |
| [ADR-0012](ADR-0012-intrusive-scripts-only-in-the-deep-check.md) | Intrusive scripts run only in an interactive, step-up deep check of one device | accepted | 2026-10-29 | network-audit |
| [ADR-0013](ADR-0013-vulnerability-data-import-is-step-up.md) | Importing vulnerability data is a step-up route | accepted | 2026-10-29 | network-audit |
| [ADR-0014](ADR-0014-presence-check-and-device-inventory.md) | The presence check is a model-free background loop over one app-wide device inventory | accepted | 2026-10-29 | network-audit |
| [ADR-0015](ADR-0015-decoy-listeners-low-interaction-and-step-up.md) | Decoy services are low-interaction listeners, off by default, switched by a step-up route | accepted | 2026-10-30 | network-audit |
| [ADR-0016](ADR-0016-active-ipv6-discovery-one-echo-unprivileged.md) | Active IPv6 discovery sends one echo through the unprivileged ICMP socket, off by default | proposed | 2026-10-30 | network-audit |
| [ADR-0017](ADR-0017-on-link-ipv6-scanning-keyed-by-mac.md) | The audit scans on-link IPv6-only devices, keyed by hardware address, once the Constitution is amended | proposed | 2026-10-31 | network-audit |
| [ADR-0018](ADR-0018-router-log-only-wakes-the-presence-check.md) | The router's log may only wake the presence check early; nothing in it is kept, shown or acted on | proposed | 2026-11-01 |
| [ADR-0019](ADR-0019-outside-lookup-is-opt-in-and-fails-open-to-unknown.md) | The outside lookup of open ports is opt-in, sends only the router's stated public address, and never reads a failure as "nothing is open" | accepted | 2026-11-01 | network-audit |
| [ADR-0020](ADR-0020-dns-check-only-records-and-never-reads-silence-as-agreement.md) | The DNS check only records, compares sets over DNS-over-HTTPS, and a missing answer is never read as agreement | accepted | 2026-11-01 | network-audit |
| [ADR-0021](ADR-0021-an-entered-public-address-stands-in-when-the-router-gives-none.md) | An entered public address stands in when the router gives none, and the router's own answer wins | accepted | 2026-11-01 | network-audit |
| [ADR-0022](ADR-0022-modules-is-the-navigation-group-for-feature-areas.md) | Modules is the navigation group for self-contained feature areas, and a page keeps its own settings | accepted | 2026-11-02 | admin-ui, network-audit |
| [ADR-0023](ADR-0023-network-ideas-rejected-or-already-built.md) | Network detection ideas that were weighed and rejected, parked, or found already built | accepted | 2026-11-02 | network-audit |
| [ADR-0024](ADR-0024-nearby-discovery-two-multicast-datagrams-bounded-replies.md) | Nearby discovery sends two fixed multicast datagrams and reads a bounded set of unicast replies, off by default (proposed, with Constitution I.9) | proposed | 2026-11-03 | network-audit |
| [ADR-0025](ADR-0025-network-timeline-reads-existing-records-summary-is-local-and-typed.md) | The Network timeline reads records the program already keeps, and its model summary is local-only and typed-fields-only | accepted | 2026-11-04 |
| [ADR-0026](ADR-0026-program-files-are-compared-with-the-checksums-dpkg-keeps.md) | Packaged program files are compared with the checksums dpkg keeps, and a pass that is cut short never resolves a finding | accepted | 2026-11-04 |
| [ADR-0027](ADR-0027-installed-packages-are-judged-against-canonicals-usn-oval.md) | Installed packages are judged against Canonical's per-release USN OVAL file, with Pro-only fixes left out | accepted | 2026-11-04 |
| [ADR-0028](ADR-0028-stack-module-registry.md) | Switchable capabilities are modules in stacks, described by one registry and switched in one place | accepted | 2026-11-05 |
| [ADR-0029](ADR-0029-the-service-keeps-memory-deny-write-execute-so-nuclei-is-removed.md) | The service keeps MemoryDenyWriteExecute, so the nuclei web checks are removed and built in | accepted | 2026-11-06 | network-audit |
| [ADR-0030](ADR-0030-failed-logins-are-read-from-btmp-with-the-utmp-group.md) | Failed logins to this computer are read from btmp, with the utmp group, and only their times | proposed | 2026-11-06 | network-audit |
| [ADR-0031](ADR-0031-the-wifi-look-is-a-guarded-nmcli-with-no-network-and-only-unix-sockets.md) | The Wi-Fi look-alike check runs nmcli as a guarded program with no network and only Unix sockets, after the jail's Unix-socket gap is closed | proposed | 2026-11-06 | network-audit, guardrails |
| [ADR-0032](ADR-0032-a-stack-is-a-package-with-a-manifest-kept-apart-by-tests.md) | A stack is a package with a manifest, kept apart by tests, and Network is the first one moved out of the main source | proposed | 2026-11-06 | network-audit, platform |
| [ADR-0033](ADR-0033-the-sandbox-is-core-and-stacks-gain-no-way-around-it.md) | The sandbox is core and cannot be swapped or configured off, and a stack gains no way around it | proposed | 2026-11-06 | guardrails, network-audit, platform |
| [ADR-0034](ADR-0034-the-privileged-helper-is-core-not-a-stack-and-decides-who-is-calling.md) | The privileged helper is core, not a stack, and it decides who is calling | proposed | 2026-11-07 | network-audit, guardrails, platform |
| [ADR-0035](ADR-0035-a-stack-reaches-the-service-only-through-named-ports-and-keeps-only-the-files-it-lists.md) | A stack reaches the service only through named ports and keeps only the files it lists | proposed | 2026-11-08 | network-audit |
| [ADR-0036](ADR-0036-a-stack-offers-findings-by-id-and-the-core-words-them.md) | A stack offers findings by id and the core words them | accepted | 2026-11-09 | network-audit |
| [ADR-0037](ADR-0037-where-the-vulnerability-and-ubuntu-feeds-live-when-network-becomes-a-stack.md) | Where the vulnerability and Ubuntu feeds live when Network becomes a stack | accepted | 2026-11-09 | network-audit |
| [ADR-0038](ADR-0038-the-network-stack-owns-the-sensors-and-the-core-keeps-the-judgement.md) | The Network stack owns the sensors and the core keeps the judgement | accepted | 2026-11-09 | network-audit |
| [ADR-0039](ADR-0039-how-the-listening-and-discovering-sensors-reach-the-network-stack.md) | How the listening and discovering sensors reach the Network stack | proposed | 2026-11-09 | network-audit |
| [ADR-0040](ADR-0040-the-traffic-look-is-its-own-helper-action-with-its-own-limits.md) | The traffic look is its own helper action with its own limits | proposed | 2026-11-09 | network-audit |
| [ADR-0041](ADR-0041-testssl-and-osquery-are-looks-with-typed-results-and-one-home-for-each-fact.md) | testssl.sh and osquery are guarded looks whose results are typed, and each fact has one home | proposed | 2026-11-09 |
| [ADR-0042](ADR-0042-the-footprint-lives-in-the-core-and-its-findings-are-keyed-on-a-label.md) | The footprint lives in the core, and its findings are keyed on a label, not a device | accepted | 2026-11-10 | network-audit |
| [ADR-0043](ADR-0043-the-traffic-look-reads-a-tls-server-name-and-names-the-holder-of-an-address-on-this-computer.md) | The traffic look reads a TLS server name and names the holder of an address on this computer | proposed | 2026-11-11 | network-audit |
| [ADR-0044](ADR-0044-a-city-table-is-an-index-file-searched-on-disk-never-held-in-memory.md) | A city table is an index file searched on disk, never held in memory | proposed | 2026-11-11 | network |
| [ADR-0045](ADR-0045-a-route-trace-is-unprivileged-udp-probes-on-the-operators-press.md) | A route trace is unprivileged UDP probes on the operator's press, for an address the last look saw | proposed | 2026-11-11 | network |
| [ADR-0046](ADR-0046-the-traffic-view-has-a-map-of-cities-drawn-from-a-built-in-outline.md) | The Traffic view has a map of cities, drawn from an outline built into the page | proposed | 2026-11-11 | network |
| [ADR-0047](ADR-0047-other-devices-connections-come-from-the-routers-connection-table-over-ssh.md) | Other devices' connections come from the router's connection table, read over SSH on the operator's press | proposed | 2026-11-12 | network |
| [ADR-0048](ADR-0048-the-router-watch-keeps-a-history-and-the-audit-reads-the-router-in-its-own-code.md) | The router watch keeps a history of companies and countries, and the audit's own code reads the router once a night (proposed, with the I.15 amendment) |
| [ADR-0049](ADR-0049-the-audit-assessment-reads-a-bounded-window-digest-attached-to-the-scan-result.md) | The audit's assessment reads a bounded window digest that the scan result carries |
| [ADR-0050](ADR-0050-a-finding-carries-typed-details-and-the-host-watch-keeps-file-names.md) | A finding carries typed details, and the host watch keeps file names with a short sum for each |
| [ADR-0051](ADR-0051-the-agents-stack-takes-the-watch-and-the-core-keeps-the-walk-the-words-and-the-gate.md) | The Agents stack takes the watch; the core keeps the folder walk, the words and the gate |
| [ADR-0052](ADR-0052-radio-is-its-own-stack-with-generic-sensors-and-the-core-keeps-capture-and-judgement.md) | Radio is its own stack with generic sensors, and the core keeps the capture and the judgement |
| [ADR-0053](ADR-0053-radio-keeps-a-keyed-tag-for-other-peoples-devices-and-seals-the-owners-with-hard-size-and-age-limits.md) | Radio keeps a keyed tag for other people's devices and seals the owner's, with hard size and age limits |
| [ADR-0054](ADR-0054-a-scheduled-privileged-action-runs-under-a-bounded-session-a-person-approved.md) | A scheduled privileged action runs under a bounded session a person approved | accepted | 2026-11-13 |
| [ADR-0055](ADR-0055-radio-may-keep-a-short-signal-track-per-device-on-disk-only-when-the-owner-turns-it-on.md) | Radio may keep a short signal track per device on disk, only when the owner turns it on | accepted | 2026-11-14 |
| [ADR-0056](ADR-0056-radio-may-watch-for-up-to-seven-days-under-one-session-and-a-stopped-watch-is-offered-again-never-resumed-by-itself.md) | Radio may watch for up to seven days under one session, and a stopped watch is offered again, never resumed by itself | accepted | 2026-11-15 |
| [ADR-0057](ADR-0057-radio-keeps-the-maker-and-wi-fi-generation-of-a-device-next-to-its-tag-and-a-network-name-in-memory-only.md) | Radio keeps the maker and Wi-Fi generation of a device next to its tag, and a network name in memory only | accepted | 2026-11-15 |
| [ADR-0058](ADR-0058-radio-names-a-device-from-what-it-broadcasts-learns-its-class-and-joins-the-network-inventory-by-tag.md) | Radio names a device from what it broadcasts, learns its class from the owner's answers, and joins the network inventory by tag | proposed | 2026-11-17 |
| [ADR-0059](ADR-0059-radio-watch-reads-the-mesh-routers-client-list-and-listens-between-sweeps.md) | A radio watch listens again between sweeps on the owner's channels and reads the mesh router's list of joined devices | proposed | 2026-11-18 |
| [ADR-0060](ADR-0060-the-radio-helper-thins-repeated-control-frames-before-its-frame-cap.md) | The radio helper thins repeated control frames before its frame cap | proposed | 2026-11-18 |
| [ADR-0061](ADR-0061-network-and-radio-facts-are-joined-by-tag-in-the-core.md) | Network and Radio facts about one device are joined by tag in the core | accepted | 2026-11-18 |
| [ADR-0062](ADR-0062-presence-reads-the-routers-list-of-joined-devices.md) | Presence reads the router's list of joined Wi-Fi devices, so a dozing phone stays online | proposed | 2026-11-18 |
| [ADR-0063](ADR-0063-one-list-of-the-owners-wi-fi-networks-and-one-look-alike-check.md) | One list of the owner's Wi-Fi networks, owned by the core, and one look-alike check over both sensors | proposed | 2026-11-18 |
| [ADR-0064](ADR-0064-the-dashboard-shows-and-links-and-the-stack-pages-control.md) | The dashboard shows and links; the stack pages control | accepted | 2026-11-19 |
| [ADR-0066](ADR-0066-stack-pages-keep-only-control-and-the-dashboard-holds-everything-to-see-and-decide.md) | The stack pages keep only control; the dashboard holds everything to see and to decide about an item, for a stack that is on | accepted | 2026-11-19 |
| [ADR-0067](ADR-0067-a-stack-is-switched-off-by-switching-its-modules-off.md) | A stack is switched off by switching all of its modules off; it has no setting of its own | accepted | 2026-11-19 |
| [ADR-0068](ADR-0068-reminders-are-a-stack-with-its-own-store-a-notify-port-and-a-schedule-port.md) | Reminders are a stack: it keeps its own store, sends the owner's own words through a notify port, and schedules through a schedule port | proposed | 2026-11-19 |
| [ADR-0065](ADR-0065-dashboard-card-height-is-fixed-order-is-seen-order-and-stack-cards-follow-the-stack.md) | A dashboard card's height is a minimum, its order is the order seen, and a stack's cards are offered while the stack is on | accepted | 2026-11-19 |
| [ADR-0069](ADR-0069-mcp-is-a-general-client-in-the-agents-stack-with-dynamic-tools-and-per-server-trust.md) | MCP is a general client in the Agents stack: tools are dynamic, every server has its own trust, and the old node-herder code is rewritten away | proposed | 2026-11-19 |
| [ADR-0070](ADR-0070-network-is-one-dashboard-tab-with-sub-tabs-one-card-per-question-and-settings-grouped-by-what-they-watch.md) | Network is one dashboard tab with sub-tabs, one card per question, and the settings are grouped by what they watch | proposed | 2026-11-19 |
| [ADR-0071](ADR-0071-a-stack-may-ask-the-core-for-a-model-answer-through-a-narrow-model-port-with-no-tools-no-memory-and-no-choice-of-model.md) | A stack may ask the core for a model answer through a narrow model port: no tools, no memory, no choice of model | proposed | 2026-11-20 |
| [ADR-0072](ADR-0072-a-stack-hands-the-core-a-structured-notice-and-the-core-words-and-sends-it.md) | A stack hands the core a structured notice and the core words and sends it | proposed | 2026-11-20 |
| [ADR-0073](ADR-0073-the-newsroom-follows-watches-one-object-that-says-where-to-read-and-what-to-look-for.md) | The Newsroom follows watches, one object that says where to read and what to look for | proposed | 2026-11-20 |
| [ADR-0074](ADR-0074-the-watch-form-looks-a-source-up-before-it-is-saved-through-two-keyless-reads-of-the-newsroom-stack.md) | The watch form looks a source up before it is saved, through two keyless reads of the Newsroom stack | proposed | 2026-11-21 |
| [ADR-0081](ADR-0081-a-written-brief-is-read-or-unread-on-the-server-and-the-dashboard-card-shows-and-links-only.md) | A written brief is read or unread on the server, one poller feeds every dot, and the dashboard card shows and links only | proposed | 2026-11-23 |
| [ADR-0085](ADR-0085-the-service-counts-model-calls-per-day-and-per-model-in-one-small-file-and-the-brief-shows-them-under-home.md) | The service counts model calls per day and per model in one small file, and the brief shows them under Home | proposed | 2026-11-24 |
| [ADR-0086](ADR-0086-the-newsroom-opens-on-the-brief-and-every-card-is-a-front-page-that-links-into-it.md) | The Newsroom opens on the brief, and every card is a front page that links into it | proposed | 2026-11-25 |
| [ADR-0087](ADR-0087-left-out-alerts-stay-in-the-journal-with-their-own-bound-and-a-deleted-brief-keeps-its-slot.md) | Left-out alerts stay in the journal with their own bound, and a deleted brief keeps its slot | proposed | 2026-11-25 |
| [ADR-0088](ADR-0088-a-watch-learns-from-the-owners-verdicts-as-a-short-list-of-example-titles-sent-with-what-it-looks-for.md) | A watch learns from the owner's verdicts as a short list of example titles sent with what it looks for | proposed | 2026-11-25 |
| [ADR-0089](ADR-0089-a-stacks-page-is-for-setting-it-up-and-everything-to-read-see-or-decide-is-on-its-dashboard-tab.md) | A stack's page is for setting it up; everything to read, see or decide about an item is on its dashboard tab, with no exception for reading | accepted | 2026-11-25 |
| [ADR-0084](ADR-0084-the-home-block-ranks-its-facts-by-a-level-the-core-sets-and-folds-when-nothing-needs-a-look.md) | The Home block ranks its facts by a level the core sets and folds when nothing needs a look | proposed | 2026-11-24 |
| [ADR-0083](ADR-0083-your-day-reads-the-owners-reminders-through-a-read-only-day-port-and-a-written-brief-goes-to-a-phone-as-a-short-summary-the-core-words.md) | Your day reads the owner's reminders through a read-only Day port, and a written brief goes to a phone as a short summary the core words | proposed | 2026-11-24 |
| [ADR-0082](ADR-0082-one-story-reworded-is-one-candidate-and-an-outlet-under-two-spellings-is-one-outlet.md) | One story reworded is one candidate, and an outlet under two spellings is one outlet | proposed | 2026-11-24 |
| [ADR-0078](ADR-0078-a-brief-section-is-a-category-that-gathers-the-topics-sharing-it.md) | A Brief section is a category that gathers the topics sharing it | proposed | 2026-11-23 |
| [ADR-0080](ADR-0080-the-briefs-page-is-laid-out-like-the-brief-sections-hold-topics-and-an-edition-picks-sections-watches-and-home.md) | The Briefs page is laid out like the brief: sections hold topics, and an edition picks sections, watches and home | proposed | 2026-11-23 |
| [ADR-0079](ADR-0079-the-brief-topic-form-asks-for-one-thing-and-fills-the-rest-from-starters.md) | The Brief topic form asks for one thing and fills the rest from starters | superseded | 2026-11-23 |
| [ADR-0077](ADR-0077-the-brief-reads-counts-and-fixed-words-about-the-home-network-and-radio-through-a-read-only-home-port.md) | The Brief reads counts and fixed words about the home network and Radio through a read-only Home port (proposed) |
| [ADR-0076](ADR-0076-the-briefs-topic-form-offers-a-built-in-list-of-well-known-sources-and-the-owners-own-list-and-copies-the-pick-into-the-topic.md) | The Briefs topic form offers a built-in list of well-known sources and the owner's own list, and copies the pick into the topic | proposed | 2026-11-22 |
| [ADR-0075](ADR-0075-the-newsroom-brief-module-collects-headlines-through-the-day-with-no-model-and-writes-an-edition-once.md) | The Newsroom Brief module collects headlines through the day with no model and writes an edition once | proposed | 2026-11-21 |
