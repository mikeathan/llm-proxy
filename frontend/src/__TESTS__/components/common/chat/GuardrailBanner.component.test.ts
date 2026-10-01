import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const post = vi.fn()
vi.mock('../../../../services/httpClient', () => ({ post: (...args: unknown[]) => post(...args) }))

import GuardrailBanner from '../../../../components/common/chat/GuardrailBanner.vue'

const DECISION = { decision_id: 'd1', tool: 'terminal', reason: 'rm is blocked', category: 'terminal', args: 'rm -rf x' }

function mountBanner(submit = vi.fn().mockResolvedValue(undefined)) {
  const w = mount(GuardrailBanner, { props: { decision: DECISION as never, submit }, global: { stubs: { Icon: true } } })
  const button = (name: RegExp) => w.findAll('button').find((b) => name.test(b.text()))!
  return { w, submit, button }
}

// The parent's submitDecision is the one place the decision is sent: the
// banner used to POST it too, so every decision went out twice (and a second
// "allow & remember" could persist the override again).
describe('GuardrailBanner', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shows what was blocked and why, as an alert', () => {
    const { w } = mountBanner()
    expect(w.attributes('role')).toBe('alert')
    expect(w.text()).toContain('terminal')
    expect(w.text()).toContain('rm is blocked')
  })

  it.each([
    [/allow and remember/i, [true, true]],
    [/allow once/i, [true, false]],
    [/^deny$/i, [false, false]],
  ] as const)('%s sends exactly one decision, through the parent', async (name, args) => {
    const { button, submit } = mountBanner()
    await button(name).trigger('click')
    await flushPromises()
    expect(submit).toHaveBeenCalledTimes(1)
    expect(submit).toHaveBeenCalledWith(...args)
    expect(post).not.toHaveBeenCalled()
  })

  it('locks its buttons while a decision is being sent, and frees them if it fails', async () => {
    let fail!: (e: Error) => void
    const submit = vi.fn().mockImplementation(() => new Promise<void>((_, reject) => { fail = reject }))
    const { w, button } = mountBanner(submit)
    await button(/^deny$/i).trigger('click')
    expect(w.findAll('button').every((b) => b.attributes('disabled') !== undefined)).toBe(true)
    fail(new Error('network'))
    await flushPromises()
    expect(button(/^deny$/i).attributes('disabled')).toBeUndefined()
  })
})
