// Pure sandboxing-domain helpers shared by the host Security page and the
// automation form. All logic is deterministic and framework-free so it is unit-
// testable; types come from src/types (single export source).
import type { SandboxSurface, SandboxingConfig } from '../types/admin'

export const ENFORCEMENT_NONE = 'none'

// UI label for a surface with no mechanism reported by the backend.
export const SURFACE_NOT_REPORTED = 'not reported by backend'
// UI label for a surface the backend reports as not enforced without a reason.
export const SURFACE_OFF = 'off'
// Prefix joining "off" to the downgrade reason ("off — <reason>").
const SURFACE_OFF_REASON_SEPARATOR = 'off — '

// Backend defaults for fields an older/missing settings doc does not carry.
// filesystem/network are intentionally left undecided (absent) so the PUT omits
// them and the backend keeps its per-key semantics.
export function defaultSandboxingConfig(): SandboxingConfig {
  return {
    enabled: true,
    functional: true,
    max_memory_mb: 2048,
    max_storage_gb: 2,
    egress_proxy: 0,
    egress_allow_domains: [],
    egress_deny_domains: [],
  }
}

// normalizeSandboxingConfig rebuilds a config with every editable key present in
// a fixed order. That gives JSON.stringify a stable key, which is what the
// dirty-check relies on, and fills backend defaults. Undecided *bool keys stay
// ABSENT (deleted, not `undefined`) so a round-trip PUT never turns "undecided"
// into an explicit value and the JSON stays clean.
export function normalizeSandboxingConfig(raw: Partial<SandboxingConfig> | undefined): SandboxingConfig {
  const cfg: SandboxingConfig = {
    enabled: raw?.enabled ?? true,
    functional: raw?.functional ?? true,
    max_memory_mb: raw?.max_memory_mb ?? 2048,
    max_storage_gb: raw?.max_storage_gb ?? 2,
    egress_proxy: raw?.egress_proxy ?? 0,
    egress_allow_domains: raw?.egress_allow_domains ?? [],
    egress_deny_domains: raw?.egress_deny_domains ?? [],
  }
  if (raw?.filesystem !== undefined) cfg.filesystem = raw.filesystem
  if (raw?.network !== undefined) cfg.network = raw.network
  return cfg
}

// State predicates mirror backend semantics: an absent key is undecided —
// filesystem undecided resolves ON, network undecided resolves ALLOWED-legacy.

export function isNetworkAllowed(cfg: SandboxingConfig): boolean {
  return cfg.network === undefined || cfg.network === true
}

export function isNetworkDecided(cfg: SandboxingConfig): boolean {
  return cfg.network !== undefined
}

export function isFilesystemOn(cfg: SandboxingConfig): boolean {
  return cfg.filesystem === undefined || cfg.filesystem === true
}

// Host-network switch explicitly OFF (the hard ceiling). Automation grants are
// inert while this is true, regardless of any per-run grant.
export function isHostNetworkOff(cfg: SandboxingConfig): boolean {
  return cfg.network === false
}

// Enforcement labels from the backend Effective projection (SPEC-006 §II.7.4).
// formatSandboxSurface shows the full downgrade ("off — <reason>") for the
// Effective card; sandboxSurfaceShort shows a compact mechanism-or-off pill.

export function formatSandboxSurface(surface: SandboxSurface | undefined): string {
  if (!surface) return SURFACE_NOT_REPORTED
  if (surface.mechanism && surface.mechanism !== ENFORCEMENT_NONE) return surface.mechanism
  if (surface.reason) return `${SURFACE_OFF_REASON_SEPARATOR}${surface.reason}`
  return SURFACE_OFF
}

export function sandboxSurfaceShort(surface: SandboxSurface | undefined): string {
  if (surface?.mechanism && surface.mechanism !== ENFORCEMENT_NONE) return surface.mechanism
  return SURFACE_OFF
}
