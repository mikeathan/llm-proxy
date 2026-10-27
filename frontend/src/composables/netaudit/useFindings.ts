import { ref } from 'vue'
import { DispatcherService } from '../../services/automation/dispatcherService'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import type { FindingsReport } from '../../types/netaudit'

const LOAD_FAILED = 'The latest scan could not be read.'
const ACCEPT_FAILED = 'It could not be marked as fine.'
const REVOKE_FAILED = 'It could not be taken back.'
const RESCAN_FAILED = 'The scan could not be started.'
const QUEUED_STATUS = 'queued'
const SCAN_STARTED = 'The scan has started. The report updates when it finishes.'
const SCAN_QUEUED = 'The scan is waiting for the run before it. The report updates when it finishes.'

/** The latest scan of one automation, and accepting or taking back a finding. Reloads after each change. */
export function useFindings(workspaceId: () => string, automation: () => string) {
  const report = ref<FindingsReport | null>(null)
  const loading = ref(false)
  const error = ref('')
  const actionError = ref('')
  /** What to tell the operator after a scan was started; empty when there is nothing to say. */
  const notice = ref('')

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

  /** Runs the audit again; with a device, only that device (the audit's own settings still apply). */
  async function rescan(device?: string): Promise<boolean> {
    notice.value = ''
    try {
      const started = await DispatcherService.triggerAutomation(workspaceId(), automation(), undefined, device)
      actionError.value = ''
      const what = device ? `Scanning ${device}. ` : ''
      notice.value = what + (started.status === QUEUED_STATUS ? SCAN_QUEUED : SCAN_STARTED)
      return true
    } catch (err) {
      actionError.value = errorMessage(err, RESCAN_FAILED)
      return false
    }
  }

  return { report, loading, error, actionError, notice, load, accept, revoke, rescan }
}
