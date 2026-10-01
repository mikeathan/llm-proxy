// An in-memory Storage for tests (the unit project runs in node, which has no
// localStorage). `failWrites` simulates a full or blocked store.
export function memoryStorage(initial: Record<string, string> = {}, failWrites = false): Storage {
  const map = new Map(Object.entries(initial))
  return {
    get length() {
      return map.size
    },
    clear: () => map.clear(),
    getItem: (key) => map.get(key) ?? null,
    key: (index) => [...map.keys()][index] ?? null,
    removeItem: (key) => void map.delete(key),
    setItem: (key, value) => {
      if (failWrites) throw new Error('QuotaExceededError')
      map.set(key, String(value))
    },
  }
}
