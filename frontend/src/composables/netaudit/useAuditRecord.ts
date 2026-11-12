import { ref, watch } from "vue"
import { DispatcherService } from "../../services/automation/dispatcherService"
import { parseAuditRecord, recordPathOf } from "../../utils/netaudit/auditRecord"
import type { AuditRecord } from "../../types/auditRecord"

/**
 * The typed record of an audit's latest report: read from the run's folder when the report sits in one, and null when there is no
 * record (an older report, a record that cannot be read or is from a version this screen does not know). A failure is never shown as an
 * error: the findings below are complete without it. If the report changes while one is being read, the newer read wins.
 */
export function useAuditRecord(workspace: () => string, reportPath: () => string, refreshKey: () => string = () => "") {
  const record = ref<AuditRecord | null>(null)
  const loading = ref(false)
  let seq = 0

  async function load() {
    const path = recordPathOf(reportPath())
    const mine = ++seq
    if (!path) {
      record.value = null
      loading.value = false
      return
    }
    loading.value = true
    try {
      const parsed = parseAuditRecord(await DispatcherService.readWorkspaceFile(workspace(), path))
      if (mine === seq) record.value = parsed
    } catch {
      if (mine === seq) record.value = null
    } finally {
      if (mine === seq) loading.value = false
    }
  }

  watch([workspace, reportPath, refreshKey], () => void load(), { immediate: true })
  return { record, loading, reload: load }
}
