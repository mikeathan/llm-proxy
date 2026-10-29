import { onScopeDispose, ref } from 'vue'
import { DispatcherService } from '../../services/automation/dispatcherService'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import type { DeviceExpectation, FindingsReport } from '../../types/netaudit'

const LOAD_FAILED = 'The latest scan could not be read.'
const ACCEPT_FAILED = 'It could not be marked as fine.'
const REVOKE_FAILED = 'It could not be taken back.'
const MARK_FAILED = 'It could not be marked as fixed.'
const UNMARK_FAILED = 'The mark could not be taken back.'
const EXPECT_FAILED = 'What this device should offer could not be saved.'
const UNEXPECT_FAILED = 'The expected ports could not be taken back.'
const RESCAN_FAILED = 'The scan could not be started.'
const QUEUED_STATUS = 'queued'
const WHOLE_NETWORK = 'the whole network'
const SCAN_STARTED = 'The scan has started. The report updates when it finishes.'
/** How long a scan we started may go unreported by the page before its buttons come back (the page polls the run state). */
const STARTING_GRACE_MS = 20_000
const SCAN_QUEUED = 'The scan is waiting for the run before it. The report updates when it finishes.'

/** The latest scan of one automation, and accepting or taking back a finding. Reloads after each change. */
export function useFindings(workspaceId: () => string, automation: () => string) {
  const report = ref<FindingsReport | null>(null)
  /** What the operator says each device should offer; empty when none is set or they could not be read. */
  const expectations = ref<DeviceExpectation[]>([])
  const loading = ref(false)
  const error = ref('')
  const actionError = ref('')
  /** What to tell the operator after a scan was started; empty when there is nothing to say. */
  const notice = ref('')
  /**
   * What a scan we just started is scanning ("192.0.2.10", or "the whole network"); empty when none was. The run state the
   * page learns by polling lags the click, so this keeps the screen busy from the click until the page reports the run.
   */
  const starting = ref('')
  let startingTimer: ReturnType<typeof setTimeout> | undefined
  const clearStarting = () => {
    clearTimeout(startingTimer)
    starting.value = ''
  }
  onScopeDispose(() => clearTimeout(startingTimer))

  // The expectations only add a line to a device; a failure to read them must not hide the report, so it leaves none.
  async function loadExpectations(): Promise<void> {
    try {
      const items = await NetauditService.expectations(workspaceId())
      expectations.value = Array.isArray(items) ? items : []
    } catch {
      expectations.value = []
    }
  }

  async function load(): Promise<void> {
    loading.value = true
    try {
      const [latest] = await Promise.all([NetauditService.findings(workspaceId(), automation()), loadExpectations()])
      report.value = latest
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

  /** "I've done this": records that the operator applied the fix. The next scan decides whether it worked. */
  const markFixed = (key: string) =>
    act(() => NetauditService.markFixed({ workspace_id: workspaceId(), automation: automation(), key, note: '' }), MARK_FAILED)

  const unmarkFixed = (key: string) => act(() => NetauditService.unmarkFixed(workspaceId(), key), UNMARK_FAILED)

  /** Says what a device should offer (its open ports right now, or what the operator typed); the next scan reports any difference. */
  const setExpected = (device: string, ports: number[]) =>
    act(() => NetauditService.setExpectation({ workspace_id: workspaceId(), device, ports, note: '' }), EXPECT_FAILED)

  const removeExpected = (device: string) => act(() => NetauditService.removeExpectation(workspaceId(), device), UNEXPECT_FAILED)

  /** Runs the audit again; with a device, only that device (the audit's own settings still apply). */
  async function rescan(device?: string): Promise<boolean> {
    notice.value = ''
    try {
      const started = await DispatcherService.triggerAutomation(workspaceId(), automation(), undefined, device)
      actionError.value = ''
      const what = device ? `Scanning ${device}. ` : ''
      notice.value = what + (started.status === QUEUED_STATUS ? SCAN_QUEUED : SCAN_STARTED)
      starting.value = device ?? WHOLE_NETWORK
      clearTimeout(startingTimer)
      startingTimer = setTimeout(clearStarting, STARTING_GRACE_MS)
      return true
    } catch (err) {
      actionError.value = errorMessage(err, RESCAN_FAILED)
      return false
    }
  }

  return { report, expectations, loading, error, actionError, notice, starting, clearStarting, load, accept, revoke, markFixed, unmarkFixed, setExpected, removeExpected, rescan }
}
