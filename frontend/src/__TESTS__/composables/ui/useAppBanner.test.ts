import { describe, it, expect, beforeEach } from 'vitest'
import { useAppBanner } from '../../../composables/ui/useAppBanner'

describe('useAppBanner', () => {
  beforeEach(() => {
    const { clear } = useAppBanner()
    clear()
  })

  it('defaults to no active banner', () => {
    const { active } = useAppBanner()
    expect(active.value).toBeNull()
  })

  it('shows a banner with severity and message', () => {
    const { active, show } = useAppBanner()
    show({ severity: 'error', message: 'boom' })
    expect(active.value?.severity).toBe('error')
    expect(active.value?.message).toBe('boom')
    expect(active.value?.persistent).toBeFalsy()
  })

  it('persistent flag is presentation-only (drives dismiss in the component)', () => {
    const { active, show } = useAppBanner()
    show({ severity: 'critical', message: 'no model', persistent: true })
    expect(active.value?.persistent).toBe(true)
  })

  it('clear removes the active banner', () => {
    const { active, show, clear } = useAppBanner()
    show({ severity: 'notice', message: 'using fallback' })
    clear()
    expect(active.value).toBeNull()
  })
})
