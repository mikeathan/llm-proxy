import { ref } from 'vue'
import { ReminderService } from '../../services/reminders/reminderService'
import type { ReminderEdit, ReminderList } from '../../types/reminders'

const LOAD_FAILED = 'Reminders could not be read.'
const ACTION_FAILED = 'The change could not be made.'

const reasonOf = (e: unknown, fallback: string): string => (e instanceof Error && e.message ? e.message : fallback)

// The open and recently finished reminders, with the actions that change them.
// Whoever shows the list refreshes it; a change refreshes it too.
export function useReminders() {
  const reminders = ref<ReminderList>({ open: [], recent: [] })
  const loading = ref(true)
  const error = ref<string | null>(null)
  // The server's reason for the last refused or failed change; empty when none.
  const actionError = ref('')
  const saving = ref(false)

  async function load() {
    try {
      reminders.value = await ReminderService.list()
      error.value = null
    } catch (e) {
      error.value = reasonOf(e, LOAD_FAILED)
    } finally {
      loading.value = false
    }
  }

  async function change(action: () => Promise<unknown>): Promise<boolean> {
    saving.value = true
    actionError.value = ''
    try {
      await action()
      await load()
      return true
    } catch (e) {
      actionError.value = reasonOf(e, ACTION_FAILED)
      return false
    } finally {
      saving.value = false
    }
  }

  const save = (id: number, edit: ReminderEdit) => change(() => ReminderService.edit(id, edit))
  const remove = (id: number) => change(() => ReminderService.remove(id))

  return { reminders, loading, error, actionError, saving, load, save, remove }
}
