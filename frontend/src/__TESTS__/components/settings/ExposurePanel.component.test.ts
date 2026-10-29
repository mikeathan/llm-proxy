import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { AuthExposure } from '../../../types/auth'

const service = vi.hoisted(() => ({ exposure: vi.fn() }))
vi.mock('../../../services/auth/authService', () => ({ AuthService: service }))

import ExposurePanel from '../../../components/settings/ExposurePanel.vue'

const exposure = (over: Partial<AuthExposure> = {}): AuthExposure => ({
  listening: { address: '127.0.0.1:4001', network: false }, encrypted: false, inference_key_required: false,
  webhooks: { inbound: 0, with_secret: 0 }, sandbox: { enabled: true, filesystem: 'landlock' }, attention: [], ...over,
})

async function show(e: AuthExposure) {
  service.exposure.mockResolvedValue(e)
  const w = mount(ExposurePanel)
  await flushPromises()
  return w
}

beforeEach(() => service.exposure.mockReset())

describe('ExposurePanel', () => {
  it('says where the server listens and what protects it, in words', async () => {
    const w = await show(exposure())
    const facts = w.get('[data-test="exposure-facts"]').text()
    expect(facts).toContain('127.0.0.1:4001')
    expect(facts).toContain('this machine only')
    expect(facts).toContain('Not encrypted')
    expect(facts).toContain('Not required: /v1 is open')
    expect(facts).toContain('No connector receives messages')
    expect(facts).toContain('On · files: landlock')
    expect(w.find('[data-test="exposure-ok"]').exists()).toBe(true)
  })

  it('counts webhook secrets of the connectors that receive messages', async () => {
    const w = await show(exposure({ webhooks: { inbound: 2, with_secret: 1 } }))
    expect(w.get('[data-test="exposure-facts"]').text()).toContain('1 of 2 connectors that receive messages')
  })

  it('shows each item that needs attention with what to do about it, and not the all-clear', async () => {
    const w = await show(exposure({
      listening: { address: '0.0.0.0:4001', network: true },
      attention: [{ id: 'unencrypted', text: 'This connection is not encrypted.', fix: 'Put a TLS front end in front.' }],
    }))
    const attention = w.get('[data-test="exposure-attention"]').text()
    expect(attention).toContain('This connection is not encrypted.')
    expect(attention).toContain('Put a TLS front end in front.')
    expect(w.get('[data-test="exposure-facts"]').text()).toContain('other machines can connect')
    expect(w.find('[data-test="exposure-ok"]').exists()).toBe(false)
  })

  it('says when the sandbox is off', async () => {
    const w = await show(exposure({ sandbox: { enabled: false, filesystem: 'none' } }))
    expect(w.get('[data-test="exposure-facts"]').text()).toContain('Off')
  })

  it('reports a failure instead of showing nothing', async () => {
    service.exposure.mockRejectedValueOnce(new Error('boom'))
    const w = mount(ExposurePanel)
    await flushPromises()
    expect(w.get('[data-test="exposure-error"]').text()).toContain('boom')
  })
})
