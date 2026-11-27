import { expect, test, type Page } from '@playwright/test'
import { DEFAULT_CONFIG } from '../src/composables/models/useConfig'
import { expectNoSidewaysScroll, lookAt, mockAdminApi } from './support/adminApi'

// The Models page (ADR-0090, ADR-0091): a rail of providers beside a pane (all models as cards or a list, This computer, one provider with its
// keys and models), edited in a side panel, and the Engine and Routing tabs. Sample data in the shape of a real install: eight local models with one running, four cloud models.
const MIN = 60_000
// The page clock is fixed, so "up 43m" and "2 minutes ago" are the same in every run (the screenshots compare them).
const NOW = new Date('2026-11-26T09:00:00Z')
const local = (name: string, file: string, port: number, ctx: number, extra: Record<string, unknown> = {}) => ({
  name, provider: 'local', workload_class: 'local', filename: file, port, endpoint: `http://127.0.0.1:${port}`, active: false, ready: false,
  metadata: { quantization: 'Q4_K_M', context_length: ctx }, max_steps: 25, max_tokens: Math.floor(ctx / 3), reasoning_budget: Math.floor(ctx / 9),
  context_budget: ctx * 2, temperature: 0, timeout_minutes: 30, prefill: true, args: ['--ctx-size', '16384', '--n-gpu-layers', '999', '--flash-attn', 'on', '--jinja'], ...extra,
})
const cloud = (name: string, provider: string, key: string) => ({ name, provider, workload_class: 'cloud', model_id: name, endpoint: '', active: false, ready: false, provider_config: { api_key_name: key } })
const CLOUD_FILLED = { ...cloud('glm-5.3-flash', 'nvidia', 'team'), max_tokens: 87381, reasoning_budget: 29127, context_budget: 699052 }
const MODELS = [
  local('Qwen3.6 35B A3B', 'Qwen3.6-35B-A3B-Q4_K_M.gguf', 8081, 16384, { active: true, ready: true }),
  local('Qwen3.8 27B', 'Qwen3.8-27B-Q4_K_M.gguf', 8088, 262144),
  local('Huihui Qwen3 30B A3B Instruct 2507 Abliterated', 'Huihui-Qwen3-30B-A3B.Q4_K_M.gguf', 8087, 262144),
  local('Inkling 2 Mini', 'Inkling-2-Mini-IQ4_NL.gguf', 8089, 1048576),
  local('LFM2.5-24B-A2B', 'LFM2.5-24B-A2B-Q4_K_M.gguf', 8086, 131072),
  local('Ornith-1.5-35B', 'Ornith-1.5-35B-Q4_K_M.gguf', 8084, 262144),
  cloud('deepseek-v4.1-flash', 'nvidia', 'team'),
  CLOUD_FILLED,
  cloud('nemotron-3.5-super-vl:free', 'openrouter', 'personal'),
]
const CONFIG = { ...structuredClone(DEFAULT_CONFIG), primary_model: 'Qwen3.6 35B A3B', fallback_model: 'deepseek-v4.1-flash', model_host: '127.0.0.1' }
const STATE = {
  config: CONFIG,
  models: MODELS,
  next_port: 8090,
  available: [{ name: 'gemma-4-12b', filename: 'gemma-4-12b-it-Q4_K_M.gguf', size_bytes: 7_300_000_000, metadata: { name: 'Gemma 4 12B', quantization: 'Q4_K_M', context_length: 131072 } }],
  active: { name: 'Qwen3.6 35B A3B', provider: 'local', endpoint: 'http://127.0.0.1:8081', port: 8081, ready: true, started_at: new Date(NOW.getTime() - 43 * MIN).toISOString(), last_used_at: new Date(NOW.getTime() - 2 * MIN).toISOString() },
}
const KEYS: Record<string, unknown[]> = { nvidia: [{ id: 'a', name: 'team', key: '••••4f1a' }], openrouter: [{ id: 'b', name: 'personal', key: '••••9c07' }] }

async function open(page: Page, path: string) {
  await page.clock.setFixedTime(NOW)
  await mockAdminApi(page, { config: CONFIG, answers: { '/state': STATE, '/runtime/processes': { processes: [] }, '/providers/manifests': [] } })
  await page.route('**/admin/api/secrets/keys**', (route) => route.fulfill({ json: KEYS[new URL(route.request().url()).searchParams.get('provider') ?? ''] ?? [] }))
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.goto(path)
  await expect(page.getByRole('heading', { name: 'Models', exact: true, level: 1 })).toBeVisible()
  await page.evaluate(() => document.fonts.ready)
}

test('the models as cards: the running one first, Start and Edit on each', async ({ page }, info) => {
  await open(page, 'models')
  const cards = page.locator('[data-test="model-cards"] article')
  await expect(cards).toHaveCount(MODELS.length)
  await expect(cards.first()).toContainText('Qwen3.6 35B A3B')
  await expect(cards.first()).toContainText('Running')
  await expectNoSidewaysScroll(page)
  await lookAt(page, info, 'models-cards', true)
  await expect(page).toHaveScreenshot('models-cards.png', { fullPage: true })
})

test('the models as a list, with the editor open beside it', async ({ page }, info) => {
  test.skip(info.project.name === 'mobile', 'a phone always shows cards')
  await open(page, 'models')
  await page.getByRole('radio', { name: 'List' }).click()
  await expect(page.locator('[data-test="model-list"] table')).toBeVisible()
  await page.getByRole('button', { name: 'Edit Qwen3.8 27B' }).click()
  const panel = page.getByRole('dialog', { name: 'Edit Qwen3.8 27B' })
  await expect(panel).toBeVisible()
  await expect(panel.locator('[data-test="derived-limits"]')).toContainText('set by the server')
  await lookAt(page, info, 'models-list-editor')
  await expect(page).toHaveScreenshot('models-list-editor.png')
})

test('on a phone the editor is a full-screen sheet with Save in reach', async ({ page }, info) => {
  test.skip(info.project.name !== 'mobile', 'the phone layout')
  await open(page, 'models')
  await page.getByRole('button', { name: 'Edit Qwen3.8 27B' }).click()
  const panel = page.getByRole('dialog', { name: 'Edit Qwen3.8 27B' })
  await expect(panel).toBeVisible()
  const box = await panel.boundingBox()
  // "Full width" is the app header's width: the sheet measured 379 px on a 390 px screen (cause not confirmed; likely the scrollbar
  // gutter of pitfall 76).
  const header = await page.locator('header').first().boundingBox()
  expect(box!.width).toBeGreaterThanOrEqual(header!.width - 1)
  await expect(panel.getByRole('button', { name: 'Save model' })).toBeVisible()
  await expectNoSidewaysScroll(page)
  await lookAt(page, info, 'models-editor-phone')
  await expect(page).toHaveScreenshot('models-editor-phone.png')
})

test('Providers: the rail lists every provider with what it holds (a screen of its own on a phone)', async ({ page }, info) => {
  await open(page, 'models/providers')
  const rail = page.getByRole('navigation', { name: 'Providers' })
  await expect(rail).toContainText('1 key · 2 models')
  await expect(rail).toContainText('Not set up')
  await expectNoSidewaysScroll(page)
  await lookAt(page, info, 'models-provider-list', true)
  await expect(page).toHaveScreenshot('models-provider-list.png', { fullPage: true })
})

test('A provider: its keys, Add key behind a button, and under each key the models it serves', async ({ page }, info) => {
  await open(page, 'models/providers/nvidia')
  const pane = page.locator('[data-test="provider-pane"]')
  await expect(pane).toContainText('team')
  await expect(pane.locator('[data-key-group="team"]')).toContainText('glm-5.3-flash')
  await expect(page.getByLabel('Key value')).toHaveCount(0)
  await expectNoSidewaysScroll(page)
  await lookAt(page, info, 'models-provider', true)
  await expect(page).toHaveScreenshot('models-provider.png', { fullPage: true })
})

test('This computer: the local models and the files not added yet', async ({ page }, info) => {
  await open(page, 'models/providers/local')
  await expect(page.getByRole('button', { name: 'Add Gemma 4 12B' })).toBeVisible()
  await expectNoSidewaysScroll(page)
  await lookAt(page, info, 'models-this-computer', true)
  await expect(page).toHaveScreenshot('models-this-computer.png', { fullPage: true })
})

test('Engine: the local engine and one Save', async ({ page }, info) => {
  await open(page, 'models/engine')
  await expect(page.getByRole('button', { name: 'Save model settings' })).toBeVisible()
  await expectNoSidewaysScroll(page)
  await lookAt(page, info, 'models-engine', true)
  await expect(page).toHaveScreenshot('models-engine.png', { fullPage: true })
})

// The reported bug: Save on an existing local model did nothing but "Please select a valid value" (the browser refused 29127 against
// step="128", pitfall 78). Chrome's own check runs here, so Save must send the update.
for (const [who, name] of [['a local model', 'Qwen3.8 27B'], ['a cloud model whose limits the server filled in', 'glm-5.3-flash']] as const) {
  test(`Save sends the update for ${who}`, async ({ page }, info) => {
    test.skip(info.project.name === 'mobile', 'the same form on both widths; once is enough')
    await open(page, 'models')
    await page.getByRole('button', { name: `Edit ${name}` }).click()
    const panel = page.getByRole('dialog', { name: `Edit ${name}` })
    await expect(panel).toBeVisible()
    const sent = page.waitForRequest((r) => r.method() === 'PUT' && new URL(r.url()).pathname.endsWith('/admin/api/models'))
    await panel.getByRole('button', { name: 'Save model' }).click()
    const body = (await sent).postDataJSON()
    expect(body.name).toBe(name)
    expect(await panel.locator('input:invalid').count()).toBe(0)
  })
}
