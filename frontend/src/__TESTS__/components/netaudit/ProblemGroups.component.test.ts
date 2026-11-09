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

  const dns = (ip: string, over: Partial<ScanFinding> = {}): ScanFinding => ({ ...problem('dns'), key: `k-${ip}`, device: ip, port: 0, evidence: 'Silent for a day.', ...over })

  it('says a line every place shares once, and keeps each place to the device', () => {
    const w = mount(ProblemGroups, { props: { groups: groupProblems([dns('192.0.2.1'), dns('192.0.2.2')]), advanced: false } })
    expect(w.get('[data-test="shared-evidence"]').text()).toBe('Silent for a day.')
    const rows = w.findAll('[data-test="problem"]')
    expect(rows).toHaveLength(2)
    expect(rows[0]!.text()).not.toContain('Silent for a day.')
  })

  it('shows each place by the name the person gave the device, with its address', () => {
    const w = mount(ProblemGroups, { props: { groups: groupProblems([dns('192.0.2.1'), dns('192.0.2.2')]), advanced: false, names: { '192.0.2.1': 'Pixel 8' } } })
    const places = w.findAll('[data-test="problem-place"]').map((p) => p.text())
    expect(places).toEqual(['Pixel 8 · 192.0.2.1', '192.0.2.2'])
  })

  it('offers one button to accept every place of a kind, only when there are several', async () => {
    const many = mount(ProblemGroups, { props: { groups: groupProblems([dns('192.0.2.1'), dns('192.0.2.2')]), advanced: false } })
    await many.get('[data-test="accept-all"]').trigger('click')
    expect((many.emitted('accept-many')![0]![0] as ScanFinding[]).map((f) => f.key)).toEqual(['k-192.0.2.1', 'k-192.0.2.2'])
    const one = mount(ProblemGroups, { props: { groups: groupProblems([dns('192.0.2.1')]), advanced: false } })
    expect(one.find('[data-test="accept-all"]').exists()).toBe(false)
  })

  it('starts every card folded in notes mode, however short the list', () => {
    const w = mount(ProblemGroups, { props: { groups: groupProblems([problem('a')]), advanced: false, notes: true } })
    expect(isOpen(w)).toEqual(['false'])
  })
})
