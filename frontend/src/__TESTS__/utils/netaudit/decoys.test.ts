import { describe, expect, it } from 'vitest'
import { touchSummary } from '../../../utils/netaudit/decoys'
import type { DecoyTouch } from '../../../types/netaudit'

const touch = (source: string, port = 2222): DecoyTouch => ({ source, port, first: '2026-10-30T01:00:00Z', last: '2026-10-30T02:00:00Z', count: 1 })

describe('touchSummary', () => {
  it('is the plain "nothing" line when no device connected', () => expect(touchSummary([])).toBe('Nothing has connected to a decoy.'))
  it('counts devices, not connections: one device on two decoys is one device', () => {
    expect(touchSummary([touch('192.168.1.7', 2222), touch('192.168.1.7', 8023)])).toBe('1 device connected')
  })
  it('uses the plural for several devices', () => expect(touchSummary([touch('192.168.1.7'), touch('192.168.1.8')])).toBe('2 devices connected'))
})
