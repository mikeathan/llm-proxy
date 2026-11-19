import type { McpAuthKind, McpProblem, McpServerForm, McpStatus, McpTrust } from '../types/mcp'

// The Agents page's Tools tab (ADR-0069): the words for MCP servers, in plain English with the next step for every way a connection can fail.

export const MCP_TAB_INTRO =
  'MCP servers give the agents more tools: files, a database, a calendar. Each tool asks you first until you set its server to Allow. Nothing is contacted until you add a server.'
export const MCP_ADD_LABEL = 'Add a server'
export const MCP_EMPTY_TITLE = 'No MCP servers'
export const MCP_EMPTY_BODY = 'Add the address of a server to give the agents its tools. A tool never runs without your say-so until you set its server to Allow.'
export const MCP_LOAD_FAILED_TITLE = 'MCP servers unavailable'
export const MCP_LOAD_FAILED_NEXT = 'Reload the page. If it stays this way, check that the service is running.'
export const MCP_OFF_TITLE = 'MCP servers are switched off'
export const MCP_OFF_BODY = 'No server is contacted and no tool is offered until the module is switched on.'
export const MCP_OFF_LINK = 'Switch it on the Modules tab'

export const MCP_TRUST_LABELS: Record<McpTrust, string> = { ask: 'Ask each time', allow: 'Allow' }
export const MCP_TRUST_HINTS: Record<McpTrust, string> = {
  ask: 'Every call of this server\'s tools waits for your approval. Automations and other unattended runs cannot use it.',
  allow: 'Calls run without asking, including in unattended runs. Choose this only for a server you trust.',
}
export const MCP_TRUST_LABEL = 'When an agent wants to use a tool'

export const MCP_AUTH_LABELS: Record<McpAuthKind, string> = { none: 'None', bearer: 'Bearer token', header: 'A header' }
export const MCP_AUTH_LABEL = 'Sign in with'

export const MCP_STATUS_LABELS: Record<McpStatus, string> = { connecting: 'Connecting', ready: 'Connected', stopped: 'Not working', off: 'Off' }

export const MCP_FIELD_NAME = 'Name'
export const MCP_FIELD_NAME_HINT = 'Lower-case letters, digits and hyphens. It starts the name of each tool, like mcp__name__tool.'
export const MCP_FIELD_URL = 'Address'
export const MCP_FIELD_URL_HINT = 'The MCP endpoint, usually ending in /mcp.'
export const MCP_FIELD_HEADER = 'Header name'
export const MCP_FIELD_SECRET_BEARER = 'Token'
export const MCP_FIELD_SECRET_HEADER = 'Header value'
export const MCP_SECRET_KEEP_HINT = 'A value is stored, sealed. Leave this empty to keep it; type a new one to replace it.'
export const MCP_SECRET_NEW_HINT = 'Stored sealed and never shown again.'
export const MCP_SECRET_PLACEHOLDER_STORED = 'Stored (leave empty to keep)'
export const MCP_ADVANCED_LABEL = 'Advanced'
export const MCP_FIELD_CA = 'Certificate authority file'
export const MCP_FIELD_CA_HINT = 'The full path of a .pem file, for a server whose certificate your own authority signed.'
export const MCP_HOST_CHANGED_HINT = 'The address changed, so type the sign-in value again: a stored value is never sent to another address.'

export const MCP_TEST_LABEL = 'Test connection'
export const MCP_SAVE_ADD = 'Add server'
export const MCP_SAVE_EDIT = 'Save changes'
export const MCP_CANCEL = 'Cancel'
export const MCP_EDIT = 'Edit'
export const MCP_REMOVE = 'Remove'
export const MCP_TOOLS_SHOW = 'Show tools'
export const MCP_TOOLS_HIDE = 'Hide tools'
export const MCP_NO_TOOLS = 'This server offers no tools.'
export const MCP_TOOL_OFF_HINT = 'A tool you switch off is not offered to the agents.'
export const MCP_TEST_RESULT_OK = 'Connected.'

export const mcpToolCount = (n: number): string => (n === 1 ? '1 tool' : `${n} tools`)
export const mcpRemoveTitle = (name: string): string => `Remove ${name}?`
export const MCP_REMOVE_MESSAGE = 'The server and its stored sign-in value are deleted. Its tools stop being offered at once.'
export const mcpAddedToast = (name: string): string => `${name} added`
export const mcpSavedToast = (name: string): string => `${name} saved`
export const mcpRemovedToast = (name: string): string => `${name} removed`
export const MCP_ACTION_FAILED = 'The change could not be made.'
export const MCP_TEST_FAILED = 'The test could not be run.'

/** What to do about each way a connection can fail. A problem the screen has no advice for shows the backend's own words. */
export const MCP_PROBLEM_ADVICE: Record<McpProblem, string> = {
  unreachable: 'Check the address and that the server is running and reachable from this computer.',
  guard: 'Guardrails refused this address. Allow local network access (or internet access, for a public server) under Settings → Guardrails.',
  sign_in: 'The server wants a sign-in. Edit the server and give it the token or header it expects.',
  forbidden: 'The server did not accept the sign-in. Check that the token is the right one and has access.',
  not_mcp: 'The address answered, but not as an MCP server. Check the path; MCP servers usually end in /mcp.',
  tls: 'The certificate is not trusted. If the server uses your own certificate authority, give its file under Advanced.',
  timeout: 'The server took too long to answer. Try again; if it keeps happening, check the server.',
  version: 'This program cannot speak the version of MCP this server uses.',
  secret: 'The stored sign-in value could not be opened (the keys were reset). Edit the server and type it again.',
  sign_in_incomplete: 'The sign-in has no value. Edit the server and type the token or header value.',
  ca_file: 'The certificate authority file could not be read. Check the path and that the service may read it.',
  closed: 'The server closed the connection. It reconnects on its own.',
  too_many_tools: 'The server offers more tools than are shown. Switch off the ones the agents do not need.',
  invalid: 'The entry is not valid. Edit the server and fix the name or the address.',
}

export const blankMcpForm = (): McpServerForm => ({ name: '', url: '', trust: 'ask', auth_kind: 'none', auth_header: '', secret: '', ca_cert: '' })

/** The status tag's state for each status. */
export const MCP_STATUS_STATE = { connecting: 'running', ready: 'success', stopped: 'error', off: 'neutral' } as const
