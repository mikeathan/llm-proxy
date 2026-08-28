import type { AdminState } from '../../types/admin'
import type { ModelBanner } from '../../types/model'
import { escapeHtml } from '../../utils/format/format'

// computeModelBanner derives the global persistent warning banner from admin
// state. Only the primary model drives banners:
//   - critical: neither primary nor fallback set (requests fail)
//   - notice:   primary unset but a fallback exists (using fallback)
//   - null:     primary set (or fallback alone, which is fine)
// A model is only "ok" if it is both set and present in the catalogue, so a
// dangling primary (cleared post-removal) is still treated as unset.
export function computeModelBanner(state: AdminState | null): ModelBanner | null {
  const cfg = state?.config
  if (!cfg) return null

  const names = new Set((state?.models ?? []).map((m) => m.name))
  const primaryOk = !!cfg.primary_model && names.has(cfg.primary_model)
  const fallbackOk = !!cfg.fallback_model && names.has(cfg.fallback_model)

  if (!primaryOk && !fallbackOk) {
    return {
      severity: 'critical',
      message:
        'No primary or fallback model set. Requests will fail. Open Settings -> Global to choose a model.',
      html: 'No primary or fallback model set. Requests will fail. Open <strong>Settings &rarr; Global</strong> to choose a model.',
      action: { label: 'Configure models', settingsTab: 'local' },
    }
  }
  if (!primaryOk && fallbackOk) {
    // cfg.fallback_model is an operator-chosen registry name. Escape it before
    // embedding in the html variant to keep the v-html boundary safe.
    const fb = escapeHtml(cfg.fallback_model ?? '')
    return {
      severity: 'notice',
      message: `Primary model not set — using fallback "${cfg.fallback_model}". Set a primary in Settings -> Global.`,
      html: `Primary model not set — using fallback <strong>"${fb}"</strong>. Set a primary in <strong>Settings &rarr; Global</strong>.`,
      action: { label: 'Review models', settingsTab: 'local' },
    }
  }
  return null
}
