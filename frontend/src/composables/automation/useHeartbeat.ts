import { computed, ref, watch, type Ref } from 'vue'
import { DispatcherService } from '../../services/automation/dispatcherService'
import {
  HEARTBEAT_DEFAULT_EVERY,
  activeHoursInvalid,
  joinActiveHours,
  splitActiveHours,
} from '../../utils/automation/heartbeat'
import type { HeartbeatConfig, HeartbeatDraft, HeartbeatState } from '../../types/heartbeat'

const LOAD_FAILED = 'The heartbeat settings could not be read.'

const draftFrom = (config: HeartbeatConfig): HeartbeatDraft => {
  const { from, to } = splitActiveHours(config.active_hours)
  return {
    enabled: config.enabled,
    every: config.every || HEARTBEAT_DEFAULT_EVERY,
    model: config.model ?? '',
    connector: config.notify?.connector ?? '',
    activeFrom: from,
    activeTo: to,
  }
}

const errorText = (err: unknown, fallback: string) => (err instanceof Error ? err.message : fallback)

// One workspace's heartbeat: the saved state, an editable draft and the save.
export function useHeartbeat(workspaceId: Readonly<Ref<string>>) {
  const state = ref<HeartbeatState | null>(null)
  const draft = ref<HeartbeatDraft | null>(null)
  const loading = ref(false)
  const loadError = ref('')
  const saving = ref(false)
  const saveError = ref('')

  const dirty = computed(
    () => !!state.value && !!draft.value && JSON.stringify(draft.value) !== JSON.stringify(draftFrom(state.value.config)),
  )

  // A half-filled or empty window cannot be saved; the panel says to set two different times or neither.
  const activeHoursInvalidNow = computed(
    () => !!draft.value && activeHoursInvalid(draft.value.activeFrom, draft.value.activeTo),
  )

  function adopt(next: HeartbeatState) {
    state.value = next
    draft.value = draftFrom(next.config)
  }

  async function load() {
    loading.value = true
    loadError.value = ''
    try {
      adopt(await DispatcherService.getHeartbeat(workspaceId.value))
    } catch (err) {
      loadError.value = errorText(err, LOAD_FAILED)
    } finally {
      loading.value = false
    }
  }

  // Delivery details beyond the connector (dedup, retention) are kept as saved.
  function payload(current: HeartbeatState, edited: HeartbeatDraft): HeartbeatConfig {
    return {
      enabled: edited.enabled,
      every: edited.every,
      model: edited.model,
      active_hours: joinActiveHours(edited.activeFrom, edited.activeTo),
      notify: edited.connector ? { ...current.config.notify, connector: edited.connector } : undefined,
    }
  }

  async function save() {
    if (!state.value || !draft.value || activeHoursInvalidNow.value) return
    saving.value = true
    saveError.value = ''
    try {
      adopt(await DispatcherService.putHeartbeat(workspaceId.value, payload(state.value, draft.value)))
    } catch (err) {
      saveError.value = `Could not save the heartbeat: ${errorText(err, 'the request failed')}. Your changes are still here, so you can try again.`
    } finally {
      saving.value = false
    }
  }

  function discard() {
    saveError.value = ''
    if (state.value) draft.value = draftFrom(state.value.config)
  }

  watch(workspaceId, load, { immediate: true })
  return {
    state,
    draft,
    loading,
    loadError,
    saving,
    saveError,
    dirty,
    activeHoursInvalid: activeHoursInvalidNow,
    load,
    save,
    discard,
  }
}
