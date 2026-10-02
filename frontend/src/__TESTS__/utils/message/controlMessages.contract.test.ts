import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, it, expect } from 'vitest'
import { isInternalMessage } from '../../../utils/message/turnGrouper'

// Contract with the backend: every message isAgentControlMessage
// (backend/internal/core/assistant/session.go) treats as agent-internal must be
// hidden by the chat UI too. The list is read from the Go source, so a new
// control prompt added there fails this test until the UI filter knows it.
const BACKEND = resolve(__dirname, '../../../../../backend/internal/core/assistant')
const session = readFileSync(resolve(BACKEND, 'session.go'), 'utf8')
const templates = readFileSync(resolve(BACKEND, 'prompts/templates.go'), 'utf8')

// Below this the parsing below has broken, not the contract.
const MIN_CONTROL_PROMPTS = 15

function controlFunctionBody(): string {
  const start = session.indexOf('func isAgentControlMessage(')
  const end = session.indexOf('\n}\n', start)
  expect(start, 'isAgentControlMessage not found in session.go').toBeGreaterThan(-1)
  return session.slice(start, end)
}

// The first string literal a Go constant starts with: its fixed prefix.
function constantPrefix(name: string): string {
  const match = templates.match(new RegExp(`\\b${name}\\s*=\\s*("(?:[^"\\\\]|\\\\.)*"|\`[^\`]*\`)`))
  expect(match, `${name} not found in templates.go`).not.toBeNull()
  const literal = match![1]!
  return literal.startsWith('`') ? literal.slice(1, -1) : (JSON.parse(literal) as string)
}

function controlPrefixes(): string[] {
  const body = controlFunctionBody()
  const constants = [...new Set([...body.matchAll(/prompts\.(\w+)/g)].map((m) => m[1]!))]
  const literals = [...body.matchAll(/HasPrefix\(content, "((?:[^"\\]|\\.)*)"\)/g)].map((m) => JSON.parse(`"${m[1]}"`) as string)
  return [...constants.map(constantPrefix), ...literals]
}

describe('agent control messages (contract with session.go)', () => {
  const prefixes = controlPrefixes()

  it('reads the whole list from the backend', () => {
    expect(prefixes.length).toBeGreaterThanOrEqual(MIN_CONTROL_PROMPTS)
  })

  it.each(prefixes.map((p) => [p.slice(0, 48), p]))('hides "%s…"', (_label, prefix) => {
    expect(isInternalMessage({ role: 'user', content: `${prefix} trailing detail` })).toBe(true)
  })

  it('still shows what the operator typed', () => {
    expect(isInternalMessage({ role: 'user', content: 'search for the latest news' })).toBe(false)
    expect(isInternalMessage({ role: 'user', content: '[draft] my own bracketed note' })).toBe(false)
    expect(isInternalMessage({ role: 'user', content: 'CRITICAL: prod is down, check the logs' })).toBe(false)
    expect(isInternalMessage({ role: 'user', content: '[System: please summarise]' })).toBe(false)
  })

  it('hides the empty "[stuck]" placeholder the backend writes for a stalled model', () => {
    expect(isInternalMessage({ role: 'assistant', content: '', reasoning_content: '[stuck]' })).toBe(true)
    expect(isInternalMessage({ role: 'assistant', content: 'done', reasoning_content: '[stuck]' })).toBe(false)
  })
})
