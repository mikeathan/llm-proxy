import { computed, type Ref } from 'vue'
import { usePersistedState } from './usePersistedState'
import { RESOURCE_NAME_PATTERN } from '../../constants/validation'

// The workspace the operator last opened, kept in this browser through the one
// persistence primitive (plan D3), so the sidebar's assistant shortcut goes
// straight there. A module-level singleton: the Workspaces page writes it, the
// sidebar reads it.
const STORAGE_KEY = 'last-workspace'
const STORAGE_VERSION = 1

// A stored value is used only if it is a name the backend accepts.
const parseWorkspace = (data: unknown): string | null =>
  typeof data === 'string' && RESOURCE_NAME_PATTERN.test(data) ? data : null

let last: Ref<string | null> | undefined

export function useLastWorkspace() {
  last ??= usePersistedState<string | null>(STORAGE_KEY, { version: STORAGE_VERSION, parse: parseWorkspace, fallback: null })
  const store = last

  function remember(workspace: string) {
    if (store.value !== workspace) store.value = workspace
  }

  // Only the named workspace is forgotten (e.g. when it is deleted).
  function forget(workspace: string) {
    if (store.value === workspace) store.value = null
  }

  return { lastWorkspace: computed(() => store.value), remember, forget }
}
