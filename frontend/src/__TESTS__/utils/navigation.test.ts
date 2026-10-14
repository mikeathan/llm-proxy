import { describe, expect, it } from 'vitest'
import { badgeTarget, buildNavigation, homeLocation } from '../../utils/navigation'
import { ROUTE_NAMES } from '../../types/routes'
import type { RunEndItem } from '../../types/notifications'

const base = { lastWorkspace: 'demo' as string | null, routeName: ROUTE_NAMES.overview as string | null, destination: 'overview' as const, badges: {} }
const item = (items: ReturnType<typeof buildNavigation>, id: string) => items.find((i) => i.id === id)!

describe('buildNavigation', () => {
  it('lists Assistant first, then the rest, with Workspace singular', () => {
    expect(buildNavigation(base).map((i) => i.label)).toEqual(['Assistant', 'Overview', 'Workspace', 'Automations', 'Reminders', 'Models', 'Activity', 'Settings'])
  })

  it('opens the assistant and the files of the workspace last used', () => {
    const items = buildNavigation(base)
    expect(item(items, 'assistant').to).toEqual({ name: ROUTE_NAMES.workspaceAssistant, params: { ws: 'demo' } })
    expect(item(items, 'workspace').to).toEqual({ name: ROUTE_NAMES.workspaceFiles, params: { ws: 'demo', path: [] } })
  })

  it('sends Assistant and Workspace to the workspace list while none is known', () => {
    const items = buildNavigation({ ...base, lastWorkspace: null })
    expect(item(items, 'assistant').to).toEqual({ name: ROUTE_NAMES.workspaces })
    expect(item(items, 'workspace').to).toEqual({ name: ROUTE_NAMES.workspaces })
  })

  it('marks one item current: the assistant route is Assistant, other workspace routes are Workspace', () => {
    const current = (routeName: string, destination: Parameters<typeof buildNavigation>[0]['destination']) =>
      buildNavigation({ ...base, routeName, destination }).filter((i) => i.current).map((i) => i.id)
    expect(current(ROUTE_NAMES.workspaceAssistant, 'workspaces')).toEqual(['assistant'])
    expect(current(ROUTE_NAMES.workspaceFiles, 'workspaces')).toEqual(['workspace'])
    expect(current(ROUTE_NAMES.workspaceSection, 'workspaces')).toEqual(['workspace'])
    expect(current(ROUTE_NAMES.workspaces, 'workspaces')).toEqual(['workspace'])
    expect(current(ROUTE_NAMES.automation, 'automations')).toEqual(['automations'])
    expect(current(ROUTE_NAMES.settings, 'settings')).toEqual(['settings'])
  })

  it('marks nothing current where no destination owns the route', () => {
    expect(buildNavigation({ ...base, routeName: ROUTE_NAMES.notFound, destination: undefined }).some((i) => i.current)).toBe(false)
  })

  it('carries each badge count, zero when there is none', () => {
    const items = buildNavigation({ ...base, badges: { assistant: 2, automations: 1 } })
    expect(items.map((i) => i.badge)).toEqual([2, 0, 0, 1, 0, 0, 0, 0])
  })

  it('puts four destinations in the phone tab bar and leaves the rest to More', () => {
    const items = buildNavigation(base)
    expect(items.filter((i) => i.tab).map((i) => i.id)).toEqual(['assistant', 'workspace', 'automations', 'reminders'])
    expect(items.filter((i) => !i.tab).map((i) => i.id)).toEqual(['overview', 'models', 'activity', 'settings'])
  })
})

describe('badgeTarget', () => {
  const run = (kind: RunEndItem['kind'], destination: RunEndItem['destination']) => ({ kind, destination }) as Pick<RunEndItem, 'kind' | 'destination'>

  it('badges the assistant for an assistant run and the destination for anything else', () => {
    expect(badgeTarget(run('assistant', 'workspaces'))).toBe('assistant')
    expect(badgeTarget(run('automation', 'automations'))).toBe('automations')
    expect(badgeTarget(run('automation', 'workspaces'))).toBe('workspace')
  })
})

describe('homeLocation', () => {
  it('is the assistant of the workspace last used, else the Overview', () => {
    expect(homeLocation('demo')).toEqual({ name: ROUTE_NAMES.workspaceAssistant, params: { ws: 'demo' } })
    expect(homeLocation(null)).toEqual({ name: ROUTE_NAMES.overview })
  })
})
