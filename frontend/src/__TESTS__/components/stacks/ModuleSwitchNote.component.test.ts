import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import ModuleSwitchNote from '../../../components/stacks/ModuleSwitchNote.vue'
import { ROUTE_NAMES } from '../../../types/routes'

const mountNote = async (props: Record<string, unknown>) => {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/network/:tab?', name: ROUTE_NAMES.network, component: { template: '<div />' } }] })
  await router.push('/network/settings')
  return mount(ModuleSwitchNote, { props: { enabled: false, ...props }, global: { plugins: [router] } })
}

describe('ModuleSwitchNote', () => {
  it('says whether the module is on, with no switch of its own', async () => {
    const off = await mountNote({})
    expect(off.text()).toContain('Switched off.')
    expect(off.find('input').exists()).toBe(false)
    expect((await mountNote({ enabled: true })).text()).toContain('Switched on.')
  })

  it('points at the one place the module is switched', async () => {
    const link = (await mountNote({})).get('a')
    expect(link.attributes('href')).toBe('/network/modules')
    expect(link.text()).toBe('Switch it on the Modules tab')
  })

  it('keeps the explanation the switch carried, the long part folded', async () => {
    const w = await mountNote({ hint: 'Fake services.', details: 'Long text.' })
    expect(w.text()).toContain('Fake services.')
    const hidden = () => (w.get('[data-test="toggle-details-text"]').attributes('style') ?? '').includes('display: none')
    expect(hidden()).toBe(true)
    await w.get('[data-test="toggle-details"]').trigger('click')
    expect(hidden()).toBe(false)
  })
})
