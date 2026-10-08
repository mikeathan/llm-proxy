import { ref, watch, type Ref } from 'vue'
import { DispatcherService } from '../../services/automation/dispatcherService'
import type { MemoryMode } from '../../types/automation'

const KNOWN_MODES: MemoryMode[] = ['', 'on', 'off']
const LOAD_FAILED = 'The workspace settings could not be read.'
const SAVE_FAILED = 'The setting could not be saved.'

// A workspace's assistant memory override. The config endpoint replaces the whole
// document, so a save reads it fresh, changes this one field and writes it back.
export function useAssistantMemory(workspaceId: Readonly<Ref<string>>) {
  const mode = ref<MemoryMode>('')
  const loading = ref(false)
  const saving = ref(false)
  const error = ref<string | null>(null)

  async function load() {
    loading.value = true
    error.value = null
    try {
      const config = await DispatcherService.getWorkspaceConfig(workspaceId.value)
      mode.value = KNOWN_MODES.includes(config?.assistant_memory) ? config.assistant_memory : ''
    } catch (err) {
      error.value = err instanceof Error ? err.message : LOAD_FAILED
    } finally {
      loading.value = false
    }
  }

  async function save(next: MemoryMode) {
    saving.value = true
    error.value = null
    try {
      const config = await DispatcherService.getWorkspaceConfig(workspaceId.value)
      await DispatcherService.updateWorkspaceConfig(workspaceId.value, { ...config, assistant_memory: next })
      mode.value = next
    } catch (err) {
      error.value = err instanceof Error ? err.message : SAVE_FAILED
    } finally {
      saving.value = false
    }
  }

  watch(workspaceId, load, { immediate: true })
  return { mode, loading, saving, error, save }
}
