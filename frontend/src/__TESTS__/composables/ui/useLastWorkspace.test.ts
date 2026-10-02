import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { memoryStorage } from '../../helpers/memoryStorage'

// The workspace the operator last opened, kept in this browser so the sidebar's
// assistant shortcut survives a reload. A fresh module per test: it is a singleton.
async function load(stored?: unknown) {
  const initial: Record<string, string> = stored === undefined ? {} : { 'admin-ui:last-workspace': JSON.stringify({ version: 1, data: stored }) }
  vi.stubGlobal('localStorage', memoryStorage(initial))
  vi.resetModules()
  const { useLastWorkspace } = await import('../../../composables/ui/useLastWorkspace')
  return useLastWorkspace()
}

describe('useLastWorkspace', () => {
  beforeEach(() => vi.unstubAllGlobals())
  afterEach(() => vi.unstubAllGlobals())

  it('remembers a workspace across a reload, and forgets only that one', async () => {
    const first = await load()
    expect(first.lastWorkspace.value).toBeNull()
    first.remember('demo')
    expect(first.lastWorkspace.value).toBe('demo')
    first.forget('lab')
    expect(first.lastWorkspace.value).toBe('demo')
    first.forget('demo')
    expect(first.lastWorkspace.value).toBeNull()

    expect((await load('demo')).lastWorkspace.value).toBe('demo')
  })

  it('ignores a stored name the backend would never accept', async () => {
    expect((await load('../etc')).lastWorkspace.value).toBeNull()
    expect((await load(42)).lastWorkspace.value).toBeNull()
  })
})
