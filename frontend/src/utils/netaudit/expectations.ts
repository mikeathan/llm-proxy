import { MAX_EXPECTED_PORTS } from '../../constants/netaudit'
import type { ScanFinding } from '../../types/netaudit'

const OPEN_PORT = 'open_port'
const TCP = 'tcp'
const NONE_LABEL = 'nothing'
const LOWEST_PORT = 1
const HIGHEST_PORT = 65535

/** The distinct TCP ports the latest scan found open on a device, lowest first (an accepted port is still open). */
export function openPortsOf(findings: readonly ScanFinding[], device: string): number[] {
  const ports = findings.filter((f) => f.device === device && f.check === OPEN_PORT && (f.proto || TCP) === TCP).map((f) => f.port ?? 0)
  return [...new Set(ports.filter((n) => n >= LOWEST_PORT))].sort((a, b) => a - b)
}

/** "22, 631", or "nothing" for a device that should not answer at all. */
export function formatPorts(ports: readonly number[]): string {
  return ports.length ? ports.join(', ') : NONE_LABEL
}

/** Ports typed by the operator: numbers split by commas or spaces. An empty text is the empty list; null when anything is not a usable port. */
export function parsePorts(text: string): number[] | null {
  const parts = text.split(/[\s,]+/).filter(Boolean)
  const ports: number[] = []
  for (const part of parts) {
    const n = Number(part)
    if (!/^\d+$/.test(part) || n < LOWEST_PORT || n > HIGHEST_PORT) return null
    if (!ports.includes(n)) ports.push(n)
  }
  if (ports.length > MAX_EXPECTED_PORTS) return null
  return ports.sort((a, b) => a - b)
}
