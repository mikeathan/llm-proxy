// A width-driven matchMedia stand-in: `(min-width: Npx)` queries evaluate
// against a mutable viewport width and fire `change` when it crosses them.
type Listener = () => void

export function fakeMatchMedia(initialWidth: number) {
  let width = initialWidth
  const listeners = new Map<string, Set<Listener>>()
  const minWidth = (query: string) => Number(/min-width:\s*(\d+)px/.exec(query)?.[1] ?? 0)

  const matchMedia = (query: string) => {
    const set = listeners.get(query) ?? new Set<Listener>()
    listeners.set(query, set)
    return {
      media: query,
      get matches() {
        return width >= minWidth(query)
      },
      addEventListener: (_type: string, fn: Listener) => set.add(fn),
      removeEventListener: (_type: string, fn: Listener) => set.delete(fn),
    } as unknown as MediaQueryList
  }

  return {
    matchMedia,
    resize(next: number) {
      width = next
      listeners.forEach((set) => set.forEach((fn) => fn()))
    },
    listenerCount: () => [...listeners.values()].reduce((n, set) => n + set.size, 0),
  }
}
