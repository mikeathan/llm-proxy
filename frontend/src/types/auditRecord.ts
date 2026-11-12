/** The audit's typed record, kept as audit.json beside a run's report.md (the server's netaudit.AuditRecord). Version 1. */
export interface AuditRecord {
  version: number
  audit: string
  scanned_at: string
  /** When the scan before was made; absent for a first scan. */
  previous_scan_at?: string
  engine: string
  scanned: number
  up: number
  counts: AuditRecordCounts
  vulns: AuditRecordVulns
  exposed: AuditRecordExposed[]
  window?: AuditRecordWindow
  notes: string[]
  /** The model's words (Markdown), or "". */
  assessment: string
}

export interface AuditRecordCounts {
  new: number
  changed: number
  resolved: number
  accepted: number
  open_ports: number
  /** Every check finding not accepted; `by_severity` splits it. */
  open: number
  by_severity: Record<string, number>
}

/** How far known-vulnerability matching reached; `enabled` false means it was not asked for. */
export interface AuditRecordVulns {
  enabled: boolean
  skipped?: boolean
  total: number
  checked: number
  coarse: number
  uncomparable: number
}

export interface AuditRecordExposed {
  device: string
  port: number
  proto: string
  service: string
  detail?: string
  kind: string
}

export interface AuditRecordRange {
  min: number
  max: number
}

/** A finding that appeared, went away or came back inside the window. `check` is the check's plain title. */
export interface AuditRecordChange {
  device: string
  port?: number
  check: string
  at: string
  came_back?: number
}

export interface AuditRecordModule {
  title: string
  noun: string
  flagged_word: string
  done: number
  flagged: number
  unclear: number
}

export interface AuditRecordWindow {
  window: string
  scans: { count: number; devices_up: AuditRecordRange; open_ports: AuditRecordRange }
  new_in_window: AuditRecordChange[]
  new_more?: number
  gone_in_window: AuditRecordChange[]
  gone_more?: number
  returned_in_window: AuditRecordChange[]
  returned_more?: number
  activity: { new_devices?: number; address_changes?: number; ports_opened?: number; ports_closed?: number; router_refused_attempts?: number }
  modules: AuditRecordModule[]
  /** Records that could not be read, named so a gap is never taken for a quiet network. */
  unread: string[]
}
