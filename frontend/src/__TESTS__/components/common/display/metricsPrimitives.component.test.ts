import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import StatCard from '../../../../components/common/display/StatCard.vue'
import Sparkline from '../../../../components/common/display/Sparkline.vue'
import Meter from '../../../../components/common/display/Meter.vue'
import SlotBar from '../../../../components/common/display/SlotBar.vue'

describe('StatCard', () => {
  it('shows a label, a tabular value with unit, a caption and extra content', () => {
    const w = mount(StatCard, { props: { label: 'Throughput', value: '48.6', unit: 'tok/s', caption: 'since page load' }, slots: { default: '<i data-test="extra" />' } })
    expect(w.text()).toContain('Throughput')
    expect(w.get('[data-test="stat-value"]').text()).toBe('48.6')
    expect(w.text()).toContain('tok/s')
    expect(w.text()).toContain('since page load')
    expect(w.find('[data-test="extra"]').exists()).toBe(true)
  })
})

describe('Sparkline', () => {
  it('is an image with a text summary, one bar per value, the latest emphasised', () => {
    const w = mount(Sparkline, { props: { values: [2, 4, 8], label: 'Tokens per second', unit: 'tok/s' } })
    const img = w.get('[role="img"]')
    expect(img.attributes('aria-label')).toBe('Tokens per second: 3 samples, latest 8 tok/s, peak 8 tok/s')
    const bars = w.findAll('[data-test="spark-bar"]')
    expect(bars).toHaveLength(3)
    expect(bars[0]!.attributes('style')).toContain('height: 25%')
    expect(bars[2]!.attributes('data-latest')).toBe('true')
  })

  it('says when there is no history yet', () => {
    const w = mount(Sparkline, { props: { values: [], label: 'CPU load', unit: '%' } })
    expect(w.get('[role="img"]').attributes('aria-label')).toBe('CPU load: no samples yet')
  })
})

describe('Meter', () => {
  it('is a meter with its value in text and attributes', () => {
    const w = mount(Meter, { props: { label: 'Memory used', value: 63.4 } })
    const meter = w.get('[role="meter"]')
    expect(meter.attributes('aria-label')).toBe('Memory used')
    expect(meter.attributes('aria-valuenow')).toBe('63.4')
    expect(meter.attributes('aria-valuemin')).toBe('0')
    expect(meter.attributes('aria-valuemax')).toBe('100')
    expect(w.text()).toContain('63%')
    expect(w.get('[data-test="meter-fill"]').attributes('style')).toContain('width: 63.4%')
  })

  it('clamps the fill and accepts custom text', () => {
    const w = mount(Meter, { props: { label: 'VRAM used', value: 150, text: '9.0 / 8.0 GB' } })
    expect(w.get('[data-test="meter-fill"]').attributes('style')).toContain('width: 100%')
    expect(w.text()).toContain('9.0 / 8.0 GB')
  })
})

describe('SlotBar', () => {
  it('draws one block per slot, the used ones filled, as a labelled meter', () => {
    const w = mount(SlotBar, { props: { label: 'Cloud lane', used: 2, limit: 3 } })
    const meter = w.get('[role="meter"]')
    expect(meter.attributes('aria-label')).toBe('Cloud lane')
    expect(meter.attributes('aria-valuenow')).toBe('2')
    expect(meter.attributes('aria-valuemax')).toBe('3')
    expect(meter.attributes('aria-valuetext')).toBe('2 of 3 slots in use')
    expect(w.findAll('[data-test="slot"]')).toHaveLength(3)
    expect(w.findAll('[data-test="slot"][data-used="true"]')).toHaveLength(2)
    expect(w.text()).toContain('2/3')
  })

  it('never draws more used slots than exist', () => {
    const w = mount(SlotBar, { props: { label: 'Local lane', used: 4, limit: 1 } })
    expect(w.findAll('[data-test="slot"][data-used="true"]')).toHaveLength(1)
    expect(w.text()).toContain('4/1')
  })
})
