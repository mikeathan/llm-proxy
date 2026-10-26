// Network scan (plan network-audit): the shapes the /admin/api/netaudit endpoints return and take.
import type { SettingsTab } from './admin'

/** Simple shows plain sentences and what needs a look; Advanced shows ports, versions and the full list. */
export type NetauditViewMode = 'simple' | 'advanced'

/** What a requirement is about; the server names them, the UI picks the wording around them. */
export type RequirementCode = 'scope_empty' | 'host_network_off' | 'nmap_missing' | 'no_sandbox'

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
  severity: string
  evidence?: string
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
}

/** What the report shows, sorted from the findings: the parts the filter tiles count. */
export interface ReportSummary {
  needLook: ScanFinding[]
  /** Checks that found something and nobody has accepted, new or not: they stay until dealt with. */
  problems: ScanFinding[]
  added: ScanFinding[]
  changed: ScanFinding[]
  accepted: ScanFinding[]
  resolved: ResolvedFinding[]
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
}

/** The parts of the Help panel; a warning opens it on the one that explains it. */
export type NetauditHelpTopic = 'basics' | 'setup' | 'reports' | 'safety' | 'words'

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
