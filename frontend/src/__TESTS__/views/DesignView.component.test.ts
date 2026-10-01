import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import DesignView from '../../views/DesignView.vue'
import { CONTRAST_PAIRS } from '../../theme/contrastPairs'
import { TOKEN_REGISTRY } from '../../theme/tokenRegistry'
import { PRESET_IDS } from '../../theme/presets'
import { useTheme } from '../../composables/ui/useTheme'

// main.ts starts the theme in the app; the view only reads and selects.
useTheme().start()

const colourTokens = Object.entries(TOKEN_REGISTRY).filter(([, spec]) => spec.kind === 'colour')

describe('DesignView (dev-only /design)', () => {
  it('shows a swatch for every colour token and a row for every contrast pair', () => {
    const wrapper = mount(DesignView)
    expect(wrapper.findAll('[data-test="swatch"]')).toHaveLength(colourTokens.length)
    expect(wrapper.findAll('[data-test="contrast-row"]')).toHaveLength(CONTRAST_PAIRS.length)
  })

  it('switches the applied theme from the preset picker', async () => {
    const wrapper = mount(DesignView)
    await wrapper.get(`[data-test="preset-retro-paper"]`).trigger('click')
    await nextTick()
    expect(document.documentElement.dataset.theme).toBe('retro-paper')
    expect(wrapper.findAll('[data-test^="preset-"]')).toHaveLength(PRESET_IDS.length)
    expect(wrapper.get('[data-test="preset-retro-paper"]').attributes('aria-pressed')).toBe('true')
  })

  it('reports every pair of the active preset as passing', () => {
    const wrapper = mount(DesignView)
    expect(wrapper.findAll('[data-test="contrast-row"][data-pass="false"]')).toHaveLength(0)
  })

  it('shows every primitive, including each button variant and all feedback states', () => {
    const wrapper = mount(DesignView)
    expect(wrapper.findAll('[data-test="button-row"]')).toHaveLength(4)
    // Two ruler meters and three slot bars.
    expect(wrapper.findAll('[role="meter"]')).toHaveLength(5)
    expect(wrapper.find('table caption').exists()).toBe(true)
    expect(wrapper.text()).toContain('No automations yet')
    expect(wrapper.find('[role="alert"]').text()).toContain('Could not load runs')
    expect(wrapper.find('[role="status"]').exists()).toBe(true)
  })
})
