import { ref } from 'vue'
import { AdminApiService } from '../services/adminService'
import type { GlobalConfig } from '../types/admin'

const DEFAULT_CONFIG: GlobalConfig = {
  providers: {
    local: {
      type: 'local',
      model_dir: '',
      llama_server_binary: '',
      default_args: [],
      environment: {},
    },
  },
  model_host: '',
  idle_timeout_seconds: 1800,
}

const editConfig = ref<GlobalConfig>({ ...DEFAULT_CONFIG })
const isSaving = ref(false)
let seeded = false

const seedFromState = (config: GlobalConfig): void => {
  if (!seeded) {
    const raw = JSON.parse(JSON.stringify(config))
    // Ensure structure
    if (!raw.providers) raw.providers = {}
    if (!raw.providers.local) {
      raw.providers.local = {
        type: 'local',
        model_dir: '',
        llama_server_binary: '',
        default_args: [],
        environment: {},
      }
    }
    editConfig.value = raw
    seeded = true
  }
}

const updateConfig = async (onSaved?: () => void): Promise<void> => {
  isSaving.value = true
  try {
    await AdminApiService.updateConfig(editConfig.value)
    alert('Configuration saved')
    onSaved?.()
  } catch (e: any) {
    console.error(e)
    alert(`Error saving configuration: ${e.message}`)
  } finally {
    isSaving.value = false
  }
}

export function useConfig(onSaved?: () => void) {
  return {
    editConfig,
    isSaving,
    seedFromState,
    updateConfig: () => updateConfig(onSaved),
  }
}
