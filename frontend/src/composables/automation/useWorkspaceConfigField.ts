import { ref, watch, type Ref } from 'vue'
import { DispatcherService } from '../../services/automation/dispatcherService'

const LOAD_FAILED = 'The workspace settings could not be read.'
const SAVE_FAILED = 'The setting could not be saved.'

/**
 * One string field of a workspace's config, loaded for the workspace in view and saved on its own. The config endpoint
 * replaces the whole document, so a save reads it fresh, changes this one field and writes it back — it never writes
 * a copy that went stale while the page was open.
 */
export function useWorkspaceConfigField<T extends string>(
  workspaceId: Readonly<Ref<string>>,
  field: string,
  normalize: (raw: unknown) => T,
  fallback: T,
) {
  const value = ref(fallback) as Ref<T>
  const loading = ref(false)
  const saving = ref(false)
  const error = ref<string | null>(null)

  async function load() {
    loading.value = true
    error.value = null
    try {
      const config = await DispatcherService.getWorkspaceConfig(workspaceId.value)
      value.value = normalize(config?.[field])
    } catch (err) {
      error.value = err instanceof Error ? err.message : LOAD_FAILED
    } finally {
      loading.value = false
    }
  }

  async function save(next: T) {
    saving.value = true
    error.value = null
    try {
      const config = await DispatcherService.getWorkspaceConfig(workspaceId.value)
      await DispatcherService.updateWorkspaceConfig(workspaceId.value, { ...config, [field]: next })
      value.value = next
    } catch (err) {
      error.value = err instanceof Error ? err.message : SAVE_FAILED
    } finally {
      saving.value = false
    }
  }

  watch(workspaceId, load, { immediate: true })
  return { value, loading, saving, error, save }
}
