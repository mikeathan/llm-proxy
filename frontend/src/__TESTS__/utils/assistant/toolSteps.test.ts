import { describe, it, expect } from 'vitest'
import { describeToolStep, formatToolArguments, formatToolResult, TOOL_RESULT_PREVIEW_CHARS } from '../../../utils/assistant/toolSteps'

describe('describeToolStep', () => {
  it.each([
    ['internet_search', '{"query":"Claude Haiku 5.5"}', { kind: 'search', verb: 'Searched the web', target: 'Claude Haiku 5.5' }],
    ['fetch_url', '{"url":"https://example.com/a"}', { kind: 'fetch', verb: 'Read a web page', target: 'https://example.com/a' }],
    ['execute_terminal_command', '{"command":"node --version"}', { kind: 'terminal', verb: 'Ran a command', target: 'node --version' }],
    ['read_file', '{"path":"notes/a.md"}', { kind: 'read', verb: 'Read a file', target: 'notes/a.md' }],
    ['write_file', '{"path":"out.txt","content":"x"}', { kind: 'write', verb: 'Wrote a file', target: 'out.txt' }],
    ['append_file', '{"path":"log.txt","content":"x"}', { kind: 'write', verb: 'Added to a file', target: 'log.txt' }],
    ['edit_file_block', '{"path":"a.ts","old_block":"x","new_block":"y"}', { kind: 'edit', verb: 'Edited a file', target: 'a.ts' }],
    ['list_directory', '{"path":"."}', { kind: 'list', verb: 'Listed a folder', target: '.' }],
    ['memory_search', '{"query":"incident"}', { kind: 'memory-search', verb: 'Searched memory', target: 'incident' }],
    ['memory_update', '{"content":"The commander is Marcus."}', { kind: 'memory-save', verb: 'Saved to memory', target: 'The commander is Marcus.' }],
    ['scan_local_network', '{}', { kind: 'network', verb: 'Scanned the local network', target: '' }],
    ['notify_user', '{"message":"done"}', { kind: 'notify', verb: 'Sent a notification', target: 'done' }],
  ])('%s reads as a plain step', (name, args, expected) => {
    expect(describeToolStep(name, args)).toEqual(expected)
  })

  it('names an unknown or MCP tool by itself, with its first text argument', () => {
    expect(describeToolStep('github__create_issue', '{"title":"Bug","body":"…"}')).toEqual({ kind: 'other', verb: 'Used github__create_issue', target: 'Bug' })
  })

  it('copes with arguments that are not JSON', () => {
    expect(describeToolStep('read_file', '{"path": broken')).toEqual({ kind: 'read', verb: 'Read a file', target: '' })
  })
})

describe('formatToolArguments', () => {
  it('lists each argument, with text as-is and anything else as JSON', () => {
    expect(formatToolArguments('{"path":"a.md","lines":[1,2],"force":true}')).toEqual([
      { key: 'path', value: 'a.md' },
      { key: 'lines', value: '[1,2]' },
      { key: 'force', value: 'true' },
    ])
  })

  it('shows unparseable arguments whole', () => {
    expect(formatToolArguments('not json')).toEqual([{ key: 'arguments', value: 'not json' }])
    expect(formatToolArguments('')).toEqual([])
  })
})

describe('formatToolResult', () => {
  it('decodes a JSON-encoded string into real lines', () => {
    expect(formatToolResult('"v26.0.0\\n11.12.1\\n"')).toEqual({ kind: 'text', text: 'v26.0.0\n11.12.1\n', truncated: false })
  })

  it('shows search results as hits, keeping only http(s) links', () => {
    const raw = JSON.stringify([
      { title: 'Haiku 5.5', url: 'https://cellcog.ai/blog', snippet: 'Confirmed.' },
      { title: 'Trap', url: 'javascript:alert(1)', snippet: 'x' },
    ])
    expect(formatToolResult(raw)).toEqual({
      kind: 'search',
      hits: [
        { title: 'Haiku 5.5', url: 'https://cellcog.ai/blog', snippet: 'Confirmed.' },
        { title: 'Trap', url: null, snippet: 'x' },
      ],
    })
  })

  it('pretty-prints other JSON and keeps plain text as it is', () => {
    expect(formatToolResult('{"saved":true}')).toEqual({ kind: 'json', text: '{\n  "saved": true\n}', truncated: false })
    expect(formatToolResult('saved to memory (id: 8)')).toEqual({ kind: 'text', text: 'saved to memory (id: 8)', truncated: false })
  })

  it('cuts a very large result to a preview unless asked for all of it', () => {
    const big = 'x'.repeat(TOOL_RESULT_PREVIEW_CHARS + 10)
    const preview = formatToolResult(big)
    expect(preview).toEqual({ kind: 'text', text: 'x'.repeat(TOOL_RESULT_PREVIEW_CHARS), truncated: true })
    expect(formatToolResult(big, { full: true })).toEqual({ kind: 'text', text: big, truncated: false })
  })
})
