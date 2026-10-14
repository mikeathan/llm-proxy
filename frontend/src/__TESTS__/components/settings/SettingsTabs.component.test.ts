import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import SettingsTabs from '../../../components/settings/SettingsTabs.vue'
import { getSettingsTabGroups } from '../../../domain/settings'
import { ROUTE_NAMES } from '../../../types/routes'
import type { SettingsTab } from '../../../types/admin'

const TABS: SettingsTab[] = ['appearance', 'local', 'local-models', 'limits', 'guardrails', 'gemini', 'openai', 'mcp', 'communication', 'search', 'processes']
const LABELS: Record<string, string> = { local: 'Local Engine', 'local-models': 'Local Models', gemini: 'Google Gemini', openai: 'OpenAI', guardrails: 'Agent Guardrails', security: 'Security', limits: 'Limits & reset', mcp: 'MCP Servers', communication: 'Communication', search: 'Search', processes: 'Processes', appearance: 'Appearance' }

function mountTabs(active: SettingsTab) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/settings/:section?', name: ROUTE_NAMES.settings, component: { render: () => null } }],
  })
  return mount(SettingsTabs, {
    props: { groups: getSettingsTabGroups(TABS), active, labelOf: (t: SettingsTab) => LABELS[t] ?? t },
    global: { plugins: [router] },
  })
}

describe('SettingsTabs', () => {
  it('links the six groups, each to its first section, and marks the current group', () => {
    const w = mountTabs('openai')
    const tabs = w.get('nav[aria-label="Settings"]').findAll('a')
    expect(tabs.map((a) => a.text())).toEqual(['General', 'Models', 'Safety', 'Connectors', 'Appearance', 'System'])
    expect(tabs.map((a) => a.attributes('href'))).toEqual([
      '/settings/local',
      '/settings/local-models',
      '/settings/security',
      '/settings/communication',
      '/settings/appearance',
      '/settings/processes',
    ])
    expect(tabs.filter((a) => a.attributes('aria-current') === 'page').map((a) => a.text())).toEqual(['Models'])
  })

  it('shows the sections of a group that has several, and marks the current one', () => {
    const w = mountTabs('openai')
    const chips = w.get('nav[aria-label="Models sections"]').findAll('a')
    expect(chips.map((a) => a.text())).toEqual(['Local Models', 'Google Gemini', 'OpenAI'])
    expect(chips.filter((a) => a.attributes('aria-current') === 'page').map((a) => a.text())).toEqual(['OpenAI'])
  })

  it('shows no sections for a group that has only one', () => {
    const w = mountTabs('local')
    expect(w.find('nav[aria-label$="sections"]').exists()).toBe(false)
  })

  it('follows a section to its group (Guardrails sits under Safety)', () => {
    const w = mountTabs('guardrails')
    expect(w.get('nav[aria-label="Settings"] a[aria-current="page"]').text()).toBe('Safety')
    expect(w.findAll('nav[aria-label="Safety sections"] a').map((a) => a.text())).toEqual(['Security', 'Limits & reset', 'Agent Guardrails'])
  })
})
