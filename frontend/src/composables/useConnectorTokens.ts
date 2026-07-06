import { ref } from "vue"
import type { Ref } from "vue"
import { AdminApiService } from "../services/admin/adminService"

export function useConnectorTokens() {
  const tokens = ref<Record<string, { masked: string; dirty: string | null }>>({})

  async function load(name: string) {
    try {
      const masked = await AdminApiService.fetchToolSecret("connector", name)
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

  // Persists dirty tokens and masks them. Returns false on first API error —
  // the caller uses this to skip the emit when persistence fails.
  async function saveDirty(saveError: Ref<string>): Promise<boolean> {
    for (const [name, tok] of Object.entries(tokens.value)) {
      if (tok?.dirty) {
        try {
          await AdminApiService.saveToolSecret("connector", name, tok.dirty)
          tok.masked = tok.dirty.slice(0, 4) + "..."
          tok.dirty = null
        } catch (err) {
          saveError.value = `Failed to save token for "${name}": ${err}`
          return false
        }
      }
    }
    return true
  }

  return { tokens, load, ensureTracked, saveDirty }
}
