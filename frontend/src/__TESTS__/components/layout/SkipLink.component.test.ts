import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import SkipLink from '../../../components/layout/SkipLink.vue'

describe('SkipLink', () => {
  it('jumps to the page content, hidden until it has keyboard focus', () => {
    const a = mount(SkipLink, { props: { target: 'content' } }).get('a')
    expect(a.attributes('href')).toBe('#content')
    expect(a.text()).toBe('Skip to content')
    expect(a.classes()).toContain('sr-only')
    expect(a.classes()).toContain('focus:not-sr-only')
  })

  it('moves focus into the target, so the next Tab continues from there', async () => {
    document.body.innerHTML = '<main id="content" tabindex="-1">page</main>'
    const w = mount(SkipLink, { props: { target: 'content' }, attachTo: document.body })
    await w.get('a').trigger('click')
    expect(document.activeElement?.id).toBe('content')
    w.unmount()
  })
})
