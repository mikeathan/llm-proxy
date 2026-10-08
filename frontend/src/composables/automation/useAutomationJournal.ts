import { ref, watch } from 'vue'
import { DispatcherService } from '../../services/automation/dispatcherService'

const LOAD_ERROR = 'Could not load the journal. Try again in a moment.'
const CLEAR_ERROR = 'Could not clear the journal. Try again in a moment.'

/**
 * The learning journal of one automation: the notes it rewrites at the end of
 * each run. Loaded while the automation keeps a journal and reloaded when
 * another automation is shown; `clear` erases it so the next run starts fresh.
 */
export function useAutomationJournal(
  workspace: () => string,
  automation: () => string,
  enabled: () => boolean,
) {
  const journal = ref('')
  const loading = ref(false)
  const error = ref('')

  async function load() {
    if (!enabled()) {
      journal.value = ''
      error.value = ''
      return
    }
    loading.value = true
    try {
      journal.value = (await DispatcherService.getAutomationJournal(workspace(), automation())).journal
      error.value = ''
    } catch {
      error.value = LOAD_ERROR
    } finally {
      loading.value = false
    }
  }

  async function clear() {
    try {
      await DispatcherService.clearAutomationJournal(workspace(), automation())
      journal.value = ''
      error.value = ''
    } catch {
      error.value = CLEAR_ERROR
    }
  }

  watch([workspace, automation, enabled], load, { immediate: true })

  return { journal, loading, error, reload: load, clear }
}
