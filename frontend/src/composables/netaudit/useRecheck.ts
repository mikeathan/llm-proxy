import { reactive, ref } from 'vue'
import type { InjectionKey } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { RECHECK_FAILED } from '../../constants/netaudit'
import { errorMessage } from '../../utils/errors'
import type { RecheckController, RecheckResult } from '../../types/netaudit'

/** How the Findings screen hands its recheck to the cards that show a finding (they sit several components below). */
export const RECHECK_INJECTION: InjectionKey<RecheckController> = Symbol('netaudit-recheck')

/**
 * "Check again" for one finding at a time: the answer of each recheck kept by finding key until the screen is read again, and the
 * key being checked. `afterCheck` reloads the report, so a fixed finding shows its verified state.
 */
export function useRecheck(workspaceId: () => string, automation: () => string, afterCheck: () => Promise<void>): RecheckController {
  const busyKey = ref('')
  const results = reactive<Record<string, RecheckResult>>({})
  const errors = reactive<Record<string, string>>({})

  async function run(key: string): Promise<void> {
    if (busyKey.value) return
    busyKey.value = key
    delete errors[key]
    try {
      results[key] = await NetauditService.recheck({ workspace_id: workspaceId(), automation: automation(), key, note: '' })
      await afterCheck()
    } catch (err) {
      errors[key] = errorMessage(err, RECHECK_FAILED)
    } finally {
      busyKey.value = ''
    }
  }

  return { busyKey, results, errors, run }
}
