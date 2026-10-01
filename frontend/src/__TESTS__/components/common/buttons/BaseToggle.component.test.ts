import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BaseToggle from '../../../../components/common/buttons/BaseToggle.vue'

describe('BaseToggle', () => {
  it('is a native checkbox switch named by its label', () => {
    const w = mount(BaseToggle, { props: { modelValue: true, label: 'Run logging' } })
    const input = w.get('input')
    expect(input.attributes('type')).toBe('checkbox')
    expect(input.attributes('role')).toBe('switch')
    expect((input.element as HTMLInputElement).checked).toBe(true)
    expect(w.get('label').text()).toBe('Run logging')
  })

  it('emits the new state when changed, from click or keyboard', async () => {
    const w = mount(BaseToggle, { props: { modelValue: false, label: 'Prefill' } })
    await w.get('input').setValue(true)
    expect(w.emitted('update:modelValue')).toEqual([[true]])
  })

  it('keeps a hidden label as the accessible name', () => {
    const w = mount(BaseToggle, { props: { modelValue: false, label: 'Sandboxing master', hideLabel: true } })
    expect(w.get('label span.sr-only').text()).toBe('Sandboxing master')
  })

  it('does not change while disabled', () => {
    const w = mount(BaseToggle, { props: { modelValue: false, label: 'Prefill', disabled: true } })
    expect(w.get('input').attributes('disabled')).toBeDefined()
  })
})
