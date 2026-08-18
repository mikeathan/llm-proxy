import { computed } from 'vue'
import type { WorkloadClass, TuningFieldPolicy } from '../../types/model'

/**
 * Field policy for the model tuning form, driven by the server-computed
 * `workload_class` (single authority, §2.7).
 *
 * Local workloads derive max_tokens / context_budget from serving n_ctx, so
 * those fields are `derived` (read-only, shown as a "derived" pill). Cloud
 * workloads are `editable` and prefilled from published capabilities.
 *
 * For unsaved models the class may be unresolved (empty) — callers classify
 * only the draft endpoint; the backend remains authoritative after save.
 */
export function useTuningFieldPolicy(workload: WorkloadClass): TuningFieldPolicy {
  const isLocal = computed(() => workload === 'local')
  const isCloud = computed(() => workload === 'cloud')
  const isUnresolved = computed(() => !isLocal.value && !isCloud.value)

  return {
    workload,
    maxTokens: isLocal.value ? 'derived' : 'editable',
    contextBudget: isLocal.value ? 'derived' : 'editable',
    isLocal: isLocal.value,
    isCloud: isCloud.value,
    isUnresolved: isUnresolved.value,
  }
}
