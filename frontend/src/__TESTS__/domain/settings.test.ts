import { describe, expect, it } from 'vitest'
import { getSettingsTabGroups, groupOfSection, isSettingsSection, SETTINGS_SECTIONS } from '../../domain/settings'
import type { SettingsTab } from '../../types/admin'

describe('getSettingsTabGroups', () => {
  it('groups every section under four tabs, in order: models are on their own page', () => {
    const groups = getSettingsTabGroups()
    expect(groups.map((g) => g.id)).toEqual(['general', 'safety', 'connectors', 'appearance'])
    expect(groups.map((g) => g.label)).toEqual(['General', 'Safety', 'Connectors', 'Appearance'])
  })

  it('places each section where the operator would look for it', () => {
    const bySection = Object.fromEntries(getSettingsTabGroups().map((g) => [g.id, g.sections]))
    expect(bySection).toEqual({
      general: ['agents', 'memory', 'storage'],
      safety: ['security', 'sign-in', 'limits', 'guardrails'],
      connectors: ['communication', 'search'],
      appearance: ['appearance'],
    })
  })

  it('never lists a section twice', () => {
    const all = getSettingsTabGroups().flatMap((g) => g.sections)
    expect(new Set(all).size).toBe(all.length)
  })
})

describe('isSettingsSection', () => {
  it('knows every section and nothing that moved to the Models page', () => {
    for (const section of SETTINGS_SECTIONS) expect(isSettingsSection(section)).toBe(true)
    for (const moved of ['local', 'runs', 'local-models', 'processes', 'openai', 'nvidia', 'network-scan']) expect(isSettingsSection(moved)).toBe(false)
  })
})

describe('groupOfSection', () => {
  const groups = getSettingsTabGroups()

  it('finds the group a section belongs to', () => {
    expect(groupOfSection(groups, 'guardrails')?.id).toBe('safety')
    expect(groupOfSection(groups, 'search')?.id).toBe('connectors')
  })

  it('answers undefined for an unknown section', () => {
    expect(groupOfSection(groups, 'nope' as SettingsTab)).toBeUndefined()
  })
})
