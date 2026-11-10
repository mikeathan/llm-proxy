import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { reactive, ref } from 'vue'
import FixGuidePanel from '../../../components/netaudit/FixGuidePanel.vue'
import { RECHECK_INJECTION } from '../../../composables/netaudit/useRecheck'
import type { RecheckResult, ScanFinding } from '../../../types/netaudit'

const finding = (over: Partial<ScanFinding> = {}): ScanFinding => ({
  key: 'k', device: '192.0.2.10', port: 445, proto: 'tcp', check: 'smb_v1', severity: 'high', new: false, changed: false, accepted: false,
  guide: { steps: ['Turn it off'], risk: 'r', undo: 'u' }, recheckable: true, ...over,
})
const recheckState = (results: Record<string, RecheckResult> = {}) => ({ busyKey: ref(''), results: reactive(results), errors: reactive<Record<string, string>>({}), run: vi.fn() })
const mountPanel = (f: ScanFinding, state = recheckState()) =>
  mount(FixGuidePanel, { props: { finding: f, advanced: false }, global: { provide: { [RECHECK_INJECTION as symbol]: state } } })

describe('FixGuidePanel: check again and history', () => {
  it('offers Check now for a finding the server says can be rechecked, and asks for that finding', async () => {
    const state = recheckState()
    const w = mountPanel(finding(), state)
    await w.get('[data-test="recheck"]').trigger('click')
    expect(state.run).toHaveBeenCalledWith('k')
  })

  it('offers no button for a finding that cannot be rechecked, or one already verified', () => {
    expect(mountPanel(finding({ recheckable: false })).find('[data-test="recheck"]').exists()).toBe(false)
    const verified = mountPanel(finding({ verified_fixed_at: '2026-11-10T12:00:00Z' }))
    expect(verified.find('[data-test="recheck"]').exists()).toBe(false)
    expect(verified.get('[data-test="verified-fixed"]').text()).toContain('the problem was gone')
  })

  it('offers no button where no Findings screen provides one', () => {
    const w = mount(FixGuidePanel, { props: { finding: finding(), advanced: false } })
    expect(w.find('[data-test="recheck"]').exists()).toBe(false)
  })

  it('says each outcome in plain words, and a check that was not made records nothing', () => {
    const answer = (outcome: RecheckResult['outcome'], reason = '') => mountPanel(finding(), recheckState({ k: { outcome, reason, checked_at: '' } })).get('[data-test="recheck-answer"]').text()
    expect(answer('fixed')).toContain('the problem is gone')
    expect(answer('still_there')).toContain('still there')
    const none = answer('could_not_check', 'The device did not answer, so it could not be checked.')
    expect(none).toContain('nothing is recorded')
    expect(none).toContain('did not answer')
  })

  it('says how long a finding has been there and that it came back', () => {
    const w = mountPanel(finding({ first_seen: '2026-11-01T03:00:00Z', came_back: 2 }))
    const text = w.get('[data-test="finding-history"]').text()
    expect(text).toContain('First seen')
    expect(text).toContain('come back 2 times')
    expect(mountPanel(finding({ first_seen: '2026-11-01T03:00:00Z', came_back: 1 })).get('[data-test="finding-history"]').text()).toContain('came back once')
  })
})
