import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import SettingsTabs from '../../../components/settings/SettingsTabs.vue'
import { getSettingsTabGroups } from '../../../domain/settings'
import { SETTINGS_SECTION_LABELS } from '../../../constants/settings'
import { ROUTE_NAMES } from '../../../types/routes'
import type { SettingsTab } from '../../../types/admin'

function mountTabs(active: SettingsTab) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/settings/:section?', name: ROUTE_NAMES.settings, component: { render: () => null } }],
  })
  return mount(SettingsTabs, {
    props: { groups: getSettingsTabGroups(), active, labelOf: (t: SettingsTab) => SETTINGS_SECTION_LABELS[t] },
    global: { plugins: [router] },
  })
}

describe('SettingsTabs', () => {
  it('links the four groups, each to its first section, and marks the current group', () => {
    const w = mountTabs('search')
    const tabs = w.get('nav[aria-label="Settings"]').findAll('a')
    expect(tabs.map((a) => a.text())).toEqual(['General', 'Safety', 'Connectors', 'Appearance'])
    expect(tabs.map((a) => a.attributes('href'))).toEqual(['/settings/agents', '/settings/security', '/settings/communication', '/settings/appearance'])
    expect(tabs.filter((a) => a.attributes('aria-current') === 'page').map((a) => a.text())).toEqual(['Connectors'])
  })

  it('shows the sections of a group that has several, and marks the current one', () => {
    const w = mountTabs('memory')
    const chips = w.get('nav[aria-label="General sections"]').findAll('a')
    expect(chips.map((a) => a.text())).toEqual(['Agents', 'Memory & reminders', 'Storage & logging'])
    expect(chips.filter((a) => a.attributes('aria-current') === 'page').map((a) => a.text())).toEqual(['Memory & reminders'])
  })

  it('shows no sections for a group that has only one', () => {
    const w = mountTabs('appearance')
    expect(w.find('nav[aria-label$="sections"]').exists()).toBe(false)
  })

  it('follows a section to its group (Guardrails sits under Safety)', () => {
    const w = mountTabs('guardrails')
    expect(w.get('nav[aria-label="Settings"] a[aria-current="page"]').text()).toBe('Safety')
    expect(w.findAll('nav[aria-label="Safety sections"] a').map((a) => a.text())).toEqual(['Sandboxing', 'Sign-in & access', 'Limits & reset', 'Agent Guardrails'])
  })
})
