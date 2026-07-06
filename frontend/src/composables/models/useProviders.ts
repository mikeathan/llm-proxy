import { ref, computed } from 'vue'
import { AdminApiService } from '../../services/admin/adminService'
import { PROVIDER_ICONS, PROVIDER_LABELS, PROVIDER_STYLES } from '../../constants/providers'
import type { ProviderType, SettingsTab } from '../../types/admin'

export interface ProviderManifest {
  id: string
  name: string
  default_base_url: string
  archetype: string
  icon?: string
}

const manifests = ref<ProviderManifest[]>([])
const isLoading = ref(false)

export function useProviders() {
  async function fetchManifests() {
    if (manifests.value.length > 0) return
    isLoading.value = true
    try {
      const data = await AdminApiService.fetchProviderManifests()
      manifests.value = data
    } catch (e) {
      console.error('Failed to fetch provider manifests:', e)
    } finally {
      isLoading.value = false
    }
  }

  const cloudProviders = computed(() => {
    // Merge static ones that might not be in the registry yet (like vertex/gemini if they stay as Go)
    // and dynamic ones from the registry.
    const dynamic = manifests.value.map(m => m.id as ProviderType)
    const staticProviders: ProviderType[] = ['gemini', 'vertex']
    return Array.from(new Set([...staticProviders, ...dynamic]))
  })

  const allProviders = computed(() => ['local' as ProviderType, ...cloudProviders.value])

  const settingsTabs = computed<SettingsTab[]>(() => {
    // We want to preserve the order from SETTINGS_TABS while ensuring 
    // any dynamically discovered cloud providers are also included.
    const base = ['local', 'local-models', 'guardrails', ...cloudProviders.value, 'mcp', 'communication', 'processes']
    return Array.from(new Set(base)) as SettingsTab[]
  })

  const getIcon = (type: string) => {
    const manifest = manifests.value.find(m => m.id === type)
    if (manifest?.icon) return manifest.icon
    return PROVIDER_ICONS[type as keyof typeof PROVIDER_ICONS] || '❓'
  }

  const getLabel = (type: string) => {
    const manifest = manifests.value.find(m => m.id === type)
    if (manifest?.name) return manifest.name
    return PROVIDER_LABELS[type as keyof typeof PROVIDER_LABELS] || type
  }

  const getStyle = (type: string) => {
    return PROVIDER_STYLES[type as keyof typeof PROVIDER_STYLES] || 'bg-gray-900/30 text-gray-400 border-gray-500/30'
  }

  return {
    manifests,
    isLoading,
    fetchManifests,
    cloudProviders,
    allProviders,
    settingsTabs,
    getIcon,
    getLabel,
    getStyle
  }
}
