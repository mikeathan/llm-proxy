// Whether a chat can use helpers (sub-agents), as the server answers it (backend GET /admin/api/helpers).
export interface HelpersStatus {
  /** A cloud helper model is set up, so "Use helpers" can work. */
  available: boolean
  /** The helper model Settings names ('' when none), even when it cannot be used. */
  model: string
}
