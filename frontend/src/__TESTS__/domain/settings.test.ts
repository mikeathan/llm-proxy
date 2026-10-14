import { describe, expect, it } from 'vitest'
import { getSettingsTabGroups, groupOfSection, isProviderTab } from '../../domain/settings'
import type { SettingsTab } from '../../types/admin'

// What useProviders.settingsTabs yields, plus a security entry the manifests omit.
const ALL: SettingsTab[] = ['appearance', 'local', 'runs', 'memory', 'storage', 'local-models', 'guardrails', 'gemini', 'openai', 'mcp', 'communication', 'search', 'processes']

describe('getSettingsTabGroups', () => {
  it('groups every section under six tabs, in order', () => {
    const groups = getSettingsTabGroups(ALL)
    expect(groups.map((g) => g.id)).toEqual(['general', 'models', 'safety', 'connectors', 'appearance', 'system'])
    expect(groups.map((g) => g.label)).toEqual(['General', 'Models', 'Safety', 'Connectors', 'Appearance', 'System'])
  })

  it('places each section where the operator would look for it', () => {
    const bySection = Object.fromEntries(getSettingsTabGroups(ALL).map((g) => [g.id, g.sections]))
    expect(bySection).toEqual({
      general: ['local', 'runs', 'memory', 'storage'],
      models: ['local-models', 'gemini', 'openai'],
      safety: ['security', 'guardrails'],
      connectors: ['communication', 'search', 'mcp'],
      appearance: ['appearance'],
      system: ['processes'],
    })
  })

  it('always offers Security, even when the source tabs omit it', () => {
    expect(getSettingsTabGroups(ALL).find((g) => g.id === 'safety')!.sections).toContain('security')
  })

  it('follows discovered providers, so a new cloud provider appears under Models', () => {
    const groups = getSettingsTabGroups([...ALL, 'nvidia'])
    expect(groups.find((g) => g.id === 'models')!.sections).toEqual(['local-models', 'gemini', 'openai', 'nvidia'])
  })

  it('leaves out a group that has no sections', () => {
    const groups = getSettingsTabGroups(['local', 'appearance'])
    expect(groups.map((g) => g.id)).toEqual(['general', 'safety', 'appearance'])
  })

  it('never lists a section twice', () => {
    const all = getSettingsTabGroups(ALL).flatMap((g) => g.sections)
    expect(new Set(all).size).toBe(all.length)
  })
})

describe('groupOfSection', () => {
  const groups = getSettingsTabGroups(ALL)

  it('finds the group a section belongs to', () => {
    expect(groupOfSection(groups, 'openai')?.id).toBe('models')
    expect(groupOfSection(groups, 'guardrails')?.id).toBe('safety')
    expect(groupOfSection(groups, 'processes')?.id).toBe('system')
  })

  it('answers undefined for an unknown section', () => {
    expect(groupOfSection(groups, 'nope' as SettingsTab)).toBeUndefined()
  })
})

describe('isProviderTab', () => {
  it('is true only for cloud providers', () => {
    for (const section of ['runs', 'memory', 'storage'] as const) expect(isProviderTab(section)).toBe(false)
    expect(isProviderTab('openai')).toBe(true)
    expect(isProviderTab('local')).toBe(false)
    expect(isProviderTab('mcp')).toBe(false)
  })
})
