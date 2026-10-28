import type { AutomationFormData } from '../../types/automation'
import type { ScanSettings } from '../../types/netaudit'

// A network audit's settings in the two shapes the form needs: as typed (text fields) and as the server keeps them.
// Depth, checks and the known-vulnerability look default to thorough and on, as the server's own defaults do.

type ScanFields = Pick<AutomationFormData, 'scanTargets' | 'scanDepth' | 'scanPorts' | 'scanChecks' | 'scanKnownVulns'>

const TARGET_SEPARATOR = /[\n,]/

/** The entries typed in the networks field: separated by commas or lines, blanks dropped. */
export const splitTargets = (text: string): string[] => text.split(TARGET_SEPARATOR).map((t) => t.trim()).filter(Boolean)

export function scanFromForm(f: ScanFields): ScanSettings {
  const targets = splitTargets(f.scanTargets)
  const ports = f.scanPorts.trim()
  return { ...(targets.length ? { targets } : {}), depth: f.scanDepth, ...(ports ? { ports } : {}), checks: f.scanChecks, known_vulns: f.scanKnownVulns }
}

export function formFromScan(s: ScanSettings | undefined): ScanFields {
  return { scanTargets: (s?.targets ?? []).join(', '), scanDepth: s?.depth ?? 'thorough', scanPorts: s?.ports ?? '', scanChecks: s?.checks ?? true, scanKnownVulns: s?.known_vulns ?? true }
}

/** What an audit will do, in one plain line. */
export function scanSummary(s: ScanSettings | undefined): string {
  const parts = [
    s?.depth === 'quick' ? 'Quick scan' : 'Thorough scan',
    s?.checks === false ? 'checks off' : 'safe checks on',
    ...(s?.known_vulns === false ? ['known flaws off'] : []),
    ...(s?.ports ? [`ports ${s.ports}`] : []),
    s?.targets?.length ? s.targets.join(', ') : 'every network you allowed',
  ]
  return parts.join(' · ')
}
