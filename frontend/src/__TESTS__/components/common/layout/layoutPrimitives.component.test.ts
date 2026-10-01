import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import Panel from '../../../../components/common/layout/Panel.vue'
import PageHeader from '../../../../components/common/layout/PageHeader.vue'
import SectionHeading from '../../../../components/common/layout/SectionHeading.vue'

describe('SectionHeading', () => {
  it('renders the heading at the requested level with a decorative index', () => {
    const w = mount(SectionHeading, { props: { level: 3 }, slots: { default: 'Health' } })
    expect(w.element.tagName).toBe('H3')
    expect(w.text()).toBe('Health')
    expect(w.findAll('[aria-hidden="true"]')).toHaveLength(2)
  })
})

describe('Panel', () => {
  it('is a section labelled by its numbered title, with actions and a body', () => {
    const w = mount(Panel, { props: { title: 'Lanes' }, slots: { actions: '<button>Refresh</button>', default: '<p>body</p>' } })
    const section = w.get('section')
    const heading = w.get(`#${section.attributes('aria-labelledby')}`)
    expect(heading.text()).toBe('Lanes')
    expect(w.text()).toContain('Refresh')
    expect(w.text()).toContain('body')
  })

  it('can keep the title case (file paths)', () => {
    const upper = mount(Panel, { props: { title: 'Notes.md' } })
    expect(upper.get('h2').classes()).toContain('uppercase')
    const kept = mount(Panel, { props: { title: 'Notes.md', preserveCase: true } })
    expect(kept.get('h2').classes()).not.toContain('uppercase')
  })

  it('omits the header without a title and drops body padding when flush', () => {
    const w = mount(Panel, { props: { flush: true }, slots: { default: '<table />' } })
    expect(w.find('header').exists()).toBe(false)
    expect(w.get('[data-test="panel-body"]').classes()).not.toContain('p-4')
  })
})

describe('PageHeader', () => {
  it('renders the page title as the h1 with eyebrow, subtitle and actions', () => {
    const w = mount(PageHeader, {
      props: { title: 'Overview', eyebrow: 'Health', subtitle: 'What is running now' },
      slots: { actions: '<button>New</button>' },
    })
    expect(w.get('h1').text()).toBe('Overview')
    expect(w.text()).toContain('Health')
    expect(w.text()).toContain('What is running now')
    expect(w.text()).toContain('New')
  })
})
