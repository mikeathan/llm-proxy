import { describe, expect, it } from 'vitest'
import { homeLineRoute } from '../../../utils/newsroom/homeLines'
import type { BriefHomeLine } from '../../../types/newsroom'

const line = (id: string): BriefHomeLine => ({ id, count: 1, words: 'w' })

describe('homeLineRoute', () => {
  it('sends a radio finding to its own place on the Radio board, marked by its id', () => {
    const to = homeLineRoute(line('radio_pmf_off')) as { name: string; query?: Record<string, string> }
    expect(to.query).toMatchObject({ finding: 'radio_pmf_off' })
  })

  it('sends a radio fact that is not a finding (off, quiet, unreadable) to Radio itself', () => {
    for (const id of ['radio_off', 'radio_quiet', 'radio_unavailable']) expect((homeLineRoute(line(id)) as { query?: unknown }).query).toBeUndefined()
  })

  it('sends a device fact to the Devices board and the rest of the network to Network', () => {
    const devices = homeLineRoute(line('network_not_marked_mine')) as { params?: Record<string, string> }
    const other = homeLineRoute(line('network_unavailable')) as { params?: Record<string, string> }
    expect(JSON.stringify(devices)).not.toBe(JSON.stringify(other))
    expect(JSON.stringify(devices)).toContain('network-devices')
  })
})
