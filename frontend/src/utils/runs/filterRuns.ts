import type { AutomationRun } from '../../types/dispatcher'
import type { ActivityFilters, RunStatusFilter } from '../../types/routes'
import { RUN_STATUS_FILTERS } from '../../types/routes'

const isStatusFilter = (value: string | undefined): value is RunStatusFilter =>
  (RUN_STATUS_FILTERS as readonly (string | undefined)[]).includes(value)

const matchesStatus = (run: AutomationRun, status: RunStatusFilter) => (status === 'failed' ? !!run.error : !run.error)

function matchesQuery(run: AutomationRun, query: string): boolean {
  return [run.automation_name, run.workspace_id, run.model, run.error, run.id]
    .some((field) => field?.toLowerCase().includes(query))
}

/**
 * The Activity run list: filtered by outcome, workspace and a free-text query
 * (the route query, plan D18), newest first. Unknown values are ignored.
 */
export function filterRuns(runs: AutomationRun[], filters: Pick<ActivityFilters, 'status' | 'workspace' | 'q'>): AutomationRun[] {
  const query = filters.q?.trim().toLowerCase() ?? ''
  const status = isStatusFilter(filters.status) ? filters.status : null
  return runs
    .filter((run) => !status || matchesStatus(run, status))
    .filter((run) => !filters.workspace || run.workspace_id === filters.workspace)
    .filter((run) => !query || matchesQuery(run, query))
    .sort((a, b) => Date.parse(b.timestamp) - Date.parse(a.timestamp))
}

/** The distinct workspaces that appear in the runs, sorted — for the filter. */
export function runWorkspaces(runs: AutomationRun[]): string[] {
  return [...new Set(runs.map((run) => run.workspace_id).filter((ws): ws is string => !!ws))].sort()
}
