import { ref } from 'vue'
import { SecurityService } from '../../services/security/securityService'
import { errorMessage } from '../../utils/errors'
import type { AgentWatchView, LockdownState } from '../../types/security'

const LOAD_FAILED = 'The agent activity could not be read.'
const LOCKDOWN_LOAD_FAILED = 'The lock down switch could not be read.'
const ENGAGE_FAILED = 'Lock down could not be switched on.'
const RELEASE_FAILED = 'Lock down could not be lifted.'

/** Settings · Safety · Agent activity: the agent security record and the lock down switch. */
export function useAgentActivity() {
  const record = ref<AgentWatchView | null>(null)
  const lockdown = ref<LockdownState | null>(null)
  const loading = ref(false)
  const error = ref('')
  const lockdownError = ref('')
  const busy = ref(false)

  async function load(): Promise<void> {
    loading.value = true
    const [rec, lock] = await Promise.allSettled([SecurityService.agentWatch(), SecurityService.lockdown()])
    if (rec.status === 'fulfilled') {
      record.value = rec.value
      error.value = ''
    } else {
      error.value = errorMessage(rec.reason, LOAD_FAILED)
    }
    if (lock.status === 'fulfilled') {
      lockdown.value = lock.value
      lockdownError.value = ''
    } else {
      lockdownError.value = errorMessage(lock.reason, LOCKDOWN_LOAD_FAILED)
    }
    loading.value = false
  }

  async function change(run: () => Promise<LockdownState>, failed: string): Promise<boolean> {
    busy.value = true
    try {
      lockdown.value = await run()
      lockdownError.value = ''
      return true
    } catch (err) {
      lockdownError.value = errorMessage(err, failed)
      return false
    } finally {
      busy.value = false
    }
  }

  const engage = (revokeTokens: boolean) => change(() => SecurityService.engageLockdown(revokeTokens), ENGAGE_FAILED)
  const release = () => change(() => SecurityService.releaseLockdown(), RELEASE_FAILED)

  return { record, lockdown, loading, error, lockdownError, busy, load, engage, release }
}
