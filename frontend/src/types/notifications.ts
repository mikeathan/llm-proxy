import type { RouteLocationNamedRaw } from 'vue-router'
import type { Destination } from './routes'

// Run notifications (plan Phase 4, D5/D20): terminal transitions only.
// An assistant run can only be known to have ended — the lane snapshot has no
// status — so it is never "completed" or "failed". An automation's outcome
// comes from its run record (AutomationRun.error); without one it also "ended".
export type RunEndOutcome = 'ended' | 'completed' | 'failed'
export type RunEndKind = 'automation' | 'assistant'

export interface RunEndItem {
  /** Run identity (lane key + start time). */
  id: string
  kind: RunEndKind
  label: string
  workspace: string
  /** Set for automation runs (the lane key is the automation id). */
  automationId?: string
  outcome: RunEndOutcome
  /** The run's error text, for a failed automation. */
  error?: string
  target: RouteLocationNamedRaw
  destination: Destination
}

/** Runs that ended in the same poll tick, shown as one notification. */
export interface RunNotification {
  id: string
  createdAt: number
  items: RunEndItem[]
}
