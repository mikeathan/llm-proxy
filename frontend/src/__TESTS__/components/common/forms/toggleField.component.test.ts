import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ToggleField from '../../../../components/common/forms/ToggleField.vue'
import { MORE_ABOUT_THIS } from '../../../../constants/labels'

const props = { modelValue: false, label: 'Look for new devices', hint: 'One short sentence.' }

describe('ToggleField', () => {
  it('shows the short hint and offers no expander when there are no details', () => {
    const w = mount(ToggleField, { props })
    expect(w.text()).toContain('One short sentence.')
    expect(w.find('[data-test="toggle-details"]').exists()).toBe(false)
  })

  it('keeps the long text folded until the person opens it', async () => {
    const w = mount(ToggleField, { props: { ...props, details: 'The long explanation.' }, attachTo: document.body })
    const button = w.get('[data-test="toggle-details"]')
    expect(button.text()).toBe(MORE_ABOUT_THIS)
    expect(button.attributes('aria-expanded')).toBe('false')
    expect(w.get('[data-test="toggle-details-text"]').isVisible()).toBe(false)
    await button.trigger('click')
    expect(button.attributes('aria-expanded')).toBe('true')
    expect(w.get('[data-test="toggle-details-text"]').isVisible()).toBe(true)
    expect(w.get('[data-test="toggle-details-text"]').text()).toBe('The long explanation.')
    w.unmount()
  })

  it('links the expander to the text it opens, and the switch is not inside the expander button', () => {
    const w = mount(ToggleField, { props: { ...props, details: 'x' }, attachTo: document.body })
    const controls = w.get('[data-test="toggle-details"]').attributes('aria-controls')
    expect(controls && document.getElementById(controls)).toBeTruthy()
    expect(w.get('[data-test="toggle-details"]').find('[role="switch"]').exists()).toBe(false)
    w.unmount()
  })
})
