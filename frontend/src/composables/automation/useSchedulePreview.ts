import { onScopeDispose, ref, watch } from 'vue'
import { DispatcherService } from '../../services/automation/dispatcherService'
import type { TriggerType } from '../../types/automation'
import type { SchedulePreview } from '../../types/schedule'

/** How long typing must pause before the schedule is checked with the server. */
export const SCHEDULE_PREVIEW_DEBOUNCE_MS = 300

interface TriggerSource {
  type: TriggerType
  value: string
}

// What the server says about a schedule while it is being edited: whether it is
// valid, the zone it runs in and its next runs. The server owns the answer (it
// is the scheduler's own parser); this only asks, debounced, and drops answers
// to schedules that have since changed. A manual or empty schedule asks nothing.
export function useSchedulePreview(source: () => TriggerSource) {
  const preview = ref<SchedulePreview | null>(null)
  const checking = ref(false)
  const failed = ref(false)

  let timer: ReturnType<typeof setTimeout> | undefined
  let latest = 0

  const cancel = () => {
    clearTimeout(timer)
    timer = undefined
    latest++
  }

  const check = async (trigger: TriggerSource, ticket: number) => {
    try {
      const answer = await DispatcherService.previewSchedule(trigger)
      if (ticket === latest) preview.value = answer
    } catch {
      if (ticket === latest) failed.value = true
    } finally {
      if (ticket === latest) checking.value = false
    }
  }

  watch(
    () => ({ ...source() }),
    (trigger) => {
      cancel()
      preview.value = null
      failed.value = false
      checking.value = false
      if (trigger.type === 'manual' || !trigger.value) return
      checking.value = true
      const ticket = latest
      timer = setTimeout(() => void check(trigger, ticket), SCHEDULE_PREVIEW_DEBOUNCE_MS)
    },
    { immediate: true },
  )

  onScopeDispose(cancel)

  return { preview, checking, failed }
}
