import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { StackView, SwitchedStack } from '../../../types/stacks'

const service = vi.hoisted(() => ({ stack: vi.fn(), setModule: vi.fn() }))
vi.mock('../../../services/stacks/stacksService', () => ({ StacksService: service }))

import { useStack } from '../../../composables/stacks/useStack'

const view = (over: Partial<StackView> = {}): StackView => ({
  id: 'network', title: 'Network', purpose: '', summary: { on: 1, total: 2, stopped: 0, blocked: 0 }, actions: [],
  layers: [{ id: 'discover', title: 'Discover', modules: [{ id: 'presence', title: 'Presence check', purpose: '', sends: 'lan', step_up: false, enabled: false, needs: [] }] }],
  ...over,
})
const switched = (over: Partial<SwitchedStack> = {}): SwitchedStack => ({ ...view(), blocked: [], ...over })

describe('useStack', () => {
  beforeEach(() => { service.stack.mockReset(); service.setModule.mockReset() })

  it('loads the stack', async () => {
    service.stack.mockResolvedValue(view())
    const s = useStack('network')
    await s.load()
    expect(service.stack).toHaveBeenCalledWith('network')
    expect(s.stack.value?.title).toBe('Network')
    expect(s.error.value).toBe('')
  })

  it('says when the stack cannot be read', async () => {
    service.stack.mockRejectedValue(new Error('server down'))
    const s = useStack('network')
    await s.load()
    expect(s.stack.value).toBeNull()
    expect(s.error.value).toContain('server down')
  })

  it('shows the server\'s answer after a switch and remembers what the change left blocked', async () => {
    service.setModule.mockResolvedValue(switched({ blocked: ['Weekly summary'] }))
    const s = useStack('network')
    await s.set('presence', false)
    expect(service.setModule).toHaveBeenCalledWith('network', 'presence', false)
    expect(s.leftBlocked.value).toEqual(['Weekly summary'])
  })

  it('keeps the old state and names the refusal when the server says no', async () => {
    service.stack.mockResolvedValue(view())
    service.setModule.mockRejectedValue(new Error('needs Presence check'))
    const s = useStack('network')
    await s.load()
    await s.set('summary', true)
    expect(s.stack.value?.layers[0]?.modules[0]?.enabled).toBe(false)
    expect(s.errorFor.value).toEqual({ module: 'summary', message: 'needs Presence check' })
  })

  it('forgets an earlier refusal when the next switch works', async () => {
    service.setModule.mockRejectedValueOnce(new Error('no')).mockResolvedValueOnce(switched())
    const s = useStack('network')
    await s.set('summary', true)
    await s.set('presence', true)
    expect(s.errorFor.value).toBeNull()
  })

  it('marks only the module being switched as busy', async () => {
    let release: (v: SwitchedStack) => void = () => {}
    service.setModule.mockReturnValue(new Promise<SwitchedStack>((r) => { release = r }))
    const s = useStack('network')
    const pending = s.set('presence', true)
    expect(s.busyModule.value).toBe('presence')
    release(switched())
    await pending
    expect(s.busyModule.value).toBeNull()
  })
})
