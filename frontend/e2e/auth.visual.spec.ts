import { expect, test, type Page } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi } from './support/adminApi'

// Sign-in at both widths: the login and setup pages, the "not set up" banner, and Settings · Sign-in. Nothing may scroll
// sideways and the buttons are comfortable touch targets. The screenshots are for looking at; they are not compared.
const SIGNED_OUT = { required: true, setup_open: false, authenticated: false, elevated: false, encrypted: false }
const NOT_SET_UP = { required: false, setup_open: false, authenticated: false, elevated: false, encrypted: false }
const SETUP_OPEN = { ...NOT_SET_UP, setup_open: true }
const SIGNED_IN = { required: true, setup_open: false, authenticated: true, elevated: false, encrypted: true }

const sessions = [
  { id: 'me', browser: 'Firefox on Mac', address: '192.168.50.20', created_at: '2026-10-29T08:00:00Z', last_seen: '2026-10-29T09:00:00Z', current: true },
  { id: 'phone', browser: 'Safari on iPhone', address: '192.168.50.31', created_at: '2026-10-28T08:00:00Z', last_seen: '2026-10-29T07:00:00Z', current: false },
]
const tokens = [
  { id: 't1', name: 'eval scripts', scope: 'admin', created_at: '2026-10-28T10:00:00Z', last_used: '2026-10-29T08:30:00Z' },
  { id: 't2', name: 'the Mac', scope: 'inference', created_at: '2026-10-27T10:00:00Z' },
]
const events = [
  { at: '2026-10-29T08:00:00Z', kind: 'login', address: '192.168.50.20' },
  { at: '2026-10-29T08:05:00Z', kind: 'login_failed', address: '192.168.50.99' },
]

async function open(page: Page, path: string, status: object, extra: Record<string, unknown> = {}) {
  await mockAdminApi(page, { answers: { '/auth/status': status, ...extra } })
  await page.goto(path)
}

test('the login page fits the screen, and a signed-out visit lands on it', async ({ page }, testInfo) => {
  await open(page, 'overview', SIGNED_OUT)
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible()
  await expect(page).toHaveURL(/\/login\?next=/)
  await expectNoSidewaysScroll(page)
  if (testInfo.project.name === 'mobile') {
    const button = await page.getByRole('button', { name: 'Sign in' }).boundingBox()
    expect(button?.height).toBeGreaterThanOrEqual(44)
  }
  await lookAt(page, testInfo, 'login.png', true)
})

test('the setup page fits the screen', async ({ page }, testInfo) => {
  await open(page, 'setup', SETUP_OPEN)
  await expect(page.getByRole('heading', { name: 'Set up sign-in' })).toBeVisible()
  await page.getByLabel('New passphrase').fill('short')
  await expect(page.getByText('Use at least 12 characters.')).toBeVisible()
  await expectNoSidewaysScroll(page)
  if (testInfo.project.name === 'mobile') {
    const button = await page.getByRole('button', { name: 'Set up sign-in' }).boundingBox()
    expect(button?.height).toBeGreaterThanOrEqual(44)
  }
  await lookAt(page, testInfo, 'setup.png', true)
})

test('while sign-in is not set up, every page carries the banner', async ({ page }, testInfo) => {
  await open(page, 'overview', NOT_SET_UP)
  await expect(page.locator('[data-test="auth-banner"]')).toContainText('Sign-in is not set up')
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'banner.png')
})

test('Settings · Sign-in lists sessions and script tokens and fits the screen', async ({ page }, testInfo) => {
  await open(page, 'settings/sign-in', SIGNED_IN, { '/auth/sessions': sessions, '/auth/tokens': tokens, '/auth/events': events })
  await expect(page.getByRole('heading', { name: 'Where you are signed in' })).toBeVisible()
  await expect(page.getByText('this one')).toBeVisible()
  await expect(page.getByText('eval scripts')).toBeVisible()
  await expect(page.getByText('A wrong passphrase was tried')).toBeVisible()
  await expectNoSidewaysScroll(page)
  if (testInfo.project.name === 'mobile') {
    for (const name of ['Sign out', 'Make token', 'Change passphrase']) {
      const box = await page.getByRole('button', { name }).first().boundingBox()
      expect(box?.height, name).toBeGreaterThanOrEqual(44)
    }
  }
  await lookAt(page, testInfo, 'sign-in-settings.png', true)
})
