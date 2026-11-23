import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi } from './support/adminApi'

// The Briefs topic form: the region list and the words to skip sit level, the feeds box suggests well-known sources while the owner types (the list is
// not cut off by the card), and the browser and "My sources" fit at both widths. The data is invented; the shape is the server's.
const source = (label: string, category: string, target: string, over: Record<string, unknown> = {}) => ({ label, category, kind: 'feed', target, builtin: true, ...over })
const SOURCES = [
  source('BBC News', 'News', 'https://feeds.bbci.co.uk/news/rss.xml'), source('The Guardian World', 'News', 'https://www.theguardian.com/world/rss'),
  source('Ars Technica', 'Technology', 'https://feeds.arstechnica.com/arstechnica/index'), source('The Verge', 'Technology', 'https://www.theverge.com/rss/index.xml'),
  source('CNBC Top News', 'Business and finance', 'https://www.cnbc.com/id/100003114/device/rss/rss.html'),
  source('Reuters', 'Search a publisher', 'site:reuters.com', { kind: 'newssearch' }),
  source('Daily Blog', 'My sources', 'https://blog.example/feed.xml', { id: 9, builtin: false }),
]
const ANSWERS = {
  '/briefs/sources': SOURCES, '/briefs/editions': [], '/briefs/topics': [], '/briefs/issues': [],
  '/newsroom/sources/lookup': { found: true, title: 'Another Blog', sample: [{ title: 'One' }, { title: 'Two' }], choices: [] }, '/briefs/settings': { model_calls_per_day: 0, default: 200, max: 2000 }, '/newsroom/watches': [], '/newsroom/feed': [],
}

test('the topic form: region and skip words level, suggestions while typing, the browser and My sources', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: ANSWERS })
  await page.goto('newsroom/briefs')
  await page.getByRole('button', { name: 'Set up' }).click()
  await page.getByRole('button', { name: 'Add a topic' }).click()
  await page.getByLabel('Name', { exact: true }).first().fill('Financial')

  // The two fields of the last row start and end at the same height, whatever the skip box holds.
  const region = await page.getByLabel('News from').boundingBox()
  const skip = await page.getByRole('textbox', { name: 'Skip stories that mention (optional)' }).locator('xpath=ancestor::*[@data-test="chip-input"]').boundingBox()
  expect(region && skip).toBeTruthy()
  expect(Math.abs(region!.height - skip!.height)).toBeLessThanOrEqual(1)
  // On a phone the two fields stack, so only their heights are compared there.
  if (testInfo.project.name !== 'mobile') expect(Math.abs(region!.y - skip!.y)).toBeLessThanOrEqual(1)

  // Typing a site suggests it, and the list is drawn over the form, not cut off by the card.
  await page.getByRole('combobox', { name: 'Follow a website (optional)' }).fill('guard')
  const option = page.getByRole('option', { name: /The Guardian World/ })
  await expect(option).toBeVisible()
  await lookAt(page, testInfo, 'suggestions', true)
  const hit = await option.evaluate((el) => { const r = el.getBoundingClientRect(); return document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2) === el || el.contains(document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2)) })
  expect(hit, 'the suggestion is on top, not clipped').toBe(true)
  await option.click()
  await expect(page.getByRole('button', { name: 'Remove The Guardian World' })).toBeVisible()

  // A pasted link in neither list is read and offered for My sources, and fits the screen.
  const feeds = page.getByRole('combobox', { name: 'Follow a website (optional)' })
  await feeds.fill('https://another.example/feed.xml')
  await feeds.press('Enter')
  await expect(page.locator('[data-test="custom-feed"]')).toContainText('Reads fine: Another Blog · 2 recent stories')
  await expect(page.getByRole('button', { name: 'Save to my sources' })).toBeVisible()
  await expectNoSidewaysScroll(page)

  // The browser opens on the group that matches the topic's name.
  await page.locator('[data-test="source-browser"] summary').click()
  await expect(page.locator('[data-test="source-browser"] section').first()).toHaveAttribute('aria-label', 'Business and finance')
  await page.locator('[data-test="source-browser"] button', { hasText: 'Reuters' }).click()
  // A publisher search goes into the topic's searches, as the search itself.
  await expect(page.getByRole('button', { name: 'Remove site:reuters.com' })).toBeVisible()
  await lookAt(page, testInfo, 'browser', true)
  await expectNoSidewaysScroll(page)

  await page.getByRole('button', { name: 'Cancel' }).click()
  await expect(page.locator('[data-test="my-source"]')).toContainText('Daily Blog')
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'my-sources', true)
})

// The edition page: the news in the main column, Home and Your watches beside it on a wide screen and under it on a phone; a card holds a few stories and
// the rest wait behind "Show more"; nothing scrolls sideways. The stories are invented from the shape of a real edition.
const story = (id: number, headline: string, outlet: string, over: Record<string, unknown> = {}) => ({
  pool_id: id, headline, outlets: [outlet], posted: new Date(Date.now() - 2 * 3_600_000).toISOString(), headline_only: true, ...over,
})
const ECONOMY = [
  story(1, 'Stock Futures Rally as Dust Settles After AI-Driven Selloff', 'WSJ'),
  story(2, "Stock Market Outlook: Why the AI Selloff, Fed Rate Hike Fears, and Iran Tensions Threaten a Long Stretch of Falling Prices Across Every Major Index", "Barron's"),
  story(3, 'Asia AI rout puts governments in a bind', 'Breakingviews'),
  story(4, 'AI is making it hard for young people to find work, report says', 'NPR'),
  story(5, 'If the AI bubble bursts, it could create a serious problem for health care', 'STAT'),
  story(6, 'Oil slips as demand worries return', 'Reuters'),
  story(7, 'Bond yields edge higher ahead of the Fed meeting', 'FT'),
]
const EDITION_ANSWERS = {
  ...ANSWERS,
  '/briefs/editions': [{ id: 2, name: 'Morning', ready_by: '07:00', days: [0, 1, 2, 3, 4, 5, 6], collect_every: '2h0m0s', collect_for: '12h0m0s', enabled: true, watches: true, home: true, sections: [{ kind: 'news', topic_id: 1 }, { kind: 'news', topic_id: 2 }, { kind: 'news', topic_id: 3 }] }],
  '/briefs/issues': [{
    id: 5, edition_id: 2, name: 'Morning', made_at: new Date(Date.now() - 20 * 60_000).toISOString(), model_calls: 4,
    top: [story(1, 'Stock Futures Rally as Dust Settles After AI-Driven Selloff', 'WSJ', { summary: 'Futures rose after a sharp fall driven by fears about AI spending.', headline_only: false, outlets: ['WSJ', 'Barron\'s', 'Breakingviews'] }), ECONOMY[2]!],
    sections: [
      { kind: 'news', topic_id: 1, name: 'Economy and markets', category: 'Business', stories: ECONOMY, as_of: new Date(Date.now() - 2 * 60_000).toISOString() },
      { kind: 'news', topic_id: 2, name: 'Technology', category: 'Technology', stories: ECONOMY.slice(0, 5), as_of: new Date(Date.now() - 2 * 60_000).toISOString() },
      { kind: 'news', topic_id: 3, name: 'UK politics', category: 'Politics', stories: ECONOMY.slice(0, 2), as_of: new Date(Date.now() - 2 * 60_000).toISOString() },
    ],
    home: [
      { id: 'network_online', count: 9, words: 'Devices the network sees right now.' }, { id: 'network_not_marked_mine', count: 2, words: 'Devices on your network that you have not marked as yours.' },
      { id: 'radio_off', words: 'Radio is off, so nothing is said about the air.' },
    ],
    watches: [
      { watch: 'AI news', new: 2, headline: 'DeepSeek V4.1-Flash is still the latest model release it knows.', summary: 'Two new posts since the last edition.', link: 'https://example.com/a' },
      { watch: 'Prices', new: 0, problems: ['A Telegram channel has no public preview, so it cannot be read.'] },
    ],
  }],
}

test('the edition page: news beside Home and Your watches on a wide screen, under them on a phone, and cards that stay short', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: EDITION_ANSWERS })
  await page.goto('newsroom/briefs')
  const main = page.locator('[data-test="edition-main"]')
  const side = page.locator('[data-test="edition-side"]')
  await expect(main).toBeVisible()
  await expect(side.locator('[data-test="home-block"]')).toBeVisible()
  await expect(side.locator('[data-test="watches-block"]')).toBeVisible()
  const [m, s] = [await main.boundingBox(), await side.boundingBox()]
  expect(m && s).toBeTruthy()
  if (testInfo.project.name === 'mobile') {
    expect(s!.y, 'the side column follows the news').toBeGreaterThan(m!.y + m!.height - 1)
    expect(Math.abs(s!.width - m!.width)).toBeLessThanOrEqual(1)
  } else {
    expect(s!.x, 'the side column sits to the right').toBeGreaterThan(m!.x + m!.width - 1)
    expect(Math.abs(s!.y - m!.y)).toBeLessThanOrEqual(2)
    expect(s!.width).toBeGreaterThan(280)
  }
  // The two top stories are not repeated in their card; the card keeps its other five and shows three.
  const economy = page.locator('section[aria-label="Economy and markets"]')
  await expect(economy.locator('article')).toHaveCount(3)
  await economy.getByRole('button', { name: 'Show 2 more' }).click()
  await expect(economy.locator('article')).toHaveCount(5)
  await expect(economy).not.toContainText('Asia AI rout puts governments in a bind')
  await expect(economy.locator('[data-test="headlines-only"]')).toHaveCount(1)
  await expect(economy).not.toContainText('Headline only')
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'edition', true)
})
