import { describe, expect, it, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import AppBanner from '../../../components/ui/AppBanner.vue'
import { useAppBanner } from '../../../composables/ui/useAppBanner'

const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:p(.*)*', component: { template: '<div />' } }] })
const mountBanner = () => mount(AppBanner, { global: { plugins: [router], stubs: { Icon: true } } })

describe('AppBanner', () => {
  beforeEach(() => useAppBanner().clear())

  it('announces an error as an alert, and a notice politely', async () => {
    const w = mountBanner()
    useAppBanner().show({ severity: 'error', message: 'Could not reach the server' })
    await nextTick()
    expect(w.get('[role="alert"]').text()).toContain('Could not reach the server')
    useAppBanner().show({ severity: 'notice', message: 'Update available' })
    await nextTick()
    expect(w.get('[role="status"]').text()).toContain('Update available')
  })

  it('dismisses a transient banner from its named button, never a persistent one', async () => {
    const w = mountBanner()
    useAppBanner().show({ severity: 'critical', message: 'No model set', persistent: true, action: { label: 'Configure models', to: '/settings/local' } })
    await nextTick()
    expect(w.find('button[aria-label="Dismiss"]').exists()).toBe(false)
    expect(w.get('a').text()).toBe('Configure models')

    useAppBanner().show({ severity: 'error', message: 'Save failed' })
    await nextTick()
    await w.get('button[aria-label="Dismiss"]').trigger('click')
    expect(w.text()).not.toContain('Save failed')
  })

  it('renders the text fallback as text, not HTML', async () => {
    const w = mountBanner()
    useAppBanner().show({ severity: 'error', message: '<b>bold</b>' })
    await nextTick()
    expect(w.find('b').exists()).toBe(false)
    expect(w.text()).toContain('<b>bold</b>')
  })
})
