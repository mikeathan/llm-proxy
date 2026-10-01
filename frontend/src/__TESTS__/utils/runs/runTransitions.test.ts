import { describe, it, expect } from 'vitest'
import { endedRuns, runIdentity } from '../../../utils/runs/runTransitions'
import type { LaneHolder } from '../../../types/assistant'

const holder = (key: string, since: string): LaneHolder => ({ key, kind: 'interactive', workspace_id: 'ws', label: key, since })
const A1 = holder('chat:ws', '2026-09-28T10:00:00Z')
const A2 = holder('chat:ws', '2026-09-28T10:05:00Z')
const B1 = holder('ws/nightly', '2026-09-28T10:01:00Z')

describe('endedRuns', () => {
  it('reports holders that are gone', () => {
    expect(endedRuns([A1, B1], [B1])).toEqual([A1])
  })

  it('reports nothing for identical snapshots', () => {
    expect(endedRuns([A1, B1], [A1, B1])).toEqual([])
  })

  it('treats the same lane with a new start time as the old run ending', () => {
    expect(endedRuns([A1], [A2])).toEqual([A1])
  })

  it('reports a lane that disappears and later reappears once per run', () => {
    expect(endedRuns([A1], [])).toEqual([A1])
    expect(endedRuns([], [A2])).toEqual([])
    expect(endedRuns([A2], [])).toEqual([A2])
  })

  it('identifies a run by lane key and start time', () => {
    expect(runIdentity(A1)).not.toBe(runIdentity(A2))
  })
})
