import { ref, type InjectionKey } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { isCheck, severityRank } from '../../utils/netaudit/friendly'
import type { DeviceProblemCount, DeviceProblemLookup, NetworkAudit } from '../../types/netaudit'

/** The Network page provides this once; the device list and the traffic detail ask it, so a device with a problem links to it wherever it is shown. */
export const DEVICE_PROBLEMS_INJECTION: InjectionKey<DeviceProblemLookup> = Symbol('deviceProblems')

/**
 * The open problems of the latest audit, by device address. A problem is a check that found something and that nobody accepted; an open port
 * alone is not one. A report that cannot be read leaves no counts, and nothing is shown for it.
 */
export function useDeviceProblems(audit: () => NetworkAudit | null): DeviceProblemLookup & { load: () => Promise<void> } {
  const byDevice = ref<Map<string, DeviceProblemCount>>(new Map())

  async function load(): Promise<void> {
    const a = audit()
    if (!a) {
      byDevice.value = new Map()
      return
    }
    try {
      const report = await NetauditService.findings(a.workspaceId, a.automation)
      const counts = new Map<string, DeviceProblemCount>()
      for (const f of report.findings) {
        if (!isCheck(f) || f.accepted) continue
        const seen = counts.get(f.device)
        counts.set(f.device, { count: (seen?.count ?? 0) + 1, severity: seen && severityRank(seen.severity) >= severityRank(f.severity) ? seen.severity : f.severity })
      }
      byDevice.value = counts
    } catch {
      byDevice.value = new Map()
    }
  }

  return { load, of: (address) => byDevice.value.get(address) ?? null }
}
