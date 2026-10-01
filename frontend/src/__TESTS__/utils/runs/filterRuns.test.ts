import { describe, it, expect } from 'vitest'
import { filterRuns, runWorkspaces } from '../../../utils/runs/filterRuns'
import type { AutomationRun } from '../../../types/dispatcher'

const run = (id: string, over: Partial<AutomationRun> = {}): AutomationRun =>
  ({ id, workspace_id: 'ws', automation_name: `auto-${id}`, timestamp: `2026-09-29T10:0${id}:00Z`, error: '', output: '', duration_ms: 5, model: 'qwen', ...over })
const RUNS = [run('1'), run('2', { error: 'model not configured' }), run('3', { workspace_id: 'lab', model: 'gpt-5' })]
const ids = (runs: AutomationRun[]) => runs.map((r) => r.id)

describe('filterRuns', () => {
  it('returns every run newest first with no filters', () => {
    expect(ids(filterRuns(RUNS, {}))).toEqual(['3', '2', '1'])
  })

  it('filters by outcome', () => {
    expect(ids(filterRuns(RUNS, { status: 'failed' }))).toEqual(['2'])
    expect(ids(filterRuns(RUNS, { status: 'completed' }))).toEqual(['3', '1'])
  })

  it('filters by workspace', () => {
    expect(ids(filterRuns(RUNS, { workspace: 'lab' }))).toEqual(['3'])
  })

  it('searches name, workspace, model, error and id, case-insensitively', () => {
    expect(ids(filterRuns(RUNS, { q: 'AUTO-1' }))).toEqual(['1'])
    expect(ids(filterRuns(RUNS, { q: 'gpt' }))).toEqual(['3'])
    expect(ids(filterRuns(RUNS, { q: 'not configured' }))).toEqual(['2'])
    expect(ids(filterRuns(RUNS, { q: '  ' }))).toEqual(['3', '2', '1'])
  })

  it('combines filters', () => {
    expect(ids(filterRuns(RUNS, { workspace: 'ws', status: 'completed', q: 'qwen' }))).toEqual(['1'])
  })

  it('ignores an unknown status from a hand-edited link', () => {
    expect(ids(filterRuns(RUNS, { status: 'bogus' }))).toEqual(['3', '2', '1'])
  })
})

describe('runWorkspaces', () => {
  it('lists the distinct workspaces, sorted', () => {
    expect(runWorkspaces(RUNS)).toEqual(['lab', 'ws'])
  })
})
