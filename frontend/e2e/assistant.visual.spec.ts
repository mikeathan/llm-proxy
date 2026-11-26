import { expect, test, type Page } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi } from './support/adminApi'

// The assistant (plan assistant-calm-mobile): a chat opens at its latest message, a phone shows one bar, the
// conversation and a one-row message box, and a failed turn says what went wrong and what to do. Geometry is read from
// the page: happy-dom cannot measure any of it (audit 2026-11-26, U1-U4).
const NOW = new Date('2026-11-26T16:00:00Z')
const WEATHER = {
  id: 'conv_weather', workspace_id: 'workspace-1', timezone: 'Europe/London', updated_at: '2026-11-26T15:40:00Z',
  history: [
    { role: 'system', content: 'You are a helpful assistant.' },
    { role: 'user', content: 'What is the weather in Athens tomorrow, and should I bring an umbrella?', run: { model: 'Qwen3.6 35B A3B', duration_ms: 48_200, prompt_tokens: 9120, completion_tokens: 1430 } },
    { role: 'assistant', content: '', reasoning_content: 'Search first, then read an hourly page.', tool_calls: [{ id: 'c1', type: 'function', function: { name: 'internet_search', arguments: '{"query":"Athens weather 27 November hourly rain"}' } }] },
    { role: 'tool', tool_call_id: 'c1', content: '{"results":[{"title":"Athens hourly","url":"https://met.example/athens/hourly","snippet":"Rain from 14:00"}]}' },
    { role: 'assistant', content: '', tool_calls: [{ id: 'c2', type: 'function', function: { name: 'fetch_url', arguments: '{"url":"https://met.example/athens/hourly"}' } }] },
    { role: 'tool', tool_call_id: 'c2', content: '14:00 17°C showers 70%' },
    { role: 'assistant', content: '**Yes, take an umbrella.**\n\n| Time | Temp | Sky | Rain |\n|---|---|---|---|\n| Morning | 13–16 °C | Cloudy | 10–30% |\n| 14:00–19:00 | 15–17 °C | Showers | 70–80% |\n| Evening | 12 °C | Cloudy | 20% |\n\nThe morning stays dry; showers are likely from 2 pm to about 7 pm.' },
    { role: 'user', content: 'Remember that I prefer Celsius and short answers.', run: { model: 'Qwen3.6 35B A3B', duration_ms: 6100, completion_tokens: 80, memory_saved: ['Prefers Celsius', 'Prefers short answers'] } },
    { role: 'assistant', content: 'Noted: Celsius and short answers from now on.' },
    { role: 'user', content: 'when will opus 5.6 be released', run: { model: 'glm-5.3-flash', duration_ms: 289 } },
    { role: 'assistant', content: '', error: 'The model server rejected the request (HTTP 400): Request is missing x-opencode-session and cannot be routed efficiently.' },
  ],
}
const SESSIONS = [{ id: 'conv_weather', snippet: 'What is the weather in Athens tomorrow…', updated_at: '2026-11-26T15:40:00Z', source: 'manual' }]

async function open(page: Page, path = 'workspaces/workspace-1/assistant/conv_weather') {
  await page.clock.setFixedTime(NOW)
  await mockAdminApi(page, {
    answers: {
      '/dispatcher/workspaces': [{ id: 'workspace-1' }],
      '/conversation/sessions/workspace-1': SESSIONS,
      '/conversation/sessions/workspace-1/conv_weather': WEATHER,
      '/active-runs': { holders: [], queued: [], lanes: [] },
    },
  })
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.goto(path)
  await page.evaluate(() => document.fonts.ready)
}

const scroller = (page: Page) => page.locator('.message-container')
const distanceFromBottom = (page: Page) =>
  scroller(page).evaluate((el) => el.scrollHeight - el.scrollTop - el.clientHeight)

test('a chat opens at its latest message', async ({ page }, info) => {
  await open(page)
  await expect(page.locator('[data-test="run-failure"]')).toBeVisible()
  await expect.poll(() => distanceFromBottom(page)).toBeLessThan(4)
  await expectNoSidewaysScroll(page)
  await lookAt(page, info, 'assistant-chat')
})

test('a failed turn says what went wrong and what to do, the raw error behind Details', async ({ page }) => {
  await open(page)
  const failure = page.locator('[data-test="run-failure"]')
  await expect(failure).toContainText("The model's provider turned the request down.")
  await expect(failure.getByText('x-opencode-session')).toHaveCount(0)
  await failure.getByRole('button', { name: 'Details' }).click()
  await expect(failure.locator('[data-test="run-failure-details"]')).toContainText('x-opencode-session')
})

test('the run record under the latest answer is not clipped', async ({ page }, info) => {
  test.skip(info.project.name !== 'mobile', 'a pointer shows it on hover only')
  await open(page)
  const meta = page.locator('.turn-meta').last()
  await expect(meta).toBeVisible()
  const metaBox = (await meta.boundingBox())!
  const colBox = (await page.locator('.turn-group').last().boundingBox())!
  expect(metaBox.x).toBeGreaterThanOrEqual(colBox.x)
})

test('a phone shows one bar, the conversation and a one-row message box', async ({ page }, info) => {
  test.skip(info.project.name !== 'mobile', 'phone layout')
  await open(page)
  await expect(page.locator('[data-test="chat-bar-title"]')).toContainText('What is the weather in Athens')
  // The top strip steps aside on this page: the chat's bar is the first thing on screen.
  await expect(page.getByRole('banner')).toHaveCount(0)
  const bar = (await page.locator('.chat-header').boundingBox())!
  expect(bar.y).toBeLessThan(2)
  // The message box is one row: + , the box and send share a line.
  const plus = (await page.getByRole('button', { name: 'Commands and memory' }).boundingBox())!
  const send = (await page.getByRole('button', { name: 'Send', exact: true }).boundingBox())!
  expect(Math.abs(plus.y + plus.height / 2 - (send.y + send.height / 2))).toBeLessThan(4)
  // The conversation gets most of the screen.
  const pane = (await scroller(page).boundingBox())!
  expect(pane.height / page.viewportSize()!.height).toBeGreaterThan(0.7)
  await lookAt(page, info, 'assistant-phone')
})

test('the + menu holds commands and memory review', async ({ page }, info) => {
  test.skip(info.project.name !== 'mobile', 'phone layout')
  await open(page)
  await page.getByRole('button', { name: 'Commands and memory' }).click()
  const panel = page.getByRole('dialog', { name: 'Commands and memory' })
  await expect(panel.getByRole('button', { name: /Start a command/ })).toBeVisible()
  await expect(panel.getByRole('button', { name: /Review memory/ })).toBeVisible()
  await lookAt(page, info, 'assistant-phone-more')
})

test('the chat uses the width of the page on a desktop', async ({ page }, info) => {
  test.skip(info.project.name !== 'desktop', 'desktop layout')
  await open(page)
  const pane = (await scroller(page).boundingBox())!
  const answer = (await page.locator('.turn-answer').first().boundingBox())!
  expect(answer.width / pane.width).toBeGreaterThan(0.85)
  await lookAt(page, info, 'assistant-desktop')
})

test('a new chat offers starter prompts that fill the message box', async ({ page }) => {
  await open(page, 'workspaces/workspace-1/assistant')
  const starter = page.locator('.chat-empty-starter').first()
  const text = (await starter.textContent())!.trim()
  await starter.click()
  await expect(page.getByRole('textbox', { name: 'Message the assistant' })).toHaveValue(text)
})

// Tapping Send blurs the message box first and the tab bar comes back after it; the tap must still send. (Chrome's touch
// emulation lands the tap even with no delay before the bar returns, so this proves the tap works, not that the delay in
// useComposerFocus is needed; the delay is kept as a guard for real devices.)
test.describe('on a touch phone', () => {
  test.use({ hasTouch: true, isMobile: true })

  test('a tap on Send sends, and a tap on + opens its menu', async ({ page }, info) => {
    test.skip(info.project.name !== 'mobile', 'phone layout')
    await open(page)
    const box = page.getByRole('textbox', { name: 'Message the assistant' })
    await box.tap()
    await box.fill('is it still raining?')
    await page.getByRole('button', { name: 'Send', exact: true }).tap()
    // The message left the box and joined the conversation: the tap landed on Send.
    await expect(box).toHaveValue('')
    await expect(page.locator('.turn-group').last()).toContainText('is it still raining?')

    await box.tap()
    await page.getByRole('button', { name: 'Commands and memory' }).tap()
    await expect(page.getByRole('dialog', { name: 'Commands and memory' })).toBeVisible()
  })
})
