import { describe, expect, it } from 'vitest'
import { useProviders } from '../../../composables/models/useProviders'
import { SETTINGS_GROUP_SECTIONS } from '../../../constants/settings'

describe('useProviders settingsTabs', () => {
  it('lists every fixed Settings section, or its page is "not found"', () => {
    const tabs = new Set(useProviders().settingsTabs.value)
    // Security is always offered by the grouping itself, so the manifests need not list it.
    const missing = Object.values(SETTINGS_GROUP_SECTIONS).flat().filter((s) => s !== 'security' && !tabs.has(s))
    expect(missing).toEqual([])
  })
})
