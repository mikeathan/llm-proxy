import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import type { StackView } from '../../../types/stacks'

const service = vi.hoisted(() => ({ stack: vi.fn() }))
vi.mock('../../../services/stacks/stacksService', () => ({ StacksService: service }))

const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', name: 'home', component: { template: '<div/>' } }, { path: '/network/:tab?', name: 'network', component: { template: '<div/>' } }] })
const stackWith = (enabled: boolean): StackView => ({
  id: 'network', title: 'Network', purpose: '', actions: [], summary: { on: 1, total: 1, stopped: 0, blocked: 0 },
  layers: [{ id: 'listen', title: 'What the network says', modules: [{ id: 'login_watch', title: 'Failed logins, this computer', purpose: '', about: { watches: 'w.', never: 'n.', shows_up: 's.' }, finds: ['host_login_failures'], sends: 'nothing', step_up: false, enabled, needs: [] }] }],
})
const setup = async (enabled: boolean, check: string) => {
  const { useModuleSources } = await import('../../../composables/netaudit/useModuleSources')
  const FoundBy = (await import('../../../components/netaudit/FoundBy.vue')).default
  service.stack.mockResolvedValue(stackWith(enabled))
  await useModuleSources().load()
  const w = mount(FoundBy, { props: { check }, global: { plugins: [router] } })
  await flushPromises()
  return w
}

describe('FoundBy', () => {
  beforeEach(() => { service.stack.mockReset(); vi.resetModules() })

  it('names the module that found a problem', async () => {
    const w = await setup(true, 'host_login_failures')
    expect(w.get('[data-test="found-by"]').text()).toBe('Found by Failed logins, this computer')
  })

  it('says to switch the module on when it is off', async () => {
    const w = await setup(false, 'host_login_failures')
    expect(w.get('[data-test="found-by"]').text()).toContain('off. Switch it on to keep this check running.')
  })

  it('shows nothing for a check no single module produces', async () => {
    const w = await setup(true, 'tls_cert_expired')
    expect(w.find('[data-test="found-by"]').exists()).toBe(false)
  })
})
