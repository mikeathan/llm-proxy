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
