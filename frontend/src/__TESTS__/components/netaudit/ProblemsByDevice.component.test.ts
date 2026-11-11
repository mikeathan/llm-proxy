import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ProblemsByDevice from '../../../components/netaudit/ProblemsByDevice.vue'
import { groupProblemsByDevice } from '../../../utils/netaudit/report'
import type { ScanFinding } from '../../../types/netaudit'

const problem = (key: string, device: string, severity: string, over: Partial<ScanFinding> = {}): ScanFinding => ({
  key, device, port: 0, check: `c-${key}`, check_title: `Title ${key}`, severity, evidence: `Evidence ${key}`, new: false, changed: false, accepted: false, ...over,
})

describe('ProblemsByDevice', () => {
  const rows = groupProblemsByDevice([problem('1', '192.0.2.1', 'low'), problem('2', '192.0.2.1', 'high'), problem('3', '192.0.2.2', 'low')])

  it('shows one card per device, the worst first, each problem with its evidence', () => {
    const w = mount(ProblemsByDevice, { props: { devices: rows, advanced: false } })
    const cards = w.findAll('[data-test="device-card"]')
    expect(cards.map((c) => c.attributes('data-device'))).toEqual(['192.0.2.1', '192.0.2.2'])
    const first = cards[0]!.findAll('[data-test="device-problem"]')
    expect(first.map((p) => p.get('[data-test="device-problem-title"]').text())).toEqual(['Title 2', 'Title 1'])
    expect(first[0]!.text()).toContain('Evidence 2')
  })

  it('names a device by what the person calls it, with its address, and says "This computer" for the host', () => {
    const host = groupProblemsByDevice([problem('1', 'this-computer', 'low')])
    const w = mount(ProblemsByDevice, { props: { devices: [...rows, ...host], advanced: false, names: { '192.0.2.1': 'Pixel 8' } } })
    const titles = w.findAll('[data-test="device-name"]').map((n) => n.text())
    expect(titles).toEqual(['Pixel 8 · 192.0.2.1', '192.0.2.2', 'This computer'])
  })

  it('accepts one problem, or every problem of a device with several', async () => {
    const w = mount(ProblemsByDevice, { props: { devices: rows, advanced: false } })
    await w.findAll('[data-test="device-problem"] [data-test="accept"]')[0]!.trigger('click')
    expect((w.emitted('accept')![0]![0] as ScanFinding).key).toBe('2')
    const cards = w.findAll('[data-test="device-card"]')
    await cards[0]!.get('[data-test="accept-all"]').trigger('click')
    expect((w.emitted('accept-many')![0]![0] as ScanFinding[]).map((f) => f.key)).toEqual(['2', '1'])
    expect(cards[1]!.find('[data-test="accept-all"]').exists()).toBe(false)
  })

  it('does not offer to accept a problem that only a deep check has recorded', () => {
    const deep = groupProblemsByDevice([problem('d', '192.0.2.9', 'low', { deep_at: '2026-11-01T00:00:00Z' })])
    const w = mount(ProblemsByDevice, { props: { devices: deep, advanced: false } })
    expect(w.find('[data-test="accept"]').exists()).toBe(false)
  })
})

describe('ProblemsByDevice rows can be linked to', () => {
  it('carries each problem\'s key on its row, so a link from another page can find it', () => {
    const w = mount(ProblemsByDevice, { props: { devices: groupProblemsByDevice([problem('k1', '192.0.2.1', 'low'), problem('k2', '192.0.2.1', 'high')]), advanced: false } })
    expect(w.findAll('[data-test="device-problem"]').map((r) => r.attributes('data-key')).sort()).toEqual(['k1', 'k2'])
  })
})
