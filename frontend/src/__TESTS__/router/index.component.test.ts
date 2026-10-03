import { describe, it, expect } from 'vitest'
import { createMemoryHistory, type RouteLocationNormalized } from 'vue-router'
import { createAppRouter, routes, scrollBehavior } from '../../router'
import { ROUTE_NAMES } from '../../types/routes'

const router = createAppRouter(createMemoryHistory())
const nameOf = (path: string) => router.resolve(path).name

describe('route table (D18)', () => {
  it.each([
    ['/overview', ROUTE_NAMES.overview],
    ['/workspaces', ROUTE_NAMES.workspaces],
    ['/workspaces/ws', ROUTE_NAMES.workspace],
    ['/workspaces/ws/files', ROUTE_NAMES.workspaceFiles],
    ['/workspaces/ws/files/a/b.md', ROUTE_NAMES.workspaceFiles],
    ['/workspaces/ws/assistant', ROUTE_NAMES.workspaceAssistant],
    ['/workspaces/ws/assistant/c1', ROUTE_NAMES.workspaceAssistant],
    ['/workspaces/ws/memory', ROUTE_NAMES.workspaceSection],
    ['/workspaces/ws/settings', ROUTE_NAMES.workspaceSection],
    ['/workspaces/ws/playbooks', ROUTE_NAMES.workspaceSection],
    ['/workspaces/ws/heartbeat', ROUTE_NAMES.workspaceSection],
    ['/automations', ROUTE_NAMES.automations],
    ['/automations/new', ROUTE_NAMES.automationNew],
    ['/automations/recordings', ROUTE_NAMES.automationRecordings],
    ['/automations/ws%2Fa', ROUTE_NAMES.automation],
    ['/automations/ws%2Fa/edit', ROUTE_NAMES.automationEdit],
    ['/models', ROUTE_NAMES.models],
    ['/activity', ROUTE_NAMES.activity],
    ['/settings', ROUTE_NAMES.settings],
    ['/settings/mcp', ROUTE_NAMES.settings],
    ['/design', ROUTE_NAMES.design],
  ])('%s → %s', (path, name) => {
    expect(nameOf(path)).toBe(name)
  })

  it('sends the old workspace Security section to Settings', async () => {
    const nav = createAppRouter(createMemoryHistory())
    await nav.push('/workspaces/ws/security')
    expect(nav.currentRoute.value.name).toBe(ROUTE_NAMES.workspaceSection)
    expect(nav.currentRoute.value.fullPath).toBe('/workspaces/ws/settings')
  })

  it('ranks the static recordings segment above :id', () => {
    expect(router.resolve('/automations/recordings').params.id).toBeUndefined()
  })

  it.each(['/nope', '/workspaces/ws/bogus', '/automations/ws%2Fa/bogus'])('renders %s as not-found', (path) => {
    expect(nameOf(path)).toBe(ROUTE_NAMES.notFound)
  })

  it('redirects / to Overview and a bare workspace to its files', async () => {
    const r = createAppRouter(createMemoryHistory())
    await r.push('/')
    expect(r.currentRoute.value.name).toBe(ROUTE_NAMES.overview)
    await r.push('/workspaces/ws')
    expect(r.currentRoute.value.fullPath).toBe('/workspaces/ws/files')
  })

  it('lazy-loads every routed component', () => {
    for (const record of routes) {
      if (record.component) expect(typeof record.component).toBe('function')
    }
  })

  it('assigns every routed page to a destination except the bare and fallback pages', () => {
    for (const record of routes) {
      if (!record.component || record.meta?.bare || record.name === ROUTE_NAMES.notFound) continue
      expect(record.meta?.destination, String(record.name)).toBeDefined()
    }
  })

  it('restores the saved position on back/forward and scrolls to top otherwise', () => {
    const route = {} as RouteLocationNormalized
    expect(scrollBehavior(route, route, { left: 0, top: 480 })).toEqual({ left: 0, top: 480 })
    expect(scrollBehavior(route, route, null)).toEqual({ top: 0 })
  })
})
