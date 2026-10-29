import { describe, it, expect } from 'vitest'
import { newWebhookSecret, needsWebhookSecret } from '../../utils/webhookSecret'
import type { ConnectorConfig } from '../../types/admin'

const inbound = (over: Partial<ConnectorConfig> = {}, settings: Record<string, string> = {}): ConnectorConfig => ({
  type: 'telegram', enabled: true, settings: { workspace_id: 'ws', ...settings }, ...over,
})

describe('newWebhookSecret', () => {
  it('is 64 hex characters (inside what Telegram accepts) and different each time', () => {
    const a = newWebhookSecret()
    expect(a).toMatch(/^[0-9a-f]{64}$/)
    expect(newWebhookSecret()).not.toBe(a)
  })
})

describe('needsWebhookSecret', () => {
  it('is true for an enabled connector that receives messages and has no secret', () => {
    expect(needsWebhookSecret(inbound())).toBe(true)
  })
  it.each([
    ['has a secret', inbound({}, { webhook_token: 'x' })],
    ['is switched off', inbound({ enabled: false })],
    ['only sends (no workspace)', inbound({ settings: { chat_id: '1' } })],
  ])('is false for a connector that %s', (_n, cfg) => {
    expect(needsWebhookSecret(cfg)).toBe(false)
  })
})
