import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ProblemGroups from '../../../components/netaudit/ProblemGroups.vue'
import { groupProblems } from '../../../utils/netaudit/report'
import type { ScanFinding } from '../../../types/netaudit'

const problem = (check: string, severity = 'low'): ScanFinding => ({
  key: check, device: '192.0.2.10', port: 22, proto: 'tcp', service: 'ssh', check, check_title: `Title ${check}`, severity, evidence: 'e', new: false, changed: false, accepted: false,
})
const isOpen = (w: ReturnType<typeof mount>) => w.findAll('[data-test="card-toggle"]').map((b) => b.attributes('aria-expanded'))

// A long list starts folded to its titles so a phone shows every kind of problem on one screen (vertex, 2026-11-05: nine
// open cards made a 5,000 px page); a short one stays open so a single problem is never behind a tap.
describe('ProblemGroups', () => {
  it('opens a short list and folds a long one, and the reader can open any card', async () => {
    const short = mount(ProblemGroups, { props: { groups: groupProblems([problem('a'), problem('b')]), advanced: false } })
    expect(isOpen(short)).toEqual(['true', 'true'])

    const long = mount(ProblemGroups, { props: { groups: groupProblems([problem('a', 'high'), problem('b'), problem('c')]), advanced: false } })
    expect(isOpen(long)).toEqual(['false', 'false', 'false'])
    await long.findAll('[data-test="card-toggle"]')[1]!.trigger('click')
    expect(isOpen(long)).toEqual(['false', 'true', 'false'])
  })

  it('says the fix once: the guide when there is one, the line when there is not', () => {
    const guided = { ...problem('a'), fix: 'Do it.', guide: { steps: ['Step one'], risk: 'r', undo: 'u' } }
    const w = mount(ProblemGroups, { props: { groups: groupProblems([guided, { ...problem('b'), fix: 'Plain fix.' }]), advanced: false } })
    const cards = w.findAll('article')
    expect(cards[0]!.find('[data-test="fix"]').exists()).toBe(false)
    expect(cards[0]!.find('[data-test="fix-guide"]').exists()).toBe(true)
    expect(cards[1]!.get('[data-test="fix"]').text()).toContain('Plain fix.')
  })
})
