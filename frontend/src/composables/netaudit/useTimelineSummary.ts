import { ref, type Ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import { TIMELINE_EXPLAIN_FAILED } from '../../constants/netaudit'
import type { NetworkAudit } from '../../types/netaudit'

/**
 * The "Explain this week" button's state. Nothing runs by itself: one click is one request, and a second click while it
 * runs does nothing. The server's own words explain a refusal (a model that is not on this machine, a busy model).
 */
export function useTimelineSummary(audit: Ref<NetworkAudit | null>) {
  const summary = ref('')
  const model = ref('')
  const nothing = ref(false)
  const loading = ref(false)
  const error = ref('')

  async function explain(): Promise<void> {
    if (loading.value || !audit.value) return
    loading.value = true
    error.value = ''
    try {
      const reply = await NetauditService.explainTimeline(audit.value)
      summary.value = reply.summary
      model.value = reply.model ?? ''
      nothing.value = reply.summary === ''
    } catch (err) {
      error.value = errorMessage(err, TIMELINE_EXPLAIN_FAILED)
    } finally {
      loading.value = false
    }
  }

  function clear(): void {
    summary.value = ''
    nothing.value = false
    error.value = ''
  }

  return { summary, model, nothing, loading, error, explain, clear }
}
