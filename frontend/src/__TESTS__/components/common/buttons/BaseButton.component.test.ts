import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BaseButton from '../../../../components/common/buttons/BaseButton.vue'

const stubs = { Icon: { props: ['name'], template: '<i :data-icon="name" />' } }

describe('BaseButton', () => {
  it('renders its label, emits click and defaults to a non-submitting button', async () => {
    const w = mount(BaseButton, { slots: { default: 'Save' }, global: { stubs } })
    expect(w.text()).toBe('Save')
    expect(w.attributes('type')).toBe('button')
    await w.trigger('click')
    expect(w.emitted('click')).toHaveLength(1)
  })

  it.each(['primary', 'secondary', 'ghost', 'danger'] as const)('supports the %s variant', (variant) => {
    const w = mount(BaseButton, { props: { variant }, slots: { default: 'Go' }, global: { stubs } })
    expect(w.attributes('data-variant')).toBe(variant)
  })

  it('is disabled and busy while loading', () => {
    const w = mount(BaseButton, { props: { loading: true }, slots: { default: 'Save' }, global: { stubs } })
    expect(w.attributes('disabled')).toBeDefined()
    expect(w.attributes('aria-busy')).toBe('true')
  })

  it('names an icon-only button from its label', () => {
    const w = mount(BaseButton, { props: { icon: 'close', iconOnly: true, label: 'Close panel' }, global: { stubs } })
    expect(w.attributes('aria-label')).toBe('Close panel')
    expect(w.attributes('title')).toBe('Close panel')
    expect(w.find('[data-icon="close"]').exists()).toBe(true)
    expect(w.text()).toBe('')
  })
})

// Icon-only buttons must carry an accessible name: every `iconOnly` usage in
// the source passes `label` (plan Phase 5, BaseButton).
const sources = import.meta.glob<string>(['../../../../**/*.vue', '!../../../../__TESTS__/**'], { query: '?raw', import: 'default', eager: true })

describe('icon-only BaseButton usages', () => {
  it('all pass a label', () => {
    const offenders: string[] = []
    for (const [file, source] of Object.entries(sources)) {
      for (const tag of source.match(/<BaseButton\b[^>]*>/gs) ?? []) {
        if (/\biconOnly\b/.test(tag) && !/\blabel=/.test(tag)) offenders.push(file)
      }
    }
    expect(offenders).toEqual([])
  })
})
