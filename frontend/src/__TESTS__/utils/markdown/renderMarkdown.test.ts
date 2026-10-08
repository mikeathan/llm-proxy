import { describe, expect, it } from 'vitest'
import { renderMarkdown } from '../../../utils/markdown/renderMarkdown'

// Assistant and automation output is model-written, so it may carry hostile
// markup (echoed from a web page or file). Rendered markdown must never let it
// through as live HTML.
describe('renderMarkdown', () => {
  it('renders ordinary markdown', () => {
    const html = renderMarkdown('**bold** and `code`\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n[docs](https://example.com/x)')
    expect(html).toContain('<strong>bold</strong>')
    expect(html).toContain('<code>code</code>')
    expect(html).toContain('<table>')
    expect(html).toContain('href="https://example.com/x"')
  })

  it('wraps each table in a scroll container so wide tables never widen the page', () => {
    const html = renderMarkdown('| a | b |\n|---|---|\n| 1 | 2 |\n\ntext\n\n| c |\n|---|\n| 3 |')
    expect(html.match(/<div class="md-table-scroll"[^>]*><table>/g)).toHaveLength(2)
    expect(html.match(/<\/table><\/div>/g)).toHaveLength(2)
  })

  it('does not let literal table markup in text become a wrapper', () => {
    const html = renderMarkdown('`<table>` and <table><tr><td>x</td></tr></table>')
    expect(html).not.toContain('md-table-scroll')
    expect(html).not.toMatch(/<table>/)
  })

  it.each([
    '<img src=x onerror="alert(1)">',
    '<script>alert(1)</script>',
    'text <iframe src="https://evil"></iframe> more',
    '<div onclick="alert(1)">click</div>',
  ])('shows raw HTML as text: %s', (input) => {
    const html = renderMarkdown(input)
    expect(html).not.toMatch(/<(img|script|iframe|div)\b/i)
    expect(html).toContain('&lt;')
  })

  it.each([
    '[click](javascript:alert(1))',
    '[click](JaVaScRiPt:alert(1))',
    '![x](javascript:alert(1))',
    '[click](data:text/html;base64,PHNjcmlwdD4=)',
    '[click](vbscript:msgbox(1))',
  ])('drops unsafe link and image URLs: %s', (input) => {
    const html = renderMarkdown(input)
    expect(html.toLowerCase()).not.toMatch(/(javascript|vbscript|data):/)
  })

  it('keeps safe relative, anchor and mailto links', () => {
    expect(renderMarkdown('[a](./notes.md) [b](#top) [c](mailto:x@y.z)')).toMatch(/href="\.\/notes\.md".*href="#top".*href="mailto:x@y\.z"/)
  })
})
