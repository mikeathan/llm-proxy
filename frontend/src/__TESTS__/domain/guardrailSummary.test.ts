import { describe, expect, it } from 'vitest'
import { DEFAULT_CONFIG } from '../../composables/models/useConfig'
import { guardrailSectionSummary } from '../../domain/guardrailSummary'
import type { AgentGuardrailsConfig } from '../../types/admin'

const policy = (patch: Partial<{ [K in keyof AgentGuardrailsConfig]: Partial<AgentGuardrailsConfig[K]> }>): AgentGuardrailsConfig => {
  const base = structuredClone(DEFAULT_CONFIG.guardrails) as unknown as Record<string, Record<string, unknown>>
  for (const [section, fields] of Object.entries(patch)) base[section] = { ...base[section], ...fields }
  return base as unknown as AgentGuardrailsConfig
}

describe('guardrailSectionSummary', () => {
  it('says a switched-off tool is off, whatever else is configured', () => {
    const cfg = policy({ terminal: { enabled: false, allowed_commands: ['ls'] } })
    for (const section of ['terminal', 'filesystem', 'communication', 'network'] as const) {
      const off = policy({ [section]: { enabled: false } })
      expect(guardrailSectionSummary(section, off)).toBe('Off')
    }
    expect(guardrailSectionSummary('terminal', cfg)).toBe('Off')
  })

  it('counts what the terminal allows and its timeout', () => {
    const cfg = policy({ terminal: { enabled: true, allowed_commands: ['ls', 'git'], timeout_seconds: 60 } })
    expect(guardrailSectionSummary('terminal', cfg)).toBe('2 allowed commands · timeout 60s')
    const one = policy({ terminal: { enabled: true, allowed_commands: ['ls'], timeout_seconds: 0 } })
    expect(guardrailSectionSummary('terminal', one)).toBe('1 allowed command')
  })

  it('reports secrets blocking and the blocked patterns', () => {
    const on = policy({ global: { block_secrets: true, user_blocked_patterns: ['a', 'b'] } })
    expect(guardrailSectionSummary('global', on)).toBe('Blocking secrets · 2 blocked patterns')
    const off = policy({ global: { block_secrets: false, user_blocked_patterns: [] } })
    expect(guardrailSectionSummary('global', off)).toBe('Not blocking secrets')
  })

  it('reports file access mode and blocked names', () => {
    const rw = policy({ filesystem: { enabled: true, read_only: false, blocked_filenames: ['.env'] } })
    expect(guardrailSectionSummary('filesystem', rw)).toBe('Read and write · 1 blocked file name')
    const ro = policy({ filesystem: { enabled: true, read_only: true, blocked_filenames: [] } })
    expect(guardrailSectionSummary('filesystem', ro)).toBe('Read-only')
  })

  it('reports whether notifications need approval', () => {
    expect(guardrailSectionSummary('communication', policy({ communication: { enabled: true, require_review: true } }))).toBe('Each notification needs approval')
    expect(guardrailSectionSummary('communication', policy({ communication: { enabled: true, require_review: false } }))).toBe('Notifications sent without approval')
  })

  it('reports network reach and blocked domains', () => {
    const cfg = policy({ network: { enabled: true, allow_lan_access: false, allow_internet_access: true, blocked_domains: ['a.com', 'b.com'] } })
    expect(guardrailSectionSummary('network', cfg)).toBe('Internet only (no local network) · 2 blocked domains')
  })
})
