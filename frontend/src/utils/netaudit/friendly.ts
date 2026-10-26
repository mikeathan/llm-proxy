import type { ScanFinding, ResolvedFinding } from '../../types/netaudit'
import type { StatusState } from '../../types/ui'

// Plain wording for what a scan found, for the Simple view. Generic on purpose: it names what a port is usually used
// for, never what kind of device it is.

const PORT_MEANING: Record<number, string> = {
  21: 'File transfer (FTP)',
  22: 'Remote login',
  23: 'Remote login without encryption (Telnet)',
  25: 'Sending mail',
  53: 'Name lookups',
  80: 'Web pages',
  110: 'Reading mail',
  139: 'File sharing',
  143: 'Reading mail',
  443: 'Secure web pages',
  445: 'File sharing',
  631: 'Printing',
  3389: 'Remote desktop',
  5900: 'Screen sharing',
  8080: 'Web pages',
}

/** Ports whose service sends everything unencrypted: worth a plain warning in the Simple view. */
const PLAIN_TEXT_PORTS: ReadonlySet<number> = new Set([21, 23])

interface PortRef {
  port?: number
  service?: string
}

const GUESS_SUFFIX = ' (guess)'

/** The service name without the server's "(guess)" marker. */
export const serviceName = (service: string | undefined): string => (service ?? '').replace(GUESS_SUFFIX, '').trim()

export const isGuessedService = (service: string | undefined): boolean => !!service && service.endsWith(GUESS_SUFFIX)

/** What a port is for: the usual meaning of its number, else the service the scan named, else the bare number. */
export function portMeaning(ref: PortRef): string {
  if (ref.port !== undefined && PORT_MEANING[ref.port]) return PORT_MEANING[ref.port]!
  const name = serviceName(ref.service)
  if (name) return name
  return ref.port !== undefined ? `Port ${ref.port}` : 'Something'
}

export const isPlainText = (finding: Pick<ScanFinding, 'port'>): boolean => finding.port !== undefined && PLAIN_TEXT_PORTS.has(finding.port)

/** "Remote login (port 22)": the meaning first, the number for anyone who wants to look it up. */
export function describeService(ref: PortRef & { proto?: string }): string {
  const meaning = portMeaning(ref)
  if (ref.port === undefined || meaning === `Port ${ref.port}`) return meaning
  return `${meaning} (port ${ref.port})`
}

/** The technical form for the Advanced view: "22/tcp ssh". */
export function describePort(ref: PortRef & { proto?: string }): string {
  if (ref.port === undefined) return ''
  const base = `${ref.port}/${ref.proto || 'tcp'}`
  return ref.service ? `${base} ${ref.service}` : base
}

const plural = (n: number, one: string, many: string): string => (n === 1 ? one : many)

/** "4 things need a look", or the all-clear. */
export function needsLookSentence(n: number): string {
  return n === 0 ? 'Nothing needs a look' : `${n} ${plural(n, 'thing needs', 'things need')} a look`
}

/** How many things were seen, as one plain sentence; `checks` is how many problems the checks found, when any ran. */
export function seenSentence(devices: number, services: number, checks = 0): string {
  const seen = `${services} ${plural(services, 'service', 'services')} on ${devices} ${plural(devices, 'device', 'devices')}`
  return checks > 0 ? `${seen}, ${checks} ${plural(checks, 'problem', 'problems')} found by checks` : seen
}

export function devicesIn(findings: readonly Pick<ScanFinding, 'device'>[]): number {
  return new Set(findings.map((f) => f.device)).size
}

const OPEN_PORT = 'open_port'

/** A check is a finding beyond "this port is open": a weak setting a safe script found. */
export const isCheck = (f: { check?: string }): boolean => !!f.check && f.check !== OPEN_PORT

export const portCount = (findings: readonly { check?: string }[]): number => findings.filter((f) => !isCheck(f)).length
export const checkCount = (findings: readonly { check?: string }[]): number => findings.filter(isCheck).length

const CHECK_FALLBACK = 'A check found a problem'

type Titled = { device: string; port?: number; service?: string; check?: string; evidence?: string }

/**
 * What a finding is, in one phrase. A check is the device, then what the server wrote about it in its own fixed words
 * ("10.0.0.1: SMBv1 is enabled"); an open port is what the port is for, and where.
 */
export function findingTitle(f: Titled): string {
  return isCheck(f) ? `${f.device}: ${f.evidence || CHECK_FALLBACK}` : `${describeService(f)} on ${f.device}`
}

export function describeResolved(r: ResolvedFinding): string {
  return isCheck(r) ? `${r.device}: ${r.evidence || CHECK_FALLBACK}` : `${r.device}: ${describeService(r)}`
}

/** The sentence for one thing that needs a look: what it is, where, and (when only its version moved) what changed. */
export function lookSentence(f: Titled & { new?: boolean; version?: string }): string {
  const what = findingTitle(f)
  return isCheck(f) || f.new || !f.version ? what : `${what} changed to ${f.version}`
}

const SEVERITY_ORDER = ['info', 'low', 'medium', 'high', 'critical']

/** How serious a severity is, for sorting (higher is worse); anything unknown ranks below info. */
export const severityRank = (severity: string): number => SEVERITY_ORDER.indexOf(severity.toLowerCase())

const SEVERITY_STATE: Record<string, StatusState> = { critical: 'error', high: 'error', medium: 'warning', low: 'warning' }

export const severityState = (severity: string): StatusState => SEVERITY_STATE[severity.toLowerCase()] ?? 'neutral'

/** "3 devices were not scanned to completion". */
export function unscannedSentence(n: number): string {
  return `${n} ${plural(n, 'device was', 'devices were')} not scanned to completion`
}
