// MCP servers of the Agents stack's Tools layer (ADR-0069). The server list, its status and the tools each one offers come from one route; the sign-in
// value is only ever sent, never read back.

export const MCP_TRUSTS = ['ask', 'allow'] as const
export type McpTrust = (typeof MCP_TRUSTS)[number]

export const MCP_AUTH_KINDS = ['none', 'bearer', 'header'] as const
export type McpAuthKind = (typeof MCP_AUTH_KINDS)[number]

/** How one server's connection is doing; "off" is a server the owner switched off, which has no connection. */
export const MCP_STATUSES = ['connecting', 'ready', 'stopped', 'off'] as const
export type McpStatus = (typeof MCP_STATUSES)[number]

/** Why a connection is not working: the ids the backend gives (the advice shown is keyed on them). */
export const MCP_PROBLEMS = [
  'unreachable', 'guard', 'sign_in', 'forbidden', 'not_mcp', 'tls', 'timeout', 'version', 'secret',
  'sign_in_incomplete', 'ca_file', 'closed', 'too_many_tools', 'invalid',
] as const
export type McpProblem = (typeof MCP_PROBLEMS)[number]

export interface McpTool {
  name: string
  description?: string
  disabled?: boolean
}

export interface McpServer {
  name: string
  url: string
  enabled: boolean
  trust: McpTrust
  auth_kind: McpAuthKind
  auth_header?: string
  /** Whether a sign-in value is stored (never the value). */
  has_secret: boolean
  ca_cert?: string
  status: McpStatus
  /** The backend's words for the problem, shown when the id is not one the screen has advice for. */
  problem?: string
  problem_code?: string
  server_name?: string
  server_version?: string
  protocol?: string
  transport?: 'streamable_http' | 'sse'
  instructions?: string
  tools: McpTool[]
  /** The names of the tools the owner switched off, kept even while the server is down and lists none. */
  disabled_tools: string[]
}

/** What the list route answers: the module's switch, so the page can say why nothing runs, and the servers. */
export interface McpServersAnswer {
  enabled: boolean
  servers: McpServer[]
}

/** What the add and edit form holds. `secret` is typed text; an empty one on an edit means keep the stored value. */
export interface McpServerForm {
  name: string
  url: string
  trust: McpTrust
  auth_kind: McpAuthKind
  auth_header: string
  secret: string
  ca_cert: string
}

/** The body of a write: the form plus the parts the card changes on its own (the switch and the tools switched off). */
export interface McpServerBody extends Omit<McpServerForm, 'secret'> {
  enabled: boolean
  disabled_tools: string[]
  secret?: string
}

/** What a test sends: a saved server by name, or an entry as typed (with its sign-in value) that is not saved. */
export type McpTestRequest = { name: string } | McpServerBody
