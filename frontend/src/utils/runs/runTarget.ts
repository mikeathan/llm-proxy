import type { RouteLocationNamedRaw } from 'vue-router'
import { toActivity, toAutomation, toWorkspaceAssistant } from '../../router/routes'
import type { LaneHolder, QueuedRun } from '../../types/assistant'
import type { AutomationRun } from '../../types/dispatcher'

// A row of running (or waiting) work, as the run scheduler reports it.
type RunRow = LaneHolder | QueuedRun

/**
 * Where a running or waiting row should take the operator, or null when there is
 * nowhere to go. A chat opens its exact conversation once the server knows it
 * (the workspace chat before that); an automation opens its page, which shows the
 * live run. An external API caller has no page, and a row missing the workspace or
 * automation it names cannot be placed.
 */
export function runTarget(row: RunRow): RouteLocationNamedRaw | null {
  if (row.kind === 'interactive' && row.workspace_id) {
    return toWorkspaceAssistant(row.workspace_id, 'conversation_id' in row ? row.conversation_id : undefined)
  }
  if (row.kind === 'automation' && row.workspace_id && row.automation) {
    return toAutomation(`${row.workspace_id}/${row.automation}`)
  }
  return null
}

/** The row's name for people: a chat by its workspace, not by the scheduler's `chat:<workspace>` key. */
export function runTitle(row: RunRow): string {
  if (row.kind === 'interactive' && row.workspace_id) return `Chat in ${row.workspace_id}`
  return row.label
}

/** A finished run opens in the Activity ledger, where its details are addressable. */
export function historicalRunTarget(run: AutomationRun): RouteLocationNamedRaw {
  return toActivity({ run: run.id })
}

/** The automation a finished run belongs to; null when the run names no workspace. */
export function automationTarget(run: AutomationRun): RouteLocationNamedRaw | null {
  return run.workspace_id ? toAutomation(`${run.workspace_id}/${run.automation_name}`) : null
}
