import { computed } from 'vue'
import type { WorkloadClass, TuningFieldPolicy } from '../../types/model'

/**
 * Field policy for the model tuning form, driven by the server-computed
 * `workload_class` (single authority, §2.7).
 *
 * Local workloads derive max_tokens / context_budget from serving n_ctx and the
 * reasoning budget from max_tokens (the server clears submitted ones), so the
 * form shows them as one sentence. Cloud workloads edit them, prefilled from
 * published capabilities.
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
    isLocal: isLocal.value,
    isCloud: isCloud.value,
    isUnresolved: isUnresolved.value,
  }
}
