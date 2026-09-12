import { describe, expect, it } from 'vitest'
import {
  defaultSandboxingConfig,
  ENFORCEMENT_NONE,
  formatSandboxSurface,
  isFilesystemOn,
  isHostNetworkOff,
  isNetworkAllowed,
  isNetworkDecided,
  normalizeSandboxingConfig,
  sandboxSurfaceShort,
  SURFACE_NOT_REPORTED,
  SURFACE_OFF,
} from '../../utils/sandboxing'
import type { SandboxingConfig, SandboxSurface } from '../../types/admin'

// Sandboxing draft semantics mirror the backend's per-key *bool behavior
// (plan D6): an absent key is undecided — filesystem resolves ON, network
// resolves ALLOWED-legacy — and the PUT must keep undecided keys absent.
describe('defaultSandboxingConfig', () => {
  it('provides every editable key with the backend defaults', () => {
    expect(defaultSandboxingConfig()).toEqual({
      enabled: true,
      functional: true,
      max_memory_mb: 2048,
      max_storage_gb: 2,
      egress_proxy: 0,
      egress_allow_domains: [],
      egress_deny_domains: [],
    })
  })

  it('leaves the *bool switches undecided (undefined, key omitted on PUT)', () => {
    const cfg = defaultSandboxingConfig()
    expect('filesystem' in cfg).toBe(false)
    expect('network' in cfg).toBe(false)
    expect(cfg.filesystem).toBeUndefined()
    expect(cfg.network).toBeUndefined()
  })
})

describe('normalizeSandboxingConfig', () => {
  it('fills defaults for missing keys', () => {
    const cfg = normalizeSandboxingConfig(undefined)
    expect(cfg.enabled).toBe(true)
    expect(cfg.max_storage_gb).toBe(2)
    expect(cfg.egress_allow_domains).toEqual([])
  })

  it('preserves explicit values, including false and undecided', () => {
    const raw: Partial<SandboxingConfig> = { enabled: false, network: false, filesystem: true, max_storage_gb: 5 }
    const cfg = normalizeSandboxingConfig(raw)
    expect(cfg.enabled).toBe(false)
    expect(cfg.network).toBe(false)
    expect(cfg.filesystem).toBe(true)
    expect(cfg.max_storage_gb).toBe(5)
  })

  it('keeps undecided *bool keys absent so a PUT never materializes them', () => {
    const cfg = normalizeSandboxingConfig({ network: undefined })
    expect('network' in cfg).toBe(false)
    expect('filesystem' in cfg).toBe(false)
  })

  it('emits a deterministic JSON key order regardless of input order', () => {
    const a = normalizeSandboxingConfig({ egress_proxy: 4002, network: false, enabled: true })
    const b = normalizeSandboxingConfig({ enabled: true, network: false, egress_proxy: 4002 })
    expect(JSON.stringify(a)).toBe(JSON.stringify(b))
  })

  it('round-trips a full canonical object unchanged', () => {
    const canonical = normalizeSandboxingConfig(defaultSandboxingConfig())
    expect(normalizeSandboxingConfig(canonical)).toEqual(canonical)
  })
})

describe('state predicates', () => {
  it('network is allowed when undecided or explicitly true', () => {
    expect(isNetworkAllowed({ network: undefined } as SandboxingConfig)).toBe(true)
    expect(isNetworkAllowed({ network: true } as SandboxingConfig)).toBe(true)
    expect(isNetworkAllowed({ network: false } as SandboxingConfig)).toBe(false)
  })

  it('network is decided only when the operator wrote a value', () => {
    expect(isNetworkDecided({ network: undefined } as SandboxingConfig)).toBe(false)
    expect(isNetworkDecided({ network: false } as SandboxingConfig)).toBe(true)
  })

  it('filesystem is on when undecided or explicitly true', () => {
    expect(isFilesystemOn({ filesystem: undefined } as SandboxingConfig)).toBe(true)
    expect(isFilesystemOn({ filesystem: true } as SandboxingConfig)).toBe(true)
    expect(isFilesystemOn({ filesystem: false } as SandboxingConfig)).toBe(false)
  })

  it('host network off is only an explicit false (the hard ceiling)', () => {
    expect(isHostNetworkOff({ network: undefined } as SandboxingConfig)).toBe(false)
    expect(isHostNetworkOff({ network: true } as SandboxingConfig)).toBe(false)
    expect(isHostNetworkOff({ network: false } as SandboxingConfig)).toBe(true)
  })
})

describe('Effective surface labels', () => {
  const landlock = { mechanism: 'landlock' }
  const downgrade = { mechanism: ENFORCEMENT_NONE, reason: 'kernel too old' }
  const silentOff = { mechanism: ENFORCEMENT_NONE }

  it('full format surfaces the reason so a downgrade is never silent', () => {
    expect(formatSandboxSurface(landlock)).toBe('landlock')
    expect(formatSandboxSurface(downgrade)).toBe('off — kernel too old')
    expect(formatSandboxSurface(silentOff)).toBe(SURFACE_OFF)
    expect(formatSandboxSurface(undefined)).toBe(SURFACE_NOT_REPORTED)
  })

  it('short format keeps the compact pill text', () => {
    expect(sandboxSurfaceShort(landlock as SandboxSurface)).toBe('landlock')
    expect(sandboxSurfaceShort(downgrade as SandboxSurface)).toBe(SURFACE_OFF)
    expect(sandboxSurfaceShort(undefined)).toBe(SURFACE_OFF)
  })
})
