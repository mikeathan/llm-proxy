import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { StackView } from '../../../types/stacks'

const service = vi.hoisted(() => ({ stack: vi.fn() }))
vi.mock('../../../services/stacks/stacksService', () => ({ StacksService: service }))

const loadStrip = async () => (await import('../../../components/netaudit/CoverageStrip.vue')).default

const mod = (id: string, title: string, enabled: boolean, health?: 'working' | 'stopped') => ({
  id, title, purpose: '', about: { watches: `Watches ${id}.`, never: 'n.', shows_up: 's.' }, finds: [], sends: 'nothing' as const, step_up: false, enabled, health, needs: [],
})
const stackOf = (...modules: ReturnType<typeof mod>[]): StackView => ({
  id: 'network', title: 'Network', purpose: '', actions: [], summary: { on: 0, total: modules.length, stopped: 0, blocked: 0 },
  layers: [{ id: 'listen', title: 'What the network says', modules }],
})

describe('CoverageStrip', () => {
  // The module list is cached at module level, so each test starts from a fresh module.
  beforeEach(() => { service.stack.mockReset(); vi.resetModules() })

  it('says how many protections are watching and lists the ones that are off with what each would watch', async () => {
    service.stack.mockResolvedValue(stackOf(mod('a', 'Alpha', true, 'working'), mod('b', 'Beta', false), mod('c', 'Gamma', true, 'stopped')))
    const w = mount(await loadStrip())
    await flushPromises()
    expect(w.get('[data-test="coverage-line"]').text()).toContain('2 of 3 protections are watching · 1 stopped')
    await w.get('[data-test="coverage-line"] button').trigger('click')
    expect(w.get('[data-test="coverage-off"]').text()).toContain('Beta: Watches b.')
    expect(w.get('[data-test="coverage-stopped"]').text()).toContain('Gamma has stopped')
  })

  it('has nothing to open when every protection is on, and tells the person so', async () => {
    service.stack.mockResolvedValue(stackOf(mod('a', 'Alpha', true, 'working')))
    const w = mount(await loadStrip())
    await flushPromises()
    expect(w.get('[data-test="coverage-all-on"]').text()).toContain('Every protection is on')
  })

  it('asks the page to open the Modules tab', async () => {
    service.stack.mockResolvedValue(stackOf(mod('b', 'Beta', false)))
    const w = mount(await loadStrip())
    await flushPromises()
    await w.get('[data-test="coverage-line"] button').trigger('click')
    await w.findAll('button').find((b) => b.text().includes('Modules tab'))!.trigger('click')
    expect(w.emitted('open-tab')?.[0]).toEqual(['modules'])
  })

  it('shows nothing before the modules have been read', async () => {
    service.stack.mockReturnValue(new Promise(() => {}))
    expect(mount(await loadStrip()).find('[data-test="coverage"]').exists()).toBe(false)
  })
})
