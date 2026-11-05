import { describe, expect, it } from 'vitest'
import { agentEventSource, agentEventText, automationLine } from '../../../utils/security/agentWatch'

describe('agent watch wording', () => {
  it('says each kind in one plain line and names where it came from', () => {
    expect(agentEventText({ at: '', kind: 'tool_refused', workspace: 'ws', automation: 'nightly', tool: 'read_file' })).toBe('The safety rules refused read_file')
    expect(agentEventText({ at: '', kind: 'approval_asked', workspace: 'ws', tool: 'write_file' })).toBe('Asked you to approve write_file')
    expect(agentEventText({ at: '', kind: 'new_destination', workspace: 'ws', automation: 'weather', host: '203.0.113.9' })).toBe('Fetched from a new host: 203.0.113.9')
    expect(agentEventText({ at: '', kind: 'secret_found', workspace: 'ws', where: 'reports/a.md', detail: 'an AWS access key' })).toBe('An AWS access key was left in reports/a.md')
    expect(agentEventText({ at: '', kind: 'something_new', workspace: 'ws' })).toBe('Recorded: something_new')
    expect(agentEventSource({ at: '', kind: 'tool_refused', workspace: 'ws', automation: 'nightly' })).toBe('nightly · ws')
    expect(agentEventSource({ at: '', kind: 'tool_refused', workspace: 'ws' })).toBe('a chat · ws')
    expect(agentEventSource({ at: '', kind: 'secret_found', workspace: 'ws', where: 'a.md' })).toBe('ws') // found by the nightly pass, not a run
  })

  it('says what the destination watch does for each automation', () => {
    const base = { workspace: 'ws', automation: 'weather', hosts: 2, learning: false, learning_until: '2026-11-12T09:00:00Z', browses: false }
    expect(automationLine(base)).toBe('weather: 2 usual hosts; a new one raises an alert')
    expect(automationLine({ ...base, browses: true })).toBe('weather: reaches many hosts, so a new one is only recorded')
    expect(automationLine({ ...base, learning: true })).toMatch(/^weather: learning its usual hosts until /)
  })
})
