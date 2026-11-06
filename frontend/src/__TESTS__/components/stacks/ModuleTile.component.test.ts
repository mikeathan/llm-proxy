import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import ModuleTile from '../../../components/stacks/ModuleTile.vue'
import type { StackModule } from '../../../types/stacks'

const module_ = (over: Partial<StackModule> = {}): StackModule => ({
  id: 'phone_home', title: 'Phone-home watch', purpose: 'Where each device calls out to.',
  about: { watches: 'Which names each device asks for.', never: 'It sends nothing.', shows_up: 'In the findings.', settings: true },
  finds: [], sends: 'nothing', step_up: false, enabled: true, needs: [], ...over,
})
const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', name: 'home', component: { template: '<div/>' } }, { path: '/n/settings', name: 'settings', component: { template: '<div/>' } }] })
const mountTile = (m: StackModule, settingsTo?: { name: string }) => mount(ModuleTile, { props: { module: m, busy: false, settingsTo }, global: { plugins: [router] } })

describe('ModuleTile', () => {
  it('folds what the module watches, never does and shows up as, and opens on a tap', async () => {
    const w = mountTile(module_())
    expect(w.get('[data-test="module-about"]').text()).toContain('What it does')
    const toggle = w.get('[data-test="module-about"] button')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    await toggle.trigger('click')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    for (const text of ['Watches', 'Which names each device asks for.', 'Never', 'It sends nothing.', 'Shows up', 'In the findings.']) expect(w.text()).toContain(text)
  })

  it('links to the settings card only when the module has one', async () => {
    const withCard = mountTile(module_(), { name: 'settings' })
    await withCard.get('[data-test="module-about"] button').trigger('click')
    expect(withCard.find('[data-test="module-settings"]').exists()).toBe(true)
    const without = mountTile(module_({ about: { watches: 'w.', never: 'n.', shows_up: 's.' } }), { name: 'settings' })
    await without.get('[data-test="module-about"] button').trigger('click')
    expect(without.find('[data-test="module-settings"]').exists()).toBe(false)
  })
})
