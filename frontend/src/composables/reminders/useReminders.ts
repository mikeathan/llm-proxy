import { ref } from 'vue'
import { ReminderService } from '../../services/reminders/reminderService'
import type { ReminderList } from '../../types/reminders'

const LOAD_FAILED = 'Reminders could not be read.'

// The open and recently finished reminders, refreshed by whoever shows them.
export function useReminders() {
  const reminders = ref<ReminderList>({ open: [], recent: [] })
  const loading = ref(true)
  const error = ref<string | null>(null)

  async function load() {
    try {
      reminders.value = await ReminderService.list()
      error.value = null
    } catch (e) {
      error.value = e instanceof Error ? e.message : LOAD_FAILED
    } finally {
      loading.value = false
    }
  }

  return { reminders, loading, error, load }
}
