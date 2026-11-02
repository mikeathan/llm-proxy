import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import CollapsibleCard from '../../../../components/common/layout/CollapsibleCard.vue'

const card = (open: boolean, extra: Record<string, unknown> = {}) =>
  mount(CollapsibleCard, { props: { open, label: 'Thing', ...extra }, slots: { title: '<b>Title</b>', aside: '<button data-test="aside">⋯</button>', default: '<p data-test="body">Body</p>' } })

describe('CollapsibleCard', () => {
  it('shows its body while open and not while folded', () => {
    expect(card(true).find('[data-test="body"]').exists()).toBe(true)
    expect(card(false).find('[data-test="body"]').exists()).toBe(false)
  })

  it('asks to be folded or opened from its header, and says which it is', async () => {
    const open = card(true)
    const toggle = open.get('[data-test="card-toggle"]')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    await toggle.trigger('click')
    expect(open.emitted('update:open')).toEqual([[false]])
    const folded = card(false)
    expect(folded.get('[data-test="card-toggle"]').attributes('aria-expanded')).toBe('false')
    await folded.get('[data-test="card-toggle"]').trigger('click')
    expect(folded.emitted('update:open')).toEqual([[true]])
  })

  it('keeps the aside beside the toggle, never inside it, so a control is not nested in a button', () => {
    const w = card(true)
    expect(w.get('[data-test="card-toggle"]').find('[data-test="aside"]').exists()).toBe(false)
    expect(w.find('[data-test="aside"]').exists()).toBe(true)
  })

  it('is one neutral surface: no coloured edge or tint, so state comes only from the tags in its title', () => {
    const classes = card(true).get('article').classes()
    expect(classes).toEqual(expect.arrayContaining(['border', 'border-hairline', 'bg-surface']))
    expect(classes.some((c) => c.startsWith('border-l') || c.startsWith('border-state') || c.includes('state-'))).toBe(false)
  })

  it('names the card for assistive technology', () => {
    expect(card(true).get('article').attributes('aria-label')).toBe('Thing')
  })
})
