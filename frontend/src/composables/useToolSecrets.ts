import { ref } from "vue"
import type { Ref } from "vue"
import { AdminApiService } from "../services/admin/adminService"

// useToolSecrets manages the masked/dirty secret state for one tool-secret
// category ("connector", "search", ...). The category is fixed per consumer;
// every named entry in that category is tracked independently.
export function useToolSecrets(category: string) {
  const tokens = ref<Record<string, { masked: string; dirty: string | null }>>({})

  async function load(name: string) {
    try {
      const masked = await AdminApiService.fetchToolSecret(category, name)
      tokens.value[name] = { masked, dirty: null }
    } catch {
      tokens.value[name] = { masked: "", dirty: null }
    }
  }

  function ensureTracked(name: string) {
    if (!tokens.value[name]) {
      tokens.value[name] = { masked: "", dirty: null }
    }
  }

  // Persists dirty tokens and records the server's authoritative mask. Returns
  // false on first API error — the caller uses this to skip the emit when
  // persistence fails.
  async function saveDirty(saveError: Ref<string>): Promise<boolean> {
    for (const [name, tok] of Object.entries(tokens.value)) {
      if (tok?.dirty) {
        try {
          tok.masked = await AdminApiService.saveToolSecret(category, name, tok.dirty)
          tok.dirty = null
        } catch (err) {
          saveError.value = `Failed to save token for "${name}": ${err}`
          return false
        }
      }
    }
    return true
  }

  // Clears a tracked token server-side and locally. Any pending edit is
  // discarded, so a queued dirty value cannot resurrect a deleted secret on the
  // next save. Returns an error message, or "" on success.
  async function clear(name: string): Promise<string> {
    try {
      const masked = await AdminApiService.deleteToolSecret(category, name)
      tokens.value[name] = { masked, dirty: null }
      return ""
    } catch (err) {
      return `Failed to clear token for "${name}": ${err}`
    }
  }

  return { tokens, load, ensureTracked, saveDirty, clear }
}
