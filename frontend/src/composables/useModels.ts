import { ref, computed } from 'vue'
import { AdminApiService } from '../services/adminService'
import type { AdminState } from '../types/admin'

const state = ref<AdminState | null>(null)

const activeModel = computed(() => state.value?.active)
const availableModels = computed(() => state.value?.available ?? [])
const models = computed(() => state.value?.models ?? [])
const nextPort = computed(() => state.value?.next_port ?? 9000)

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
    await refresh()
  } catch (e: any) {
    console.error(e)
    alert(`Error starting model: ${e.message}`)
  }
}

const stopModel = async (): Promise<void> => {
  try {
    await AdminApiService.stopModel()
    await refresh()
  } catch (e: any) {
    console.error(e)
    alert(`Error stopping model: ${e.message}`)
  }
}

const addModel = async (payload: any): Promise<void> => {
  try {
    await AdminApiService.addModel(payload)
    await refresh()
  } catch (e: any) {
    console.error(e)
    alert(`Error adding model: ${e.message}`)
  }
}

const updateModel = async (payload: any): Promise<void> => {
  try {
    await AdminApiService.updateModel(payload)
    await refresh()
  } catch (e: any) {
    console.error(e)
    alert(`Error updating model: ${e.message}`)
  }
}

const removeModel = async (name: string): Promise<void> => {
  if (!confirm(`Remove model "${name}"?`)) return
  try {
    await AdminApiService.removeModel(name)
    await refresh()
  } catch (e: any) {
    console.error(e)
    alert(`Error removing model: ${e.message}`)
  }
}

const fetchProviderModels = async (provider: string): Promise<string[]> => {
  try {
    return await AdminApiService.fetchProviderModels(provider)
  } catch (e: any) {
    console.error(`[useModels] fetch provider models failed for ${provider}:`, e.message)
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
    refresh,
    startModel,
    stopModel,
    addModel,
    updateModel,
    removeModel,
    fetchProviderModels,
  }
}
