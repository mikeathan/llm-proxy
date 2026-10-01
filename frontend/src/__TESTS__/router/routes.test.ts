import { describe, it, expect } from 'vitest'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../router'
import {
  toActivity,
  toAutomation,
  toAutomationEdit,
  toAutomationNew,
  toAutomationRecordings,
  toAutomations,
  toModels,
  toOverview,
  toSettings,
  toWorkspace,
  toWorkspaceAssistant,
  toWorkspaceFile,
  toWorkspaceSection,
  toWorkspaces,
  workspaceLocation,
} from '../../router/routes'

const router = createAppRouter(createMemoryHistory())
const pathOf = (to: Parameters<typeof router.resolve>[0]) => router.resolve(to).fullPath

describe('typed route builders', () => {
  it.each([
    ['overview', toOverview(), '/overview'],
    ['workspaces', toWorkspaces(), '/workspaces'],
    ['workspace', toWorkspace('ws'), '/workspaces/ws'],
    ['files root', toWorkspaceFile('ws'), '/workspaces/ws/files'],
    ['nested file', toWorkspaceFile('ws', 'docs/plans/a.md'), '/workspaces/ws/files/docs/plans/a.md'],
    ['assistant', toWorkspaceAssistant('ws'), '/workspaces/ws/assistant'],
    ['conversation', toWorkspaceAssistant('ws', 'c-1'), '/workspaces/ws/assistant/c-1'],
    ['section', toWorkspaceSection('ws', 'memory'), '/workspaces/ws/memory'],
    ['automations', toAutomations(), '/automations'],
    ['new automation', toAutomationNew(), '/automations/new'],
    ['recordings', toAutomationRecordings(), '/automations/recordings'],
    ['automation id with a slash', toAutomation('ws/nightly'), '/automations/ws%2Fnightly'],
    ['edit automation', toAutomationEdit('ws/nightly'), '/automations/ws%2Fnightly/edit'],
    ['models', toModels(), '/models'],
    ['settings', toSettings(), '/settings'],
    ['settings section', toSettings('mcp'), '/settings/mcp'],
  ])('%s', (_label, to, path) => {
    expect(pathOf(to)).toBe(path)
  })

  it('round-trips a nested file path and an automation id through params', () => {
    expect(router.resolve(toWorkspaceFile('ws', 'docs/a b.md')).params.path).toEqual(['docs', 'a b.md'])
    expect(router.resolve(toAutomation('ws/nightly')).params.id).toBe('ws/nightly')
  })

  it('puts only the set Activity filters in the query', () => {
    expect(pathOf(toActivity())).toBe('/activity')
    expect(pathOf(toActivity({ kind: 'automation', status: 'failed', q: '' }))).toBe('/activity?kind=automation&status=failed')
  })
})

describe('workspaceLocation', () => {
  const at = (path: string) => workspaceLocation(router.resolve(path))
  const EMPTY = { ws: null, filePath: '', section: null, assistant: false, conversationId: null }

  it.each([
    ['/workspaces', EMPTY],
    ['/workspaces/ws/files', { ...EMPTY, ws: 'ws' }],
    ['/workspaces/ws/files/docs/a.md', { ...EMPTY, ws: 'ws', filePath: 'docs/a.md' }],
    ['/workspaces/ws/assistant', { ...EMPTY, ws: 'ws', assistant: true }],
    ['/workspaces/ws/assistant/c1', { ...EMPTY, ws: 'ws', assistant: true, conversationId: 'c1' }],
    ['/workspaces/ws/memory', { ...EMPTY, ws: 'ws', section: 'memory' }],
    ['/overview', EMPTY],
  ])('%s', (path, expected) => {
    expect(at(path)).toEqual(expected)
  })
})
