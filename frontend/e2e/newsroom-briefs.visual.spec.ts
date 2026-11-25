import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, expectTapArea, lookAt, mockAdminApi } from './support/adminApi'

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

test('an empty page offers ready-made topics, and the brief form asks what goes in it', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: ANSWERS })
  await page.goto('newsroom/following')
  await expect(page.locator('[data-test="starter-gallery"]')).toBeVisible()
  await lookAt(page, testInfo, 'starters', true)
  await expectNoSidewaysScroll(page)
  // A brief can be only the owner's home, so the form opens with no topic at all.
  await page.getByRole('button', { name: 'Add an edition' }).click()
  await expect(page.locator('[data-test="edition-blocks"]')).toBeVisible()
  await lookAt(page, testInfo, 'brief-form-empty', true)
  await expectNoSidewaysScroll(page)
})

// Topics by section, and the brief form with sections as tags.
const TOPICS = [
  { id: 1, name: 'UK', category: 'News', queries: ['UK news'], sources: [{ kind: 'feed', label: 'BBC News', target: 'https://feeds.bbci.co.uk/news/rss.xml' }] },
  { id: 2, name: 'Greece', category: 'News', queries: ['Greece'] },
  { id: 3, name: 'War', category: 'News', queries: ['war'] },
  { id: 4, name: 'AI and chips', category: 'Technology', queries: ['AI chips', 'Nvidia'] },
]
test('topics sit under their section and the brief form lists the sections as tags', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { ...ANSWERS, '/briefs/topics': TOPICS } })
  await page.goto('newsroom/following')
  await expect(page.locator('[data-test="topic-section-group"]')).toHaveCount(2)
  await lookAt(page, testInfo, 'sections', true)
  await expectNoSidewaysScroll(page)
  await page.getByRole('button', { name: 'Add an edition' }).click()
  await page.locator('[data-test="block-news"] label').click()
  await expect(page.locator('[data-test="edition-sections"]')).toBeVisible()
  await lookAt(page, testInfo, 'brief-form', true)
  await expectNoSidewaysScroll(page)
})

test('the topic form: region and skip words level, suggestions while typing, the browser and My sources', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { ...ANSWERS, '/briefs/topics': [{ id: 1, name: 'Financial', category: 'News' }] } })
  await page.goto('newsroom/following')
  await page.getByRole('button', { name: 'Edit Financial' }).click()
  await lookAt(page, testInfo, 'topic-form', true)

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
const DAYS = Array.from({ length: 7 }, (_, i) => {
  const d = new Date(Date.now() - (6 - i) * 86_400_000)
  const date = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
  return { date, local: [90, 120, 80, 150, 170, 200, 214][i]!, cloud: [10, 22, 18, 30, 25, 41, 38][i]! }
})
const MODEL_STATS = {
  days: DAYS,
  hours: Array.from({ length: 24 }, (_, h) => ({ local: h >= 7 && h <= 20 ? 4 + ((h * 7) % 11) : 0, cloud: h >= 9 && h <= 17 ? (h * 3) % 5 : 0 })),
  today: [
    { model: 'Qwen3.5 35B A3B', local: true, calls: 128, failed: 2, prompt_tokens: 402_000, completion_tokens: 12_300, tokens_per_sec: 41.2 },
    { model: 'Qwen3.8 27B', local: true, calls: 61, failed: 0, prompt_tokens: 190_000, completion_tokens: 8_000, tokens_per_sec: 33.4 },
    { model: 'LFM2.5-24B-A2B', local: true, calls: 25, failed: 0, prompt_tokens: 58_000, completion_tokens: 2_100, tokens_per_sec: 58.9 },
    { model: 'deepseek-v4.1-flash', local: false, calls: 22, failed: 1, prompt_tokens: 140_000, completion_tokens: 30_000 },
    { model: 'nemotron-3.5-super-vl:free', local: false, calls: 16, failed: 0, prompt_tokens: 52_000, completion_tokens: 9_000 },
  ],
}
const EDITION_ANSWERS = {
  ...ANSWERS,
  '/model-stats': MODEL_STATS,
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
      { id: 'radio_open_or_weak_ap', count: 1, words: 'Confirmed access points of yours that are open or use an outdated protection (open, WEP or WPA).', level: 'critical' },
      { id: 'network_not_marked_mine', count: 2, words: 'Devices on your network that you have not marked as yours.', level: 'check' },
      { id: 'network_online', count: 9, words: 'Devices the network sees right now.', level: 'info' },
    ],
    watches: [
      { watch: 'AI news', new: 2, headline: 'DeepSeek V4.1-Flash is still the latest model release it knows.', summary: 'Two new posts since the last edition.', link: 'https://example.com/a' },
      { watch: 'Prices', new: 0, problems: ['A Telegram channel has no public preview, so it cannot be read.'] },
    ],
  }],
}

// The page asks for the brief it shows by id.
Object.assign(EDITION_ANSWERS, { '/briefs/issues/5': EDITION_ANSWERS['/briefs/issues'][0] })

test('the edition page: news beside Home and Your watches on a wide screen, between the top stories and the sections on a phone, and cards that stay short', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: EDITION_ANSWERS })
  await page.goto('newsroom')
  const main = page.locator('[data-test="edition-main"]')
  const side = page.locator('[data-test="edition-side"]')
  await expect(main).toBeVisible()
  await expect(side.locator('[data-test="home-block"]')).toBeVisible()
  await expect(side.locator('[data-test="watches-block"]')).toBeVisible()
  // On a phone the news column has no box of its own (it dissolves into the page's grid), so the width is compared with the whole body.
  const mobile = testInfo.project.name === 'mobile'
  const [m, s] = [await (mobile ? page.locator('[data-test="edition-body"]') : main).boundingBox(), await side.boundingBox()]
  expect(m && s).toBeTruthy()
  if (mobile) {
    // What needs a look is near the top of a phone: after the top stories, before the first section card.
    const [top, first] = [await page.locator('section[aria-label="Top stories"]').boundingBox(), await page.locator('section[aria-label="Economy and markets"]').boundingBox()]
    expect(top && first).toBeTruthy()
    expect(s!.y, 'the side column follows the top stories').toBeGreaterThan(top!.y + top!.height - 1)
    expect(s!.y + s!.height, 'and comes before the first section').toBeLessThanOrEqual(first!.y + 1)
    expect(Math.abs(s!.width - m!.width)).toBeLessThanOrEqual(1)
    // The head is short: Follow is a small outline that still answers a finger, and the first headline starts soon under the title.
    await page.evaluate(() => window.scrollTo(0, 0))
    await expect(page.locator('.route-enter-active')).toHaveCount(0) // measured after the page has slid in
    await expectTapArea(page.locator('[data-test="follow-open"]'))
    await expectTapArea(page.locator('[data-test="brief-more"]'))
    const [title, headline] = [await page.getByRole('heading', { name: 'Newsroom', level: 1 }).boundingBox(), await page.locator('[data-test="lead-story"]').boundingBox()]
    expect(headline!.y - title!.y).toBeLessThan(270)
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

// The model use block under Home: folded to one line with a small trend, then open into the week and today's models. Aligned inside its column on a phone and a
// wide screen, nothing scrolls sideways, and long model names wrap instead of pushing the call count off the card.
test('the model use block opens into the week and today by model, and stays inside its column', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: EDITION_ANSWERS })
  await page.goto('newsroom')
  const block = page.locator('[data-test="model-use-block"]')
  await expect(block).toBeVisible()
  await expect(block.locator('[data-test="model-use-total"]')).toHaveText('252 calls today')
  await expect(block.locator('[data-test="model-use-body"]')).toHaveCount(0)
  await lookAt(block, testInfo, 'model-use-folded')
  await block.locator('[data-test="model-use-toggle"]').click()
  await expect(block.locator('[data-test="model-use-column"]')).toHaveCount(7)
  await expect(block.locator('[data-test="model-use-row"]')).toHaveCount(5)
  await expect(block.locator('[data-test="model-use-hour"]')).toHaveCount(24)
  const [card, chart] = [await block.boundingBox(), await block.locator('[data-test="model-use-chart"]').boundingBox()]
  expect(card && chart).toBeTruthy()
  expect(chart!.x).toBeGreaterThanOrEqual(card!.x)
  expect(chart!.x + chart!.width).toBeLessThanOrEqual(card!.x + card!.width + 1)
  for (const row of await block.locator('[data-test="model-use-row"]').all()) {
    const box = await row.boundingBox()
    expect(box!.x + box!.width, 'a row stays inside the card').toBeLessThanOrEqual(card!.x + card!.width + 1)
  }
  await expectNoSidewaysScroll(page)
  await lookAt(block, testInfo, 'model-use-open')
})

// The edition page in every theme and in print: nothing scrolls sideways, the masthead counts outlets, a source problem shows its fix, and printing hides the buttons.
const PRESETS = ['retro-dark', 'retro-dark-soft', 'retro-dark-lifted', 'retro-midnight', 'retro-forest', 'retro-plum', 'retro-paper', 'retro-paper-cool', 'retro-sand']
const PROBLEM_ISSUE = {
  ...EDITION_ANSWERS['/briefs/issues'][0]!, outlets_read: 38,
  sections: EDITION_ANSWERS['/briefs/issues'][0]!.sections.map((sec, i) => (i === 0 ? { ...sec, problems: ['News: Greece: internal: the server did not answer with HTTP 200'] } : sec)),
}
const EARLIER = { ...PROBLEM_ISSUE, id: 4, made_at: new Date(Date.now() - 26 * 3_600_000).toISOString() }

test('the edition page in every theme and in print', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { ...EDITION_ANSWERS, '/briefs/issues': [PROBLEM_ISSUE, EARLIER], '/briefs/issues/5': PROBLEM_ISSUE, '/briefs/issues/4': EARLIER } })
  await page.goto('newsroom')
  await expect(page.locator('[data-test="edition"]')).toBeVisible()
  await expect(page.locator('[data-test="masthead"]')).toBeVisible()
  await expect(page.locator('[data-test="dateline"]', { hasText: 'read from 38 outlets' })).toBeVisible()
  await expect(page.locator('[data-test="problems-fix"]').first()).toBeVisible()
  await expect(page.locator('[data-test="dateline"] [data-test="earlier-briefs"] select')).toBeVisible()
  for (const preset of PRESETS) {
    await page.evaluate((id) => document.documentElement.setAttribute('data-theme', id), preset)
    await expectNoSidewaysScroll(page)
    await lookAt(page, testInfo, `edition-${preset}`, true)
  }
  await page.emulateMedia({ media: 'print' })
  await expect(page.locator('[data-test="problems-open-setup"]').first()).toBeHidden()
  await expect(page.locator('[data-test="earlier-briefs"]')).toBeHidden()
  await expect(page.locator('#app-sidebar')).toBeHidden()
  await expect(page.locator('[data-test="brief-more"]')).toBeHidden()
  await expect(page.locator('[data-test="dateline"]')).toContainText(String(new Date().getFullYear())) // the date prints in place of the picker
  await expect(page.locator('[data-test="index-row"]')).toBeHidden()
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'edition-print', true)
})

// The dashboard's Newsroom board: a big Brief card while one is written, a dot on the tab while it is unread, and tabs that switch the card's body in place. Home carries a short Today card.
const issueOf = EDITION_ANSWERS['/briefs/issues'][0]!
const LATEST = (read: boolean) => [{ edition_id: 2, name: 'Morning', issue_id: 5, made_at: issueOf.made_at, read }]
const STACKS_WITH_NEWSROOM = { stacks: ['network', 'agents', 'radio', 'reminders', 'newsroom'].map((id) => ({ id, title: id, purpose: '', on: 2, total: 4, stopped: 0, blocked: 0 })) }
const dashboardAnswers = (read: boolean) => ({ ...EDITION_ANSWERS, '/stacks': STACKS_WITH_NEWSROOM, '/dashboard': { boards: [] }, '/briefs/latest': LATEST(read), '/briefs/issues/5': issueOf })

test('the dashboard: a front-page Brief card with a dot on the tab while it is unread, whose headlines open the brief at that story', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: dashboardAnswers(false) })
  await page.goto('dashboard?board=newsroom')
  const card = page.locator('[data-test="brief-card"]')
  await expect(card.locator('[data-test="brief-card-front"]')).toBeVisible()
  await expect(card).toHaveAttribute('data-unread', 'true')
  await expect(page.locator('[data-test="board-tab-dot"]')).toHaveCount(1)
  await expect(card.locator('[data-test="brief-attention"]')).toBeVisible()
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'dashboard-brief-unread', true)
  // A headline opens the brief at its story: the page scrolls it into view.
  const headline = card.locator('[data-test="brief-card-columns"] [data-test="brief-headline"]').first()
  const text = (await headline.textContent())?.trim() ?? ''
  await headline.click()
  await expect(page).toHaveURL(/newsroom\?edition=2&story=\d+/)
  await expect(page.locator('[data-test="edition"] article', { hasText: text }).first()).toBeInViewport()
})

test('the dashboard: Home carries a short Today card with what needs a look first', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: dashboardAnswers(false) })
  await page.goto('dashboard')
  const card = page.locator('[data-test="today-card"]')
  await expect(card).toBeVisible()
  await expect(card.locator('[data-test="brief-attention"]')).toBeVisible()
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'dashboard-today', true)
})

test('the dashboard: no dot and no New tag once the brief is read', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: dashboardAnswers(true) })
  await page.goto('dashboard?board=newsroom')
  await expect(page.locator('[data-test="brief-card-front"]')).toBeVisible()
  await expect(page.locator('[data-test="board-tab-dot"]')).toHaveCount(0)
  await expect(page.locator('[data-test="brief-card"]')).not.toHaveAttribute('data-unread', 'true')
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'dashboard-brief-read', true)
})

test('opening the Briefs page reads the brief: the Newsroom tab loses its dot', async ({ page }) => {
  await mockAdminApi(page, { answers: dashboardAnswers(false) })
  const asked: string[] = []
  // The server keeps what was read: once it is told, it answers "read".
  await page.route('**/briefs/latest', (route) => route.fulfill({ json: LATEST(asked.length > 0) }))
  page.on('request', (r) => { if (r.method() === 'POST' && r.url().includes('/briefs/issues/')) asked.push(new URL(r.url()).pathname) })
  await page.goto('newsroom')
  await expect(page.locator('[data-test="edition"]')).toBeVisible()
  await expect.poll(() => asked.length).toBeGreaterThan(0)
  expect(asked[0]).toMatch(/\/briefs\/issues\/5\/read$/)
  await expect(page.locator('[data-test="tab-dot"]')).toHaveCount(0)
})

// The Follow form, opened from a story with its words, and a story's own actions: both fit the screen at either width.
test('a story can be tuned or followed, and the Follow form fits the screen', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { ...EDITION_ANSWERS, '/briefs/topics': TOPICS.map((t) => ({ ...t, id: t.id })), '/newsroom/sources/lookup': { found: true, sample: [{ title: 'Futures rise again' }], choices: [] } } })
  await page.goto('newsroom')
  const lead = page.locator('[data-test="lead-story"]')
  await expect(lead.locator('[data-test="story-actions"]')).toBeVisible()
  await lead.locator('[data-test="story-less"]').click()
  await expect(lead.locator('[data-test="story-tune"]')).toBeVisible()
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'story-tune')
  await lead.locator('[data-test="story-follow"]').click()
  await expect(page).toHaveURL(/newsroom\/following\?kind=alerts&follow=/)
  await expect(page.locator('[data-test="follow-form"]')).toBeVisible()
  await expect(page.locator('[data-test="follow-preview-list"]')).toContainText('Futures rise again')
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'follow-form', true)
})

// Alerts: every alert says "Not for me" in words with a finger-sized target, "Set up alerts" leads to the watches, and the watch form lists what the
// watch learned from the owner. The data is invented; the shape is the server's.
const ALERT_WATCH = {
  id: 1, name: 'AI news', interval: '1h0m0s', max_age: '72h0m0s', enabled: true, look_for: 'New model releases', last_status: 'nothing_new',
  sources: [{ kind: 'telegram', target: 'aipost' }], examples: { want: ['Qwen4 released with open weights'], skip: ['OpenAI: AGIpilled people be like', 'ClaudeAI: $500 in promo credits each week'] },
}
const alert = (id: number, headline: string) => ({ id, watch_id: 1, at: new Date().toISOString(), source: 'telegram aipost', title: headline, headline, link: `https://t.me/aipost/${id}` })
const ALERT_ANSWERS = {
  '/newsroom/watches': [ALERT_WATCH],
  '/newsroom/feed': [{ day: '2026-11-25', label: 'Today', updates: 2, unread: 1, watches: [{ watch_id: 1, watch: 'AI news', updates: 2, unread: 1, current: alert(2, 'A user says Fable 5.2 Medium reached a 2730 chess rating'), earlier: [{ ...alert(1, 'Baidu opens the weights of ERNIE 5.5 Flash Preview'), read: true }] }] }],
  '/newsroom/feed/unread': { count: 1, left_out: 3 },
}

test('alerts offer "Not for me" with a finger-sized target, and the watch form lists what the watch learned', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: ALERT_ANSWERS })
  await page.goto('newsroom/alerts')
  const notForMe = page.locator('[data-test="not-for-me"]').first()
  await expect(notForMe).toBeVisible()
  await expectTapArea(notForMe)
  await expect(page.locator('[data-test="alerts-setup"]')).toBeVisible()
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'alerts', true)
  await page.locator('[data-test="alerts-setup"]').click()
  await expect(page.locator('[data-test="new-watch"]')).toBeVisible()
  await expect(page.locator('[data-test="following-read"]')).toHaveText(/See alerts/)
  await page.getByRole('button', { name: 'Edit' }).click()
  const learned = page.locator('[data-test="watch-examples"]')
  await expect(learned).toContainText('OpenAI: AGIpilled people be like')
  await expectNoSidewaysScroll(page)
  await lookAt(learned, testInfo, 'watch-examples')
})
