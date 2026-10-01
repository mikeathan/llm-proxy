import { ref, watch, type Ref } from 'vue'
import type { PersistedEnvelope, PersistedStateOptions } from '../../types/ui'

// The single persistence primitive (plan D3). Every persisted UI preference goes
// through here: namespaced key, a schema version, and corrupt / absent /
// unreadable data degrading to the fallback instead of throwing. No feature
// talks to localStorage directly.

const NAMESPACE = 'admin-ui'

export function persistedKey(key: string): string {
  return `${NAMESPACE}:${key}`
}

/** localStorage, or undefined where it is unavailable (disabled, sandboxed). */
export function browserStorage(): Storage | undefined {
  try {
    return globalThis.localStorage ?? undefined
  } catch {
    return undefined
  }
}

function isEnvelope(value: unknown): value is PersistedEnvelope {
  return typeof value === 'object' && value !== null && 'version' in value && 'data' in value
    && typeof (value as PersistedEnvelope).version === 'number'
}

function read<T>(storage: Storage | undefined, key: string, options: PersistedStateOptions<T>): T {
  let envelope: unknown
  try {
    const raw = storage?.getItem(key)
    if (raw == null) return options.fallback
    envelope = JSON.parse(raw)
  } catch {
    return options.fallback
  }
  if (!isEnvelope(envelope)) return options.fallback
  if (envelope.version === options.version) return options.parse(envelope.data) ?? options.fallback
  if (envelope.version < options.version && options.migrate) {
    const migrated = options.migrate(envelope.version, envelope.data)
    return migrated === null ? options.fallback : (options.parse(migrated) ?? options.fallback)
  }
  return options.fallback
}

export function usePersistedState<T>(key: string, options: PersistedStateOptions<T>): Ref<T> {
  const storage = 'storage' in options ? options.storage : browserStorage()
  const fullKey = persistedKey(key)
  const state = ref(read(storage, fullKey, options)) as Ref<T>
  watch(
    state,
    (value) => {
      try {
        storage?.setItem(fullKey, JSON.stringify({ version: options.version, data: value }))
      } catch {
        // Full or blocked storage: the value still lives in memory for this session.
      }
    },
    { deep: true },
  )
  return state
}
