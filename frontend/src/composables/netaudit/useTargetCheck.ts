import { onScopeDispose, ref, watch } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { splitTargets } from '../../utils/netaudit/scan'
import type { TargetProblem } from '../../types/netaudit'

/** How long typing must pause before the targets are checked. */
const PAUSE_MS = 400

/**
 * Which of an audit's typed targets the server would refuse, and why, so the form warns before the audit runs. It asks
 * the server because the allowed networks live there; a failed check shows nothing, since the run explains a refusal.
 */
export function useTargetCheck(targets: () => string) {
  const problems = ref<TargetProblem[]>([])
  let timer: ReturnType<typeof setTimeout> | undefined
  let latest = 0

  async function check(entries: string[]): Promise<void> {
    const mine = ++latest
    try {
      const found = await NetauditService.checkTargets(entries)
      if (mine === latest) problems.value = found
    } catch {
      if (mine === latest) problems.value = []
    }
  }

  watch(targets, (text) => {
    clearTimeout(timer)
    const entries = splitTargets(text)
    if (!entries.length) {
      latest++
      problems.value = []
      return
    }
    timer = setTimeout(() => void check(entries), PAUSE_MS)
  })

  onScopeDispose(() => {
    clearTimeout(timer)
    latest++
  })

  return { problems }
}
