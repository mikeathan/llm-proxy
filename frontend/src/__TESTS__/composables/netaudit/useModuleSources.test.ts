import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { StackView } from '../../../types/stacks'

const service = vi.hoisted(() => ({ stack: vi.fn() }))
vi.mock('../../../services/stacks/stacksService', () => ({ StacksService: service }))


const about = { watches: 'w.', never: 'n.', shows_up: 's.' }
const network = (): StackView => ({
  id: 'network', title: 'Network', purpose: '', actions: [], summary: { on: 1, total: 2, stopped: 0, blocked: 0 },
  layers: [{ id: 'listen', title: 'What the network says', modules: [
    { id: 'phone_home', title: 'Phone-home watch', purpose: '', about, finds: ['phone_home_new_place', 'phone_home_encrypted_dns'], sends: 'nothing', step_up: false, enabled: true, needs: [] },
    { id: 'decoys', title: 'Decoys', purpose: '', about, finds: ['decoy_touched'], sends: 'listens', step_up: true, enabled: false, needs: [] },
  ] }],
})

describe('useModuleSources', () => {
  // The stack is cached at module level on purpose (one read serves the overview, the findings and the Help), so each test starts from a fresh module.
  beforeEach(() => { service.stack.mockReset(); vi.resetModules() })
  const fresh = async () => (await import('../../../composables/netaudit/useModuleSources')).useModuleSources()

  it('names the module that produces a check, and none for a check the audit makes itself', async () => {
    service.stack.mockResolvedValue(network())
    const { load, sourceOf } = await fresh()
    await load()
    expect(sourceOf('phone_home_encrypted_dns')?.title).toBe('Phone-home watch')
    expect(sourceOf('decoy_touched')?.enabled).toBe(false)
    expect(sourceOf('tls_cert_expired')).toBeNull()
  })

  it('shows nothing, and does not throw, when the modules cannot be read', async () => {
    service.stack.mockRejectedValue(new Error('offline'))
    const { load, sourceOf } = await fresh()
    await expect(load()).resolves.toBeUndefined()
    expect(sourceOf('decoy_touched')).toBeNull()
  })

  it('treats an answer with no layers as no modules', async () => {
    service.stack.mockResolvedValue({})
    const { load, sourceOf, modules } = await fresh()
    await load()
    expect(modules.value).toEqual([])
    expect(sourceOf('decoy_touched')).toBeNull()
  })

  it('keeps what it knew when a later read fails', async () => {
    service.stack.mockResolvedValueOnce(network()).mockRejectedValueOnce(new Error('offline'))
    const { load, sourceOf } = await fresh()
    await load()
    await load()
    expect(sourceOf('decoy_touched')?.title).toBe('Decoys')
  })
})
