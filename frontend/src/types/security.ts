/** What the agent security record keeps: a refused tool call, an approval asked, a new destination, a secret found. */
export type AgentWatchKind = 'tool_refused' | 'approval_asked' | 'new_destination' | 'secret_found'

/** One entry of the agent security record. Plain names only: a tool, a host, a workspace path, a kind of secret. */
export interface AgentWatchEvent {
  at: string
  kind: AgentWatchKind | string
  workspace: string
  /** The automation of the run; absent for a chat. */
  automation?: string
  tool?: string
  host?: string
  where?: string
  detail?: string
}

/** What the record learned about one automation's destinations. */
export interface AgentWatchAutomation {
  workspace: string
  automation: string
  hosts: number
  /** True during the first week, while its usual hosts are learned. */
  learning: boolean
  learning_until: string
  /** It reached many hosts while learning (a news or research job): new hosts are recorded, never alerted. */
  browses: boolean
}

/** GET /admin/api/agent-watch. */
export interface AgentWatchView {
  events: AgentWatchEvent[]
  automations: AgentWatchAutomation[]
}

/** The lock down switch (GET/POST/DELETE /admin/api/lockdown). */
export interface LockdownState {
  on: boolean
  since?: string
  signed_out?: number
  tokens_revoked?: number
}
