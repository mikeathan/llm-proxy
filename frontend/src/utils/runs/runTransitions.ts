import type { LaneHolder } from '../../types/assistant'

/**
 * A run's identity: its lane key plus its start time (plan D20). The lane key
 * alone names a lane, not a run — deduping on it would hide every later run in
 * the same workspace.
 */
export function runIdentity(holder: LaneHolder): string {
  return `${holder.key}@${holder.since}`
}

/** Runs present in `previous` and absent from `next`: the runs that ended. */
export function endedRuns(previous: LaneHolder[], next: LaneHolder[]): LaneHolder[] {
  const stillRunning = new Set(next.map(runIdentity))
  return previous.filter((holder) => !stillRunning.has(runIdentity(holder)))
}
