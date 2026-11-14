import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import RadioWindows from '../../../components/radio/RadioWindows.vue'
import type { RadioWindow } from '../../../types/radio'

const win = (channel: number, frames: number, over: Partial<RadioWindow> = {}): RadioWindow => ({
  channel, frames, recorded: frames, own_frames: 0, unread: 0, stop: 'window', kernel_seen: frames, kernel_dropped: 0, elapsed_ms: 20000,
  on_channel: frames, off_channel: 0, channel_matched: true, at: '2026-11-14T15:00:00Z', ...over,
})

describe('RadioWindows', () => {
  it('shows one row per window, newest first, with the band and a gap between each part', () => {
    const w = mount(RadioWindows, { props: { windows: [win(1, 535), win(60, 4000, { stop: 'frame_cap', at: '2026-11-14T15:01:00Z' })] } })
    const rows = w.findAll('[data-test="radio-window"]')
    expect(rows).toHaveLength(2)
    expect(rows[0]!.attributes('data-channel')).toBe('60')
    expect(rows[0]!.get('[data-test="radio-window-channel"]').text()).toBe('Channel 60')
    expect(rows[0]!.get('[data-test="radio-window-band"]').text()).toBe('5 GHz')
    expect(rows[0]!.get('[data-test="radio-window-frames"]').text()).toBe('4,000 frames')
    expect(rows[1]!.get('[data-test="radio-window-band"]').text()).toBe('2.4 GHz')
    expect(rows[1]!.get('[data-test="radio-window-frames"]').text()).toBe('535 frames')
  })

  it('flags a window cut at the frame limit as a plain badge with its meaning', () => {
    const w = mount(RadioWindows, { props: { windows: [win(60, 4000, { stop: 'frame_cap' }), win(1, 10)] } })
    const cut = w.findAll('[data-test="radio-window-cut"]')
    expect(cut).toHaveLength(1)
    expect(cut[0]!.text()).toContain('4,000-frame limit')
    expect(w.get('[data-test="radio-window-badge"]').text()).toBe('Busy channel')
  })

  it('counts the frames that came from the owner’s devices', () => {
    const w = mount(RadioWindows, { props: { windows: [win(1, 40, { own_frames: 7 }), win(6, 10)] } })
    const own = w.findAll('[data-test="radio-window-own"]')
    expect(own).toHaveLength(1)
    expect(own[0]!.text()).toBe('7 from your devices')
  })

  it('says what a silent window means instead of showing nothing', () => {
    const w = mount(RadioWindows, { props: { windows: [win(36, 0, { stop: 'window', channel_matched: false })] } })
    expect(w.get('[data-test="radio-window"]').text()).toContain('0 frames')
    expect(w.get('[data-test="radio-window-note"]').text()).toBe('window')
  })

  it('shows the empty state when no window has finished', () => {
    const w = mount(RadioWindows, { props: { windows: [] } })
    expect(w.get('[data-test="radio-windows-empty"]').text()).toContain('No window has finished yet')
  })

  it('is capped to the latest rows', () => {
    const w = mount(RadioWindows, { props: { windows: Array.from({ length: 30 }, (_, i) => win(1, i + 1, { at: `2026-11-14T15:${String(i).padStart(2, '0')}:00Z` })) } })
    expect(w.findAll('[data-test="radio-window"]')).toHaveLength(12)
  })
})
