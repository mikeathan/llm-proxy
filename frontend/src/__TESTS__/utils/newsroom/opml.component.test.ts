import { describe, expect, it } from 'vitest'
import { opmlOf, parseOpml } from '../../../utils/newsroom/opml'

describe('opml', () => {
  it('reads the feeds of an OPML file, nested or not, each once, and leaves out what is not a feed address', () => {
    const file = `<?xml version="1.0"?><opml version="2.0"><body>
      <outline text="News"><outline type="rss" text="BBC" xmlUrl="https://feeds.bbci.co.uk/news/rss.xml"/></outline>
      <outline title="Blog" xmlUrl="https://blog.example/feed.xml"/>
      <outline title="Again" xmlUrl="https://blog.example/feed.xml"/>
      <outline title="Bad" xmlUrl="javascript:alert(1)"/>
      <outline xmlUrl="https://untitled.example/rss"/>
    </body></opml>`
    expect(parseOpml(file)).toEqual([
      { label: 'BBC', target: 'https://feeds.bbci.co.uk/news/rss.xml' },
      { label: 'Blog', target: 'https://blog.example/feed.xml' },
      { label: 'untitled.example', target: 'https://untitled.example/rss' },
    ])
    expect(parseOpml('not xml at all <')).toEqual([])
  })

  it('writes the owner\'s own feeds, escaped, and reads back what it wrote', () => {
    const text = opmlOf([
      { id: 1, label: 'Tom & Jerry "news"', category: 'My sources', kind: 'feed', target: 'https://a.example/rss?x=1&y=2', builtin: false },
      { label: 'BBC', category: 'News', kind: 'feed', target: 'https://bbc.example', builtin: true },
    ])
    expect(text).not.toContain('bbc.example')
    expect(parseOpml(text)).toEqual([{ label: 'Tom & Jerry "news"', target: 'https://a.example/rss?x=1&y=2' }])
  })
})
