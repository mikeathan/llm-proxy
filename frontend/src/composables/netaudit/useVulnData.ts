import { getCurrentScope, onScopeDispose, ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { useNetauditReadiness } from './useNetauditReadiness'
import { errorMessage } from '../../utils/errors'
import type { VulnDataStatus, VulnImportSource } from '../../types/netaudit'

const POLL_MS = 1000
const READ_FAILED = 'The vulnerability data status could not be read.'
const START_FAILED = 'The download could not be started.'
const CANCEL_FAILED = 'The download could not be stopped.'
const IMPORT_FAILED = 'The file could not be imported.'
const CLEAR_FAILED = 'The data could not be deleted.'

// One shared status: the settings card and the readiness banner show the same download. The server job keeps going
// without a page, so the screen only polls (once a second) while it is running and someone is looking; the poller
// counts its consumers and stops at zero.
const status = ref<VulnDataStatus | null>(null)
const error = ref('')
const actionError = ref('')
const busy = ref(false)
let consumers = 0
let timer: ReturnType<typeof setInterval> | null = null
let inFlight = false

const running = () => status.value?.state === 'running'

function stopTimer() {
  if (timer === null) return
  clearInterval(timer)
  timer = null
}

function syncTimer() {
  if (consumers > 0 && running()) {
    if (timer === null) timer = setInterval(() => void refresh(), POLL_MS)
  } else {
    stopTimer()
  }
}

async function refresh(): Promise<void> {
  if (inFlight) return
  inFlight = true
  try {
    apply(await NetauditService.vulnData())
    error.value = ''
  } catch (err) {
    error.value = errorMessage(err, READ_FAILED)
  } finally {
    inFlight = false
  }
}

/** Takes a new status; when a download has just ended, the readiness answer (which lists the missing data) is read again. */
function apply(next: VulnDataStatus): void {
  const wasRunning = running()
  status.value = next
  syncTimer()
  if (wasRunning && !running()) void useNetauditReadiness().refresh()
}

async function act(call: () => Promise<VulnDataStatus>, failed: string, readinessChanges = false): Promise<void> {
  busy.value = true
  try {
    apply(await call())
    if (readinessChanges) void useNetauditReadiness().refresh()
    actionError.value = ''
  } catch (err) {
    actionError.value = errorMessage(err, failed)
  } finally {
    busy.value = false
  }
}

/** The vulnerability-data download as a screen sees it. Call it from a component's setup; leaving it ends this consumer. */
export function useVulnData() {
  if (getCurrentScope()) {
    consumers++
    onScopeDispose(() => {
      consumers--
      syncTimer()
    })
  }
  syncTimer()
  return {
    status,
    error,
    actionError,
    busy,
    refresh,
    /** Fetches what is missing, refreshes what is held, or with `restart` fetches every file again. */
    start: (restart = false) => act(() => NetauditService.startVulnDownload(restart), START_FAILED),
    cancel: () => act(NetauditService.cancelVulnDownload, CANCEL_FAILED),
    /** Reads a data file the person picked, for a computer that cannot download it. */
    importFile: (source: VulnImportSource, file: File, year?: number) => act(() => NetauditService.importVulnData(source, file, year), IMPORT_FAILED, true),
    /** Deletes the downloaded data. The caller asks the person first. */
    clear: () => act(NetauditService.clearVulnData, CLEAR_FAILED, true),
  }
}
