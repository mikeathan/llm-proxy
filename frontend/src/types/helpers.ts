// Why helpers cannot run: no model named, the named one removed from the models, or not a cloud model ('' when they can).
export type HelpersProblem = '' | 'unset' | 'missing' | 'not_cloud'

// Whether a chat can use helpers (sub-agents), as the server answers it (backend GET /admin/api/helpers).
export interface HelpersStatus {
  /** A cloud helper model is set up, so "Use helpers" can work. */
  available: boolean
  /** The helper model Settings names ('' when none), even when it cannot be used. */
  model: string
  /** Why it cannot run, so the page can name the fix; absent when it can. */
  problem?: HelpersProblem
}
