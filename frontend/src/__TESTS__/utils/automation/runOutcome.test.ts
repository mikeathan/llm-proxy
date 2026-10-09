import { describe, it, expect } from 'vitest'
import { runOutcome, runOutcomeTag } from '../../../utils/automation/runOutcome'

// One rule for what a finished run's tag says, everywhere a run is listed.
describe('runOutcome', () => {
  it('failed beats warnings; warnings without an error still completed, but say so', () => {
    expect(runOutcome({ error: 'boom', warnings: ['x'] })).toBe('failed')
    expect(runOutcome({ error: '', warnings: ['report not delivered via tg'] })).toBe('warning')
    expect(runOutcome({ error: '', warnings: [] })).toBe('completed')
    expect(runOutcome({ error: '' })).toBe('completed')
  })

  it('maps each outcome to a labelled tag; colour never carries the meaning alone', () => {
    expect(runOutcomeTag({ error: 'boom' })).toEqual({ state: 'error', label: 'Failed' })
    expect(runOutcomeTag({ error: '', warnings: ['x'] })).toEqual({ state: 'warning', label: 'Completed with warnings' })
    expect(runOutcomeTag({ error: '' })).toEqual({ state: 'success', label: 'Completed' })
  })
})
