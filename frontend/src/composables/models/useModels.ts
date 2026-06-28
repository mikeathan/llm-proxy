import { ref, computed } from 'vue'
import { AdminApiService } from '../../services/adminService'
import { useToast } from '../useToast'
import type { AdminState, AgentDefaults } from '../../types/admin'

const { error: toastError, success: toastSuccess } = useToast()


const state = ref<AdminState | null>(null)

const activeModel = computed(() => state.value?.active)
const availableModels = computed(() => state.value?.available ?? [])
const models = computed(() => state.value?.models ?? [])
const nextPort = computed(() => state.value?.next_port ?? 9000)
const agentDefaults = computed<AgentDefaults>(() => state.value?.config?.agent_defaults ?? {
  max_steps: 25,
  context_budget: 8000,
  max_tokens: 3072,
  temperature: 0.1,
  reasoning_budget: 0,
  timeout_minutes: 30,
  tool_call_format: '',
  prefill: false,
})

const refresh = async (): Promise<void> => {
  try {
    state.value = await AdminApiService.fetchState()
  } catch (e: any) {
    console.error('[useModels] fetch state failed:', e.message)
  }
}

const startModel = async (name: string): Promise<void> => {
  try {
    await AdminApiService.startModel(name)
    toastSuccess(`Model ${name} started`)
    await refresh()
  } catch (e: any) {
    console.error(e)
    toastError(`Error starting model: ${e.message}`)
  }
}

const stopModel = async (): Promise<void> => {
  try {
    await AdminApiService.stopModel()
    toastSuccess("Model stopped")
    await refresh()
  } catch (e: any) {
    console.error(e)
    toastError(`Error stopping model: ${e.message}`)
  }
}

const addModel = async (payload: any): Promise<void> => {
  try {
    await AdminApiService.addModel(payload)
    toastSuccess(`Model ${payload.name} added`)
    await refresh()
  } catch (e: any) {
    console.error(e)
    toastError(`Error adding model: ${e.message}`)
  }
}

const updateModel = async (payload: any): Promise<void> => {
  try {
    await AdminApiService.updateModel(payload)
    toastSuccess(`Model ${payload.name} updated`)
    await refresh()
  } catch (e: any) {
    console.error(e)
    toastError(`Error updating model: ${e.message}`)
  }
}

const removeModel = async (name: string): Promise<void> => {
  try {
    await AdminApiService.removeModel(name)
    toastSuccess(`Model ${name} removed`)
    await refresh()
  } catch (e: any) {
    console.error(e)
    toastError(`Error removing model: ${e.message}`)
  }
}

const removeAllModels = async (provider: string): Promise<void> => {
  try {
    await AdminApiService.removeAllModels(provider)
    toastSuccess(`All ${provider} models removed`)
    await refresh()
  } catch (e: any) {
    console.error(e)
    toastError(`Error removing all models: ${e.message}`)
  }
}

const fetchProviderModels = async (provider: string, apiKeyName?: string): Promise<import('../../types/model').ProviderModelInfo[]> => {
  try {
    return await AdminApiService.fetchProviderModels(provider, apiKeyName) || []
  } catch (e: any) {
    const isConfigError = e.message?.includes('not configured') || e.message?.includes('401')
    if (!isConfigError) {
      console.error(`[useModels] fetch provider models failed for ${provider}:`, e.message)
    }
    return []
  }
}

export function useModels() {
  return {
    state,
    models,
    activeModel,
    availableModels,
    nextPort,
    agentDefaults,
    refresh,
    startModel,
    stopModel,
    addModel,
    updateModel,
    removeModel,
    removeAllModels,
    fetchProviderModels,
  }
}
