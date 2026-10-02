import type { SearchHit, ToolArgument, ToolKind, ToolResultView, ToolStepView } from '../../types/toolSteps'

// How the assistant's tool calls read in the step timeline (ChatBubble). Tool
// names follow the backend manifests (backend/internal/core/tools/manifests);
// anything else — an MCP tool, a tool added later — is named as it is.

interface ToolDef {
  kind: ToolKind
  verb: string
  /** The argument that names the step's target. */
  argKey?: string
}

const TOOLS: Record<string, ToolDef> = {
  internet_search: { kind: 'search', verb: 'Searched the web', argKey: 'query' },
  fetch_url: { kind: 'fetch', verb: 'Read a web page', argKey: 'url' },
  execute_terminal_command: { kind: 'terminal', verb: 'Ran a command', argKey: 'command' },
  read_file: { kind: 'read', verb: 'Read a file', argKey: 'path' },
  write_file: { kind: 'write', verb: 'Wrote a file', argKey: 'path' },
  append_file: { kind: 'write', verb: 'Added to a file', argKey: 'path' },
  edit_file_block: { kind: 'edit', verb: 'Edited a file', argKey: 'path' },
  list_directory: { kind: 'list', verb: 'Listed a folder', argKey: 'path' },
  memory_search: { kind: 'memory-search', verb: 'Searched memory', argKey: 'query' },
  memory_update: { kind: 'memory-save', verb: 'Saved to memory', argKey: 'content' },
  scan_local_network: { kind: 'network', verb: 'Scanned the local network' },
  get_network_info: { kind: 'network', verb: 'Read network settings' },
  notify_user: { kind: 'notify', verb: 'Sent a notification', argKey: 'message' },
}

/** The icon each kind of step shows (names in assets/svg). */
export const TOOL_KIND_ICON: Record<ToolKind, string> = {
  search: 'search',
  fetch: 'globe',
  terminal: 'terminal',
  read: 'document',
  write: 'edit',
  edit: 'edit',
  list: 'nav-workspaces',
  'memory-search': 'memory',
  'memory-save': 'memory',
  network: 'radio',
  notify: 'bell',
  other: 'tool',
}

/** Characters of a tool result shown before "Show all". */
export const TOOL_RESULT_PREVIEW_CHARS = 4000
const JSON_INDENT = 2
const SAFE_URL = /^https?:\/\//i
const OTHER_VERB_PREFIX = 'Used '

function parseObject(json: string): Record<string, unknown> | null {
  try {
    const value: unknown = JSON.parse(json)
    return value && typeof value === 'object' && !Array.isArray(value) ? (value as Record<string, unknown>) : null
  } catch {
    return null
  }
}

const firstText = (args: Record<string, unknown>): string =>
  Object.values(args).find((value): value is string => typeof value === 'string') ?? ''

/** The step a tool call reads as: its kind, a verb and its target. */
export function describeToolStep(name: string, rawArgs: string): ToolStepView {
  const args = parseObject(rawArgs) ?? {}
  const def = TOOLS[name]
  if (!def) return { kind: 'other', verb: `${OTHER_VERB_PREFIX}${name}`, target: firstText(args) }
  const value = def.argKey ? args[def.argKey] : undefined
  return { kind: def.kind, verb: def.verb, target: typeof value === 'string' ? value : '' }
}

/** A call's arguments as key/value rows: text as-is, anything else as JSON. */
export function formatToolArguments(rawArgs: string): ToolArgument[] {
  if (!rawArgs.trim()) return []
  const args = parseObject(rawArgs)
  if (!args) return [{ key: 'arguments', value: rawArgs }]
  return Object.entries(args).map(([key, value]) => ({ key, value: typeof value === 'string' ? value : JSON.stringify(value) }))
}

function asSearchHits(value: unknown): SearchHit[] | null {
  if (!Array.isArray(value) || !value.length) return null
  const isHit = (item: unknown): item is Record<string, unknown> =>
    !!item && typeof item === 'object' && typeof (item as Record<string, unknown>).title === 'string'
  if (!value.every(isHit)) return null
  return value.map((item) => {
    const url = typeof item.url === 'string' && SAFE_URL.test(item.url) ? item.url : null
    return { title: String(item.title), url, snippet: typeof item.snippet === 'string' ? item.snippet : '' }
  })
}

function preview(kind: 'text' | 'json', text: string, full: boolean): ToolResultView {
  if (full || text.length <= TOOL_RESULT_PREVIEW_CHARS) return { kind, text, truncated: false }
  return { kind, text: text.slice(0, TOOL_RESULT_PREVIEW_CHARS), truncated: true }
}

/**
 * A tool result decoded for reading. Results often arrive JSON-encoded (a
 * terminal's output as a quoted string with \n escapes): a string decodes to
 * its real lines, a list of {title,url} to search hits, other JSON is
 * pretty-printed. Past the preview size only the start is kept, unless `full`.
 */
export function formatToolResult(raw: string, options: { full?: boolean } = {}): ToolResultView {
  const full = options.full ?? false
  let value: unknown
  try {
    value = JSON.parse(raw)
  } catch {
    return preview('text', raw, full)
  }
  if (typeof value === 'string') return preview('text', value, full)
  const hits = asSearchHits(value)
  if (hits) return { kind: 'search', hits }
  return preview('json', JSON.stringify(value, null, JSON_INDENT), full)
}
