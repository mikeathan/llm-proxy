// Network scan (plan network-audit): the shapes the /admin/api/netaudit endpoints return and take.
import type { SettingsTab } from './admin'

/** Simple shows plain sentences and what needs a look; Advanced shows ports, versions and the full list. */
export type NetauditViewMode = 'simple' | 'advanced'

/** What a requirement is about; the server names them, the UI picks the wording around them. */
export type RequirementCode = 'scope_empty' | 'host_network_off' | 'nmap_missing' | 'no_sandbox' | 'vuln_data_missing' | 'vuln_data_old'

/** The one step that meets a requirement. */
export type RequirementFix =
  | { kind: 'open_settings'; section: SettingsTab }
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
export type NetauditHelpTopic = 'basics' | 'setup' | 'reports' | 'vulns' | 'devices' | 'fixes' | 'deep' | 'safety' | 'words'

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
  /** What the device called itself when asked; a hint to confirm, shown only while the person has not named it. */
  suggested_name?: string
  first_seen: string
  last_seen: string
  approved: boolean
  /** A private hardware address, as phones and laptops use to hide the real one. */
  randomized: boolean
  /** Not approved and seen lately: the same test that makes it a finding. */
  new: boolean
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

/** Which address a device list is read by: a device with both addresses is on both tabs. */
export type DeviceFamily = 'ipv4' | 'ipv6'

/** What the person did to a device. */
export type DeviceAction = 'name' | 'approve' | 'forget' | 'confirm-gateway'

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

/** The decoy services: the switch, which ports listen, any trouble, and who touched them. */
export interface DecoyState {
  enabled: boolean
  listening: number[]
  problem?: string
  ports: DecoyPort[]
  touches: DecoyTouch[]
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

/** The Network page's first screen (the server's netaudit.Overview). */
export interface NetworkOverview {
  posture: NetworkPosture
  devices: { known: number; unnamed: number; new: number; active: number }
  findings: { scanned: boolean; scanned_at?: string; open: number; serious: number; new: number }
  gateway_changed: boolean
  shared_address: boolean
  presence: { last_check?: string; problem?: string }
  series: NetworkDay[]
  recent: NetworkChange[]
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
