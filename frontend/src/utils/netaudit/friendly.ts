import type { ScanFinding, ResolvedFinding } from '../../types/netaudit'

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

/** How many things were seen, as one plain sentence. */
export function seenSentence(devices: number, services: number): string {
  return `${services} ${plural(services, 'service', 'services')} on ${devices} ${plural(devices, 'device', 'devices')}`
}

export function devicesIn(findings: readonly Pick<ScanFinding, 'device'>[]): number {
  return new Set(findings.map((f) => f.device)).size
}

export const describeResolved = (r: ResolvedFinding): string => `${r.device}: ${describeService(r)}`

/** The sentence for one thing that needs a look: what it is, where, and (when only its version moved) what changed. */
export function lookSentence(f: Pick<ScanFinding, 'device' | 'port' | 'service' | 'new' | 'version'>): string {
  const what = `${describeService(f)} on ${f.device}`
  return f.new || !f.version ? what : `${what} changed to ${f.version}`
}

/** "3 devices were not scanned to completion". */
export function unscannedSentence(n: number): string {
  return `${n} ${plural(n, 'device was', 'devices were')} not scanned to completion`
}
