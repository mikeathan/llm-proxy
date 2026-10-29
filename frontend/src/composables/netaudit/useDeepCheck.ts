import { ref } from 'vue'
import { HttpError } from '../../services/httpClient'
import { NetauditService } from '../../services/netaudit/netauditService'
import { describeDeepError } from '../../utils/netaudit/deepCheck'
import { errorMessage } from '../../utils/errors'
import type { DeepCheck } from '../../types/netaudit'

const RUN_FAILED = 'The deep check could not be run.'

/**
 * The deep check of one device (network-audit.md 13.J): the last result of each device, and starting a new one. A check is
 * only ever started by a person pressing the button; it takes a few minutes, so `running` names the device meanwhile, and
 * only one runs at a time (the server allows no more).
 */
export function useDeepCheck(workspaceId: () => string, automation: () => string) {
  const checks = ref<DeepCheck[]>([])
  /** The device being checked now, or ''. */
  const running = ref('')
  const error = ref('')

  // The results only add a line to a device; failing to read them must not hide the report, so it leaves none.
  async function load(): Promise<void> {
    try {
      const items = await NetauditService.deepChecks(workspaceId())
      checks.value = Array.isArray(items) ? items : []
    } catch {
      checks.value = []
    }
  }

  const lastOf = (device: string): DeepCheck | undefined => checks.value.find((c) => c.device === device)

  /** Runs the check; true when it finished and its result is stored. The earlier result stays when it fails. */
  async function run(device: string): Promise<boolean> {
    if (running.value) return false
    running.value = device
    error.value = ''
    try {
      const result = await NetauditService.runDeepCheck(workspaceId(), automation(), device)
      checks.value = [result, ...checks.value.filter((c) => c.device !== device)]
      return true
    } catch (err) {
      error.value = err instanceof HttpError && err.status ? describeDeepError(err.status, err.message) : errorMessage(err, RUN_FAILED)
      return false
    } finally {
      running.value = ''
    }
  }

  return { checks, running, error, load, lastOf, run }
}
