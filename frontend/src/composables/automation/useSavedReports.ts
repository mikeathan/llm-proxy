import { ref, watch } from 'vue'
import { DispatcherService } from '../../services/automation/dispatcherService'
import type { SavedReport } from '../../types/dispatcher'

const LOAD_ERROR = 'Could not load the saved reports. Try again in a moment.'
const OPEN_ERROR = 'Could not open that report. It may have been deleted.'

/**
 * The saved final reports of one automation, newest first, and the one being read. The list reloads when another
 * automation is shown and when `latest` changes (a run just saved a new report); an open report stays open across a
 * reload and closes only when the automation changes. Opening reads the file through the workspace file endpoint;
 * if a second report is opened while the first is still loading, the second wins.
 */
export function useSavedReports(
  workspace: () => string,
  automation: () => string,
  latest: () => string,
) {
  const reports = ref<SavedReport[]>([])
  const truncated = ref(false)
  const selected = ref<SavedReport | null>(null)
  const content = ref('')
  const loading = ref(false)
  const error = ref('')
  let openSeq = 0

  async function reload() {
    loading.value = true
    try {
      const list = await DispatcherService.getAutomationReports(workspace(), automation())
      reports.value = list.reports
      truncated.value = list.truncated
      error.value = ''
    } catch {
      reports.value = []
      truncated.value = false
      error.value = LOAD_ERROR
    } finally {
      loading.value = false
    }
  }

  function close() {
    openSeq++
    selected.value = null
    content.value = ''
  }

  async function open(report: SavedReport) {
    const seq = ++openSeq
    selected.value = report
    content.value = ''
    try {
      const text = await DispatcherService.readWorkspaceFile(workspace(), report.path)
      if (seq !== openSeq) return
      content.value = text
      error.value = ''
    } catch {
      if (seq !== openSeq) return
      content.value = ''
      error.value = OPEN_ERROR
    }
  }

  watch([workspace, automation], () => {
    close()
    void reload()
  }, { immediate: true })
  watch(latest, () => { void reload() })

  return { reports, truncated, selected, content, loading, error, reload, open, close }
}
