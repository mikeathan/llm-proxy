// How a tool call reads in the assistant's step timeline (ChatBubble): a kind
// (icon), a plain-language verb and the argument that names its target.

export const TOOL_KINDS = [
  'search',
  'fetch',
  'terminal',
  'read',
  'write',
  'edit',
  'list',
  'memory-search',
  'memory-save',
  'network',
  'notify',
  'other',
] as const
export type ToolKind = (typeof TOOL_KINDS)[number]

export interface ToolStepView {
  kind: ToolKind
  /** Past tense once finished, e.g. "Searched the web". */
  verb: string
  /** The argument that names what the step acted on ('' when none). */
  target: string
}

/** One argument of a tool call, as shown in an expanded step. */
export interface ToolArgument {
  key: string
  value: string
}

export interface SearchHit {
  title: string
  /** http(s) only; null when the result carried anything else. */
  url: string | null
  snippet: string
}

// A tool result, decoded for display (results often arrive JSON-encoded).
export type ToolResultView =
  | { kind: 'search'; hits: SearchHit[] }
  | { kind: 'text'; text: string; truncated: boolean }
  | { kind: 'json'; text: string; truncated: boolean }
