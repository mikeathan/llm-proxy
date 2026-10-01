import { computed, type Ref } from 'vue'
import { usePersistedState } from '../ui/usePersistedState'

// Pinned assistant conversations, per workspace, kept in this browser through
// the one persistence primitive (plan D3). A module-level singleton, so every
// list reads the same pins.
const STORAGE_KEY = 'pinned-sessions'
const STORAGE_VERSION = 1

function parsePins(data: unknown): Record<string, string[]> | null {
  if (typeof data !== 'object' || data === null || Array.isArray(data)) return null
  const pins: Record<string, string[]> = {}
  for (const [workspace, ids] of Object.entries(data)) {
    if (Array.isArray(ids)) pins[workspace] = ids.filter((id): id is string => typeof id === 'string')
  }
  return pins
}

let pins: Ref<Record<string, string[]>> | undefined

export function usePinnedSessions(workspaceId: () => string) {
  pins ??= usePersistedState<Record<string, string[]>>(STORAGE_KEY, { version: STORAGE_VERSION, parse: parsePins, fallback: {} })
  const store = pins

  const pinned = computed(() => new Set(store.value[workspaceId()] ?? []))

  function toggle(sessionId: string) {
    const ws = workspaceId()
    const current = store.value[ws] ?? []
    const next = current.includes(sessionId) ? current.filter((id) => id !== sessionId) : [...current, sessionId]
    store.value = { ...store.value, [ws]: next }
  }

  // A deleted conversation keeps no pin.
  function forget(sessionIds: readonly string[]) {
    const ws = workspaceId()
    const current = store.value[ws]
    if (!current?.some((id) => sessionIds.includes(id))) return
    store.value = { ...store.value, [ws]: current.filter((id) => !sessionIds.includes(id)) }
  }

  return { pinned, toggle, forget }
}
