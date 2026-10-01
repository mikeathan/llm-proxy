import { describe, expect, it } from 'vitest'
import casesRaw from '../fixtures/guardrailMerge.cases.json?raw'
import globalRaw from '../fixtures/guardrailMerge.global.json?raw'
import { fieldSource, mergeGuardrails, normalizeLayer, seedLayer } from '../../domain/guardrailLayers'
import type { AgentGuardrailsConfig } from '../../types/admin'

// The same fixture runs against the backend's MergeWith
// (backend/models/config_merge_contract_test.go), so this mirror cannot drift.
const cases = JSON.parse(casesRaw) as { name: string; workspace: AgentGuardrailsConfig | null; effective: AgentGuardrailsConfig }[]
const GLOBAL = JSON.parse(globalRaw) as AgentGuardrailsConfig
const clone = <T>(v: T): T => JSON.parse(JSON.stringify(v)) as T

describe('guardrail layers', () => {
  it.each(cases.map((c) => [c.name, c] as const))('merges like the backend: %s', (_, c) => {
    expect(mergeGuardrails(GLOBAL, c.workspace)).toEqual(c.effective)
  })

  it('seeds a new layer that changes nothing', () => {
    expect(mergeGuardrails(GLOBAL, seedLayer(GLOBAL))).toEqual(GLOBAL)
  })

  it('drops what a stored layer repeats from the global policy, keeping its meaning', () => {
    // What the old editor saved: a full copy of the global policy plus one edit.
    const stored = clone(GLOBAL)
    stored.terminal.allowed_commands = ['ls', 'cat', 'git']
    const layer = normalizeLayer(stored, GLOBAL)
    expect(layer.terminal.allowed_commands).toEqual(['git'])
    expect(layer.terminal.timeout_seconds).toBe(0)
    expect(layer.terminal.enabled).toBe(false)
    expect(mergeGuardrails(GLOBAL, layer)).toEqual(mergeGuardrails(GLOBAL, stored))
  })

  it('fills sections a stored layer lacks without overriding anything', () => {
    const layer = normalizeLayer({ terminal: { enabled: false, allowed_commands: ['git'] } } as unknown as AgentGuardrailsConfig, GLOBAL)
    expect(layer.filesystem.max_file_size_kb).toBe(0)
    expect(layer.communication.require_review).toBe(true)
    expect(mergeGuardrails(GLOBAL, layer).filesystem).toEqual(GLOBAL.filesystem)
  })

  it('labels each field as inherited, overridden or an exception', () => {
    const layer = seedLayer(GLOBAL)
    expect(fieldSource('terminal', 'allowed_commands', GLOBAL, null)).toBe('inherited')
    expect(fieldSource('terminal', 'allowed_commands', GLOBAL, layer)).toBe('inherited')
    layer.terminal.allowed_commands = ['git']
    layer.terminal.blocked_patterns = ['curl']
    layer.terminal.timeout_seconds = 90
    layer.global.block_secrets = true
    layer.network.allow_internet_access = false
    layer.search.enabled = true
    expect(fieldSource('terminal', 'allowed_commands', GLOBAL, layer)).toBe('exception')
    expect(fieldSource('terminal', 'blocked_patterns', GLOBAL, layer)).toBe('overridden')
    expect(fieldSource('terminal', 'timeout_seconds', GLOBAL, layer)).toBe('overridden')
    expect(fieldSource('global', 'block_secrets', GLOBAL, layer)).toBe('overridden')
    expect(fieldSource('network', 'allow_internet_access', GLOBAL, layer)).toBe('overridden')
    expect(fieldSource('search', 'enabled', GLOBAL, layer)).toBe('exception')
    // Turning off what the global policy turns on has no effect: still inherited.
    layer.filesystem.enabled = false
    expect(fieldSource('filesystem', 'enabled', GLOBAL, layer)).toBe('inherited')
  })
})
