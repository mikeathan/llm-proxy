import { onMounted, onUnmounted, ref } from 'vue'
import { ReminderService } from '../../services/reminders/reminderService'
import { POLL_INTERVAL_MS } from '../../constants/api'
import type { ReminderEdit, ReminderList } from '../../types/reminders'

const LOAD_FAILED = 'Reminders could not be read.'
const ACTION_FAILED = 'The change could not be made.'

const reasonOf = (e: unknown, fallback: string): string => (e instanceof Error && e.message ? e.message : fallback)
const emptyList = (): ReminderList => ({ open: [], recent: [] })

// The reminders list, shared by everything that shows it (the Overview panel, the
// Reminders page): one copy of the data and one timer however many are mounted.
// The timer runs while at least one consumer is mounted, pauses while the tab is
// hidden and refreshes when it is shown again; when the last consumer goes, the
// timer stops and the data is dropped, so nothing stale is shown next time.
const reminders = ref<ReminderList>(emptyList())
const loading = ref(true)
const error = ref<string | null>(null)
// The server's reason for the last refused or failed change; empty when none.
const actionError = ref('')
const saving = ref(false)

let consumers = 0
let timer: ReturnType<typeof setInterval> | null = null
// Bumped when the data is dropped, so a read that was in flight is ignored.
let epoch = 0

async function load() {
  const mine = epoch
  try {
    const list = await ReminderService.list()
    if (mine !== epoch) return
    reminders.value = list
    error.value = null
  } catch (e) {
    if (mine !== epoch) return
    error.value = reasonOf(e, LOAD_FAILED)
  } finally {
    if (mine === epoch) loading.value = false
  }
}

const startTimer = () => {
  if (!timer) timer = setInterval(load, POLL_INTERVAL_MS)
}
const stopTimer = () => {
  if (timer) clearInterval(timer)
  timer = null
}

function onVisibilityChange() {
  if (document.visibilityState === 'hidden') {
    stopTimer()
  } else if (!timer) {
    void load()
    startTimer()
  }
}

function acquire() {
  consumers++
  if (consumers > 1) return
  void load()
  if (document.visibilityState !== 'hidden') startTimer()
  document.addEventListener('visibilitychange', onVisibilityChange)
}

function release() {
  consumers--
  if (consumers > 0) return
  stopTimer()
  document.removeEventListener('visibilitychange', onVisibilityChange)
  epoch++
  reminders.value = emptyList()
  loading.value = true
  error.value = null
  actionError.value = ''
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

const create = (edit: ReminderEdit) => change(() => ReminderService.create(edit))
const save = (id: number, edit: ReminderEdit) => change(() => ReminderService.edit(id, edit))
const remove = (id: number) => change(() => ReminderService.remove(id))
const pause = (id: number) => change(() => ReminderService.pause(id))
const resume = (id: number) => change(() => ReminderService.resume(id))
const skip = (id: number) => change(() => ReminderService.skip(id))

export function useReminders() {
  onMounted(acquire)
  onUnmounted(release)
  return { reminders, loading, error, actionError, saving, load, create, save, remove, pause, resume, skip }
}
