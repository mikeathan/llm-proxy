import { ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import type { FindingsReport } from '../../types/netaudit'

const LOAD_FAILED = 'The latest scan could not be read.'
const ACCEPT_FAILED = 'It could not be marked as fine.'
const REVOKE_FAILED = 'It could not be taken back.'

/** The latest scan of one automation, and accepting or taking back a finding. Reloads after each change. */
export function useFindings(workspaceId: () => string, automation: () => string) {
  const report = ref<FindingsReport | null>(null)
  const loading = ref(false)
  const error = ref('')
  const actionError = ref('')

  async function load(): Promise<void> {
    loading.value = true
    try {
      report.value = await NetauditService.findings(workspaceId(), automation())
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, LOAD_FAILED)
    } finally {
      loading.value = false
    }
  }

  async function act(run: () => Promise<unknown>, failed: string): Promise<boolean> {
    try {
      await run()
      actionError.value = ''
    } catch (err) {
      actionError.value = errorMessage(err, failed)
      return false
    }
    await load()
    return true
  }

  const accept = (key: string, note: string, days: number) =>
    act(() => NetauditService.accept({ workspace_id: workspaceId(), automation: automation(), key, note, days }), ACCEPT_FAILED)

  const revoke = (key: string) => act(() => NetauditService.revoke(workspaceId(), key), REVOKE_FAILED)

  return { report, loading, error, actionError, load, accept, revoke }
}
