import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import MarkdownViewer from '../../../../components/common/display/MarkdownViewer.vue'

describe('MarkdownViewer', () => {
  it('reads as a document by default: answers and reports', () => {
    const w = mount(MarkdownViewer, { props: { content: '## Status\n\nAll **good**.' } })
    expect(w.classes()).toContain('markdown-body--document')
    expect(w.find('h2').text()).toBe('Status')
    expect(w.find('strong').text()).toBe('good')
  })

  it('has a compact variant for reasoning inside the step timeline', () => {
    const w = mount(MarkdownViewer, { props: { content: 'thinking…', variant: 'compact' } })
    expect(w.classes()).toContain('markdown-body--compact')
    expect(w.classes()).not.toContain('markdown-body--document')
  })
})
