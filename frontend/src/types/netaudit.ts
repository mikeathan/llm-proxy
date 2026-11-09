// Network scan (plan network-audit): the shapes the /admin/api/netaudit endpoints return and take.
import type { SettingsTab } from './admin'
import type { NetworkTab } from './routes'

/** Simple shows plain sentences and what needs a look; Advanced shows ports, versions and the full list. */
export type NetauditViewMode = 'simple' | 'advanced'

/** What a requirement is about; the server names them, the UI picks the wording around them. */
export type RequirementCode = 'scope_empty' | 'host_network_off' | 'nmap_missing' | 'no_sandbox' | 'vuln_data_missing' | 'vuln_data_old'

/** Where a requirement's fix lives: a Settings section, or `network-scan`, which the server still names and the Network page's own Settings tab now holds. */
export type RequirementFixSection = SettingsTab | 'network-scan'

/** The one step that meets a requirement. */
export type RequirementFix =
  | { kind: 'open_settings'; section: RequirementFixSection }
  | { kind: 'copy'; text: string }
  | { kind: 'confirm'; action: string }

export interface Requirement {
  code: RequirementCode
  message: string
  /** A blocking requirement hides the scan tool; the others only limit it. */
  blocking: boolean
  fix?: RequirementFix
}

export interface Readiness {
  ready: boolean
  requirements: Requirement[]
  /** Improvements that never stop an audit (the vulnerability data not being downloaded); never change `ready`. */
  notices?: Requirement[]
}

/** One allowed range, in canonical CIDR form, with how many devices it covers. */
export interface ScanRange {
  cidr: string
  devices: number
}

/** A network this machine is attached to, offered as a one-click range. */
export interface ScanSuggestion {
  interface: string
  cidr: string
  devices: number
}

export interface ScanScope {
  ranges: ScanRange[]
  allow_unconfined: boolean
  /** Minutes a scan may spend on one device before giving up on it; 0 is the standard. */
  device_timeout_minutes: number
  suggestions: ScanSuggestion[]
  /** Why the machine's networks could not be listed; the saved ranges are still returned. */
  suggestions_error?: string
}

/** A change to the scope; a field left out stays as it is. */
export interface ScopeUpdate {
  entries?: string[]
  allow_unconfined?: boolean
  device_timeout_minutes?: number
}

/** One fact about a device, with whether the latest scan found it new and whether it has been accepted. */
export interface ScanFinding {
  /** What the server accepts a finding by. */
  key: string
  device: string
  port?: number
  proto?: string
  service?: string
  version?: string
  check: string
  /** A few plain words naming the check, so one heading can cover every place it was found; absent for an open port. */
  check_title?: string
  severity: string
  evidence?: string
  /** A known_vulns finding: every CVE id (sorted), the ones CISA lists as exploited, and each one's base score. */
  cves?: string[]
  exploited?: string[]
  cve_scores?: Record<string, number>
  /** What to do about a check's problem: fixed text the server wrote. Nothing is changed on the device for you. */
  fix?: string
  /** The longer form of the fix (steps, commands to copy, risk, undo): fixed text the server owns; absent for an open port. */
  guide?: FixGuide
  /** What the operator told us: they applied the fix. `marked` waits for the next scan; `still_present` means a scan since still shows it. */
  fix_state?: FixState
  fix_marked_at?: string
  fix_note?: string
  /** Set on a problem taken from a stored deep check that no audit has carried into the report yet: it has no key the server accepts, so it can be read but not accepted. */
  deep_at?: string
  new: boolean
  /** The version differs from the scan before; `previous_version` is what it was (it may be empty). */
  changed: boolean
  previous_version?: string
  accepted: boolean
  accepted_note?: string
  accepted_until?: string
}

/** A finding as it was, listed under "resolved" because the next scan no longer showed it. */
export interface ResolvedFinding {
  device: string
  /** What kind of finding it was; absent or "open_port" for a plain open port. */
  check?: string
  evidence?: string
  port?: number
  proto?: string
  service?: string
  version?: string
  /** When the operator marked it fixed, if they had and this scan confirms it is gone. */
  fixed_at?: string
}

export interface FindingsReport {
  scanned: boolean
  scanned_at?: string
  engine?: string
  findings: ScanFinding[]
  resolved: ResolvedFinding[]
  /** Devices the scan ran out of time on: their ports are unknown, not "none". */
  unscanned: string[]
  /** How many addresses answered. */
  devices_up: number
  /** Nothing was scanned before this: the open ports are the baseline, not changes. Absent in a report from an older server. */
  first_scan?: boolean
}

/** What the report shows, sorted from the findings: the parts the filter tiles count. */
export interface ReportSummary {
  /** The scan is the baseline: no open port in it is new, and only problems need a look. */
  baseline: boolean
  needLook: ScanFinding[]
  /** Checks that found something and nobody has accepted, new or not: they stay until dealt with. */
  problems: ScanFinding[]
  added: ScanFinding[]
  changed: ScanFinding[]
  accepted: ScanFinding[]
  resolved: ResolvedFinding[]
  /** The unaccepted problems, one entry per check with every place it was found, worst first. */
  problemGroups: ProblemGroup[]
}

/** One kind of problem and everywhere the latest scan found it. */
export interface ProblemGroup {
  check: string
  title: string
  /** The worst severity among its items. */
  severity: string
  /** The suggested fix, written once for the whole group. */
  fix: string
  items: ScanFinding[]
}

/** The problems of one device, worst first; `severity` is the worst of them. */
export interface DeviceProblems {
  device: string
  severity: string
  items: ScanFinding[]
}

/** How the problems are laid out: one card per kind of problem, or one per device. */
export type ProblemLayout = 'check' | 'device'

export interface DeviceGroup<T> {
  device: string
  items: T[]
}

/** Which part of the Advanced report is listed. */
export type FindingFilter = 'all' | 'problems' | 'new' | 'changed' | 'accepted' | 'resolved'

export interface ScanAck {
  key: string
  accepted_version?: string
  note?: string
  accepted_at: string
  expires_at: string
}

export interface AcceptRequest {
  workspace_id: string
  automation: string
  key: string
  note: string
  days: number
}

/** How hard an audit scans: quick checks the common ports in seconds; thorough looks harder and takes minutes. */
export type ScanDepth = 'quick' | 'thorough'

/** What a network audit scans and how; kept with the automation and applied by the server, not by the model. */
export interface ScanSettings {
  /** Addresses, ranges or names; empty or absent means every network allowed in Settings. */
  targets?: string[]
  depth?: ScanDepth
  /** A port list such as "22,80,8000-8100"; empty means the common ports. */
  ports?: string
  checks?: boolean
  /** Look for known vulnerabilities in the software the scan recognised; absent means on. */
  known_vulns?: boolean
}

/** The parts of the Help panel; a warning opens it on the one that explains it. */
export type NetauditHelpTopic = 'basics' | 'setup' | 'reports' | 'vulns' | 'devices' | 'nearby' | 'fixes' | 'deep' | 'safety' | 'words'

export interface HelpSection {
  title: string
  paragraphs: readonly string[]
}

export interface RequirementCopy {
  /** The plain sentence the Simple view shows; Advanced shows the server's message. */
  title: string
  simple: string
  help: NetauditHelpTopic
}

/** What a failed audit run needs from the person, in one place. */
export type FailureFix = 'network-settings' | 'edit-automation' | 'none'

export interface FailureExplanation {
  title: string
  body: string
  fix: FailureFix
  /** The allowed networks help here: the run asked for something outside them. */
  showsAllowedNetworks?: boolean
}

/** A target the audit would refuse, with why in plain words; `entry` is absent when the whole list is the problem. */
export interface TargetProblem {
  entry?: string
  reason: string
}

/** Where the vulnerability-data download stands; the server derives it from what it holds, so it survives a restart. */
export type VulnDataState = 'idle' | 'running' | 'stopped' | 'done'

/** One data source (CISA's exploited list, or NVD by year) as the index holds it. */
export interface VulnSource {
  /** A zero date (year 1) means the source was never imported. */
  imported_at: string
  cves: number
  entries: number
  /** CVEs the last file's import left out, by reason. */
  no_config?: number
  needs_all?: number
  rejected?: number
  /** NVD's yearly files held; empty for the exploited list. */
  years?: number[]
}

export interface VulnDataStatus {
  state: VulnDataState
  /** The file being fetched, while running. */
  file?: string
  files_done: number
  files_total: number
  bytes_done?: number
  bytes_total?: number
  /** Why a download stopped, in plain words. */
  reason?: string
  nvd: VulnSource
  kev: VulnSource
}

/** The two files the vulnerability data is made of: NVD's yearly file and CISA's exploited list. */
export type VulnImportSource = 'nvd' | 'kev'

/** What the card shows and offers for a status. */
export type VulnDataAction = 'download' | 'resume' | 'cancel' | 'update'

export interface VulnDataSummary {
  line: string
  /** The one button to offer; null when there is nothing to do (the data is held and recent). */
  action: VulnDataAction | null
  /** Held but older than the age the card calls old. */
  old: boolean
}

/** One line of the Advanced card: a data source with when and how much it was imported. */
export interface VulnSourceRow {
  name: string
  detail: string
  /** What the import left out of the last file; absent when it left nothing out. */
  skipped?: string
}

/** One flaw of a known_vulns finding as listed; the score is null when the index gave none. */
export interface VulnFlaw {
  id: string
  score: number | null
  exploited: boolean
}

/** A command to copy for one kind of system; it is shown, never run. */
export interface GuideCommand {
  platform: string
  command: string
}

export interface FixGuide {
  steps: string[]
  commands?: GuideCommand[]
  risk: string
  undo: string
}

export type FixState = 'marked' | 'still_present'

/** Tells the server the operator applied the fix for one finding of an automation's latest scan. */
export interface FixRequest {
  workspace_id: string
  automation: string
  key: string
  note: string
}

/** What the operator says a device should offer: the TCP ports that may be open on it (none means nothing should answer). */
export interface DeviceExpectation {
  device: string
  ports: number[]
  note?: string
  set_at: string
}

/** Sets, or replaces, what one device should offer. */
export interface ExpectationRequest {
  workspace_id: string
  device: string
  ports: number[]
  note: string
}

/** One port a deep check looked at, and whether a TLS or an SSH service answered on it (so something could be judged). */
export interface DeepPort {
  port: number
  tls_answered: boolean
  ssh_answered: boolean
}

/** The last deep check of one device: when it ran, what answered, and what it found (each finding with its title, fix and guide). */
export interface DeepCheck {
  device: string
  at: string
  ports: DeepPort[]
  findings: Array<Pick<ScanFinding, 'device' | 'port' | 'proto' | 'service' | 'check' | 'check_title' | 'severity' | 'evidence' | 'fix' | 'guide'>>
}

/** What a deep check came to: problems found, a service judged and clean, or no TLS or SSH service to judge at all. */
export type DeepOutcomeState = 'problems' | 'clean' | 'nothing'

/** One device seen on the network, by its hardware (MAC) address. */
export interface NetworkDevice {
  mac: string
  /** The IPv4 address; absent for a device only seen over IPv6. */
  ip?: string
  /** IPv6 addresses, newest first. */
  ipv6?: string[]
  /** What the person calls it; empty until they name it. */
  name?: string
  /** The maker of the network card, from the IEEE registry bundled with the program; absent for a private address or an unlisted prefix. */
  vendor?: string
  /** What the device called itself when asked; a hint to confirm, shown only while the person has not named it. */
  suggested_name?: string
  first_seen: string
  last_seen: string
  approved: boolean
  /** A private hardware address, as phones and laptops use to hide the real one. */
  randomized: boolean
  /** Not approved and seen lately: the same test that makes it a finding. */
  new: boolean
  /** The person marked it as one they care most about; the attack paths treat it as a prize. */
  important?: boolean
  /** Online and has asked the router's DNS for nothing for a day: a note with harmless reasons, never a finding. */
  dns_silent?: boolean
  /** The ports it had open at its last completed scan, and when; absent until a scan has finished looking at it. */
  open_ports?: number[]
  ports_scanned_at?: string
  /** The last UDP scan and OS detection the person ran on it through the privileged helper, and when. The time is set even when nothing answered. */
  udp_ports?: UdpPort[]
  udp_scanned_at?: string
  os_guesses?: OsGuess[]
  os_detected_at?: string
}

/** The router's hardware address and whether it is the one on record. */
export interface DeviceGateway {
  ip?: string
  mac?: string
  changed: boolean
}

/** The devices seen on the network, those needing a look first. */
export interface DeviceInventory {
  devices: NetworkDevice[]
  gateway: DeviceGateway
  baselined: boolean
  updated_at: string
}

/** The presence check: a light look every few minutes for a device that was not there before. */
export interface PresenceState {
  enabled: boolean
  interval_minutes: number
  allowed_intervals: number[]
  /** When the last check finished; absent before the first. */
  last_check?: string
  /** Plain words for why the check cannot look; never "nobody is there". */
  problem?: string
}

export interface PresenceUpdate {
  enabled: boolean
  interval_minutes: number
}

/** Why active IPv6 discovery did not work: the computer's setting forbids the echo, it cannot (not Linux), or it failed. */
export type IPv6DiscoveryProblem = 'not_permitted' | 'unavailable' | 'failed'

/** Active IPv6 discovery: one echo per look so the IPv6 devices on the link are listed. */
export interface IPv6DiscoveryState {
  enabled: boolean
  /** A code the screen turns into words and a fix; absent when it works or is off. */
  problem?: IPv6DiscoveryProblem
}

/** Where a device sits in the list: needs a decision, was on the network lately, or was not. */
export type DeviceStatus = 'attention' | 'online' | 'offline'

/** The chips above the device list. */
export type DeviceFilter = 'all' | 'attention' | 'online' | 'offline' | 'unnamed'

export interface DeviceListGroup {
  status: DeviceStatus
  devices: NetworkDevice[]
}

/** What the person did to a device. */
export type DeviceAction = 'name' | 'approve' | 'important' | 'forget' | 'dismiss-suggestion' | 'confirm-gateway'

/** One fake service the decoys offer. */
export interface DecoyPort {
  number: number
  name: string
}

/** A device that connected to a decoy: who, which decoy, how often. No bytes are kept. */
export interface DecoyTouch {
  source: string
  port: number
  first: string
  last: string
  count: number
}

/** The router log listener: the switch, whether it listens, any trouble, and when the router was last heard. Nothing the router said is kept. */
export interface RouterLogState {
  enabled: boolean
  listening: boolean
  problem?: string
  last_heard?: string
  port: number  /** The DNS query lines since the listener started: how many were read, how many its limit turned away, the most in one second. */
  queries_read?: number
  queries_limited?: number
  queries_peak_per_second?: number
  queries_since?: string
}

/** An access point's security, reduced to one word by the server. */
export type WifiSecurity = 'open' | 'wep' | 'wpa' | 'wpa2' | 'wpa3' | 'other'
/** An access point the last look heard advertising one of the operator's network names. */
export interface WifiSeen { bssid: string; signal: number; security: WifiSecurity; confirmed: boolean }
/** One of the operator's network names, the access points they confirmed as theirs, and what the last look heard. */
export interface WifiNetwork { name: string; confirmed: string[]; seen: WifiSeen[] }
/** The Wi-Fi look-alikes card: the switch lives on the Modules tab; `ready` is false, with `why`, while the look cannot run. */
export interface WifiState { enabled: boolean; ready: boolean; why?: string; last_look?: string; problem?: string; networks: WifiNetwork[] }

/** What the last look at the internet-facing ports did. `outcome` is absent until one was tried; `at` is the server's zero date then. */
export type OutsideLookupOutcome = 'looked' | 'cannot_look' | 'failed'
export interface OutsideLookupLast {
  at: string
  outcome?: OutsideLookupOutcome
  /** A fixed code from the server (for example `private_address`); the words for it are in constants/netaudit. */
  reason?: string
  ports: number
  /** Where the address of a look came from: the router's own statement, or the one the operator entered. */
  source?: 'router' | 'entered'
  /** True when the router states a public address other than the one entered, so the entered one may be out of date. */
  differs?: boolean
}

/** The outside lookup: the switch, which service it asks, and what the last look did. */
export interface OutsideLookupState {
  enabled: boolean
  url: string
  default_url: string
  /** The public address the operator entered, or empty. */
  address: string
  last: OutsideLookupLast
}

/** A change to the outside lookup: either field alone. An empty `url` goes back to the default service. */
export interface OutsideLookupUpdate {
  enabled?: boolean
  url?: string
  /** An empty `address` clears the one entered. */
  address?: string
}

/** One name asked of the router's DNS and of an outside resolver. Addresses are parsed addresses; no answer text is kept. */
export type DnsCheckOutcome = 'match' | 'differ' | 'cannot_compare'
export interface DnsCheckComparison {
  at: string
  name: string
  outcome: DnsCheckOutcome
  /** A fixed code for why nothing could be compared. */
  reason?: string
  /** True when the two answers still shared an address. */
  overlap?: boolean
  router?: string[]
  outside?: string[]
}

/** The log boiled down: what to bring back after a week. Dates are the server's zero date when there is none. */
export interface DnsCheckSummary {
  comparisons: number
  matches: number
  differs: number
  cannot_compare: number
  overlapping: number
  first: string
  last: string
  differences: DnsCheckComparison[]
}

/** The DNS check: its settings, the latest cycle and the summary. `problem` is a fixed code (`internet_off`, `lan_off`). */
export interface DnsCheckState {
  enabled: boolean
  interval_minutes: number
  allowed_intervals: number[]
  names: string[]
  default_names: string[]
  /** The router's DNS address; empty means the default gateway. */
  server: string
  resolver_url: string
  default_resolver_url: string
  /** True when a difference that keeps showing is also raised as a finding in the nightly report. */
  finding_enabled?: boolean
  status: { last_run: string; problem?: string }
  summary: DnsCheckSummary
}

/** When the DNS check last ran, in words, and whether it looks stopped (nothing for over twice the interval). */
export interface DnsFreshness {
  text: string
  late: boolean
}

/** One differing comparison written for a person: both address lists as text, and whether they still shared an address. */
export interface DnsDifferenceRow {
  key: string
  name: string
  when: string
  router: string
  outside: string
  partly: boolean
}

/** A change to the DNS check; any field alone. An empty `names` list or `resolver_url` goes back to the default. */
export interface DnsCheckUpdate {
  enabled?: boolean
  interval_minutes?: number
  names?: string[]
  server?: string
  resolver_url?: string
  finding_enabled?: boolean
}

/** The decoy services: the switch, which ports listen, any trouble, and who touched them. */
export interface DecoyState {
  enabled: boolean
  listening: number[]
  problem?: string
  ports: DecoyPort[]
  touches: DecoyTouch[]
}

/** One action of the privileged helper: the operator's switch and whether the install's own list names it (null when no helper is installed). */
export interface HelperAction {
  id: string
  on: boolean
  installed: boolean | null
}

/** Where the privileged helper stands (GET /admin/api/helper): installed or not, ready or not and why, the declared scan ranges, and the command to run. */
export interface HelperStatus {
  supported: boolean
  installed: boolean
  ready: boolean
  not_ready?: string
  actions: HelperAction[]
  declared_ranges: string[]
  install_command: string
  help_path: string
}

/** One UDP port a helper scan found. "open|filtered" means no reply came back, so the port may be open or blocked. */
export interface UdpPort {
  port: number
  state: 'open' | 'open|filtered'
  service?: string
}

/** One of nmap's operating-system guesses, with the accuracy it reported (0 to 100). */
export interface OsGuess {
  name: string
  accuracy: number
}

/** What a UDP scan or an OS detection of one device found, as the service's own parser read it. */
export interface HelperProbe {
  udp_ports: UdpPort[]
  os: OsGuess[]
}

/** The helper actions the page can run on one device. */
export type ProbeAction = 'scan_udp' | 'os_detect'

/** The answer of a helper route: the parsed result, or why there is none. The raw bytes the server also sends are not used here. */
export interface HelperRunResult {
  parsed?: HelperProbe
  parse_error?: string
}

/** The one word at the top of the Network page. */
export type NetworkPosture = 'unknown' | 'quiet' | 'attention'

/** What one day looked like: counts only (the server's models.NetworkDay). */
export interface NetworkDay {
  /** YYYY-MM-DD, in the host's local time. */
  date: string
  devices_up: number
  devices: number
  unnamed: number
  findings: number
  serious: number
  /** True once an audit ran that day, so a day without one is not drawn as "no findings". */
  audited?: boolean
  /** Connection attempts from the internet that the router's log said it refused that day, and the ports most tried. */
  blocked?: number
  blocked_ports?: PortCount[]
}

export type NetworkChangeKind = 'new_device' | 'address_changed'

/** One thing that changed lately; the screen composes the words. */
export interface NetworkChange {
  kind: NetworkChangeKind
  at: string
  mac?: string
  ip?: string
  name?: string
}

export type ActivityKind = 'new_device' | 'address_changed' | 'alert' | 'ports_changed' | 'audit' | 'related' | 'router_blocks'
export type ActivityGroup = 'devices' | 'ports' | 'findings' | 'alerts' | 'router'
export type ActivityFilter = 'all' | ActivityGroup

/**
 * One row of the Activity timeline (the server's netaudit.ActivityRow); the screen composes the words from the kind and
 * its typed fields. `text` is only on an alert row: the alert's own wording, which the program wrote.
 */
export interface ActivityRow {
  kind: ActivityKind
  group: ActivityGroup
  at: string
  /** True for a row that stands for a whole day; its date is shown and not its time. */
  day?: boolean
  mac?: string
  ip?: string
  name?: string
  alert_kind?: string
  text?: string
  count?: number
  opened?: number
  closed?: number
  /** The ports most tried on a router_blocks day, busiest first. */
  ports?: PortCount[]
}

/** How many times a destination port was tried. */
export interface PortCount {
  port: number
  count: number
}

/** The router card: today's refused attempts and busiest ports, and the last days' totals (the server's netaudit.RouterBlocks). */
export interface RouterBlocks {
  today: number
  today_ports?: PortCount[]
  days: { date: string; total: number }[]
}

/** The model's plain-words summary of the timeline (the server's summaryResponse); empty when there was nothing to explain. */
export interface TimelineSummary {
  summary: string
  model?: string
}

/** Rows of the timeline that fall on one day, under that day's heading. */
export interface ActivityGroupOfRows {
  heading: string
  rows: ActivityRow[]
}

/** What the timeline draws for a day: a row as it is, or several new devices folded into one line. */
export type ActivityItem =
  | { kind: 'row'; row: ActivityRow }
  | { kind: 'fold'; rows: ActivityRow[]; text: string }

/** The Activity timeline, newest first. */
export interface NetworkActivity {
  rows: ActivityRow[]
  /** Present only once the router's log has been counted on some day. */
  router?: RouterBlocks
}

/** What the program holds of Canonical's security notices for this computer's Ubuntu release (the server's ubuntuoval.Status). */
export interface UbuntuDataStatus {
  /** False on a computer that is not Ubuntu: there is nothing to fetch for it. */
  supported: boolean
  host_release?: string
  running: boolean
  release?: string
  notices: number
  packages: number
  generated_at?: string
  imported_at?: string
  reason?: string
}

/** The Network page's first screen (the server's netaudit.Overview). */
export interface NetworkOverview {
  posture: NetworkPosture
  devices: { known: number; unnamed: number; new: number; active: number }
  /** `open` counts the problems the checks found that are not accepted (what the Findings tab lists); `ports` the open ports. */
  findings: { scanned: boolean; scanned_at?: string; open: number; serious: number; ports?: number; new: number }
  gateway_changed: boolean
  shared_address: boolean
  presence: { last_check?: string; problem?: string }
  series: NetworkDay[]
  recent: NetworkChange[]
  /** The few ways in that matter, best first; empty while there are none or the module is off. */
  attack_paths?: AttackPath[]
  /** Whether the attack paths module is on, so the card can tell "nothing found" from "switched off". */
  attack_paths_on?: boolean
}

/** The list of malware host names (abuse.ch URLhaus): how many are held, when, whether an address is saved (never the address), and whether the comparison can work. */
export interface KnownBadState {
  count: number
  loaded_at?: string
  has_address: boolean
  ready: boolean
  /** In plain words, why it cannot work yet (no address saved, list not downloaded). */
  reason?: string
}

export type AttackPathEntryKind = 'internet' | 'network'
export type AttackPathPrizeKind = 'computer' | 'router' | 'important'

export interface AttackPathStep {
  key: string
  check: string
  title: string
  port?: number
  severity: string
  /** The person accepted this finding: it stays in the path, quietly. */
  accepted?: boolean
}

export interface AttackPathPrize {
  kind: AttackPathPrizeKind
  /** `this-computer`, or the device's address. */
  where: string
  /** What the person calls an important device; shown as text. */
  name?: string
}

/** One way in to one prize: where it starts, the real findings it passes through, what it ends at, and the one fix. */
export interface AttackPath {
  id: string
  entry: { kind: AttackPathEntryKind; text: string; probing?: boolean }
  steps: AttackPathStep[]
  prize: AttackPathPrize
  fix: { key: string; text: string }
  /** How many of the listed paths the fix breaks, itself included. */
  breaks: number
  all_accepted?: boolean
}

/** The one thing the verdict card suggests doing next, and the Network tab it happens on. */
export interface NetworkNextAction {
  label: string
  tab: NetworkTab
}

/** The audit whose last result the overview counts. */
export interface NetworkAudit {
  workspaceId: string
  automation: string
}

/** One on/off setting of the network feature: the weekly summary, asking devices their names. */
export interface NetauditSwitch {
  enabled: boolean
}

/** The outcome of the latest Nearby look: a fixed code, never text a device sent. */
export type NearbyOutcome = 'heard' | 'nothing_heard' | 'off' | 'lan_off' | 'no_interface' | 'refused' | 'unavailable' | ''

/** One problem with a device that said what it is: a fixed sentence and the guide that fixes it. */
export interface NearbyProblem {
  check: string
  title: string
  text: string
  guide?: FixGuide
}

/**
 * One device that answered a Nearby question. Every text field (names, maker, model) came from the device: show it as plain
 * text only, never as markup, and never put it in a message of our own.
 */
export interface NearbyDevice {
  ip: string
  names?: string[]
  services?: string[]
  ports?: number[]
  upnp_type?: string
  maker?: string
  model?: string
  gateway?: boolean
  sources?: string[]
  first_heard: string
  last_heard: string
  /** From the device list, when it knows this address. */
  mac?: string
  name?: string
  known: boolean
  /** The device list's own "new" flag. */
  new: boolean
  problem?: NearbyProblem
}

export interface NearbyState {
  enabled: boolean
  running: boolean
  last_look?: string
  last_outcome?: NearbyOutcome
  devices: NearbyDevice[]
}

/** What a device says it is, from its service types; it is a claim, so the screen says "advertises as". */
export type NearbyKind = 'printer' | 'tv' | 'camera' | 'computer' | 'gateway' | 'other'

/** Where a device sits after the latest look: has a problem, is new, was heard in that look, or was not. */
export type NearbyStatus = 'attention' | 'new' | 'heard' | 'quiet'

export type NearbyFilter = 'all' | 'attention' | 'new' | 'quiet'

/** What one capture_window run saw: typed counts of broadcast and multicast frames (the server's models.CaptureSummary). A snapshot of one link, not a finding. */
export interface CaptureSummary {
  at: string
  interface: string
  wireless: boolean
  stopped_by: 'window' | 'frame_limit' | 'cut' | 'interface_missing' | 'interface_down' | 'socket_error' | 'unsupported'
  elapsed_ms: number
  frames: number
  /** Frames that were copies of one message heard moments before (an access point repeating a broadcast); the counts below leave them out. */
  repeated_copies: number
  max_per_second: number
  kernel_seen: number
  kernel_dropped: number
  counts: {
    arp_requests: number; arp_replies: number; arp_gratuitous: number; dhcp_from_server: number; dhcp_from_client: number; mdns: number
    nd_router_solicit: number; nd_router_advert: number; nd_neighbor_solicit: number; nd_neighbor_advert: number; nd_redirect: number; unreadable: number
  }
  claims: { mac: string; ip: string; gratuitous: boolean; off_network: boolean }[]
  dhcp_server_macs: string[]
  conflicting_addresses: number
  /** Announced addresses outside every range this link has itself, 0 when the link's own ranges are unknown. */
  off_network_addresses: number
}

/** The answer of the capture route: the parsed summary, or why there is none. */
export interface CaptureRunResult {
  parsed?: CaptureSummary
  parse_error?: string
}

/** What one look at this computer's own traffic saw. Names are text a peer chose, so they appear only here, on this view. */
export interface HostTraffic {
  at: string
  interface: string
  stopped_by: CaptureSummary['stopped_by']
  elapsed_ms: number
  frames: number
  unreadable: number
  kernel_seen: number
  kernel_dropped: number
  /** 1 means every frame was counted; N means one ordinary frame in N was kept and the byte figures are estimates. */
  sample: number
  bytes_in: number
  bytes_out: number
  bytes_through: number
  internet_bytes: number
  local_network_bytes: number
  lookups: number
  kinds: TrafficKind[]
  per_second: number[]
  destinations: TrafficDestination[]
  flows: TrafficFlow[]
  other_flows: number
  other_bytes: number
}

/** One kind of traffic, as a fixed word. */
export type TrafficKindWord = 'https' | 'http' | 'quic' | 'dns' | 'ssh' | 'ntp' | 'discovery' | 'icmp' | 'tcp' | 'udp'

export interface TrafficKind {
  kind: TrafficKindWord
  bytes: number
}

export interface TrafficDestination {
  name?: string
  address: string
  bytes: number
  internet: boolean
}

export interface TrafficFlow {
  address: string
  name?: string
  port: number
  proto: 'tcp' | 'udp' | 'icmp'
  kind: TrafficKindWord
  internet: boolean
  bytes_out: number
  bytes_in: number
}

/** The answer of the host traffic route: the parsed summary, or why there is none. */
export interface HostTrafficRunResult {
  parsed?: HostTraffic
  parse_error?: string
}

/** Why the traffic look cannot be pressed now, and where the fix is. */
export interface TrafficAvailability {
  ok: boolean
  reason: string
  /** The page that fixes it, when there is one. */
  fixTab?: NetworkTab
}

/** One bar of the per-second chart, in the chart's own units. */
export interface ChartBar {
  x: number
  y: number
  width: number
  height: number
  second: number
  bytes: number
}
