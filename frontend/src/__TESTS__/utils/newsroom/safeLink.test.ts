import { describe, it, expect } from 'vitest'
import { safeWebLink } from '../../../utils/newsroom/safeLink'

describe('safeWebLink', () => {
  it('keeps an absolute http or https address', () => {
    expect(safeWebLink('https://go.dev/blog/go1.26')).toBe('https://go.dev/blog/go1.26')
    expect(safeWebLink('http://example.com')).toBe('http://example.com/')
  })

  it.each(['javascript:alert(1)', 'data:text/html,<b>x</b>', 'ftp://example.com/a', '//evil.example/a', '/admin/api/stacks', 'not a url', '', undefined])(
    'refuses %s: a feed is the open internet, and a link must never run script or point into this app',
    (raw) => {
      expect(safeWebLink(raw)).toBeNull()
    },
  )
})
