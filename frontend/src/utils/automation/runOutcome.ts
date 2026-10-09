import type { AutomationRun, RunOutcome } from '../../types/dispatcher'
import type { StatusState } from '../../types/ui'

// The one rule for a finished run's tag, used wherever a run is listed or shown.
const OUTCOME_TAG: Record<RunOutcome, { state: StatusState; label: string }> = {
  failed: { state: 'error', label: 'Failed' },
  warning: { state: 'warning', label: 'Completed with warnings' },
  completed: { state: 'success', label: 'Completed' },
}

type RunResult = Pick<AutomationRun, 'error' | 'warnings'>

/** Failed beats warnings; a run with warnings and no error still completed. */
export function runOutcome(run: RunResult): RunOutcome {
  if (run.error) return 'failed'
  return run.warnings?.length ? 'warning' : 'completed'
}

/** The StatusTag state and label for a run. */
export function runOutcomeTag(run: RunResult): { state: StatusState; label: string } {
  return OUTCOME_TAG[runOutcome(run)]
}
