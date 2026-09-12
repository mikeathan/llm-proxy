// Host sandboxing settings editor — the Security & Sandboxing page's form
// state, dirty tracking and persistence, kept out of the component. One source
// for the GET→draft→PUT mapping so load and save can never drift (both rebuild
// the draft through utils/sandboxing normalizeSandboxingConfig).
//
// The editable draft is the requested policy only (SandboxingConfig); the
// backend Effective projection is read-only runtime state held separately and
// never PUT back.
import { computed, ref } from 'vue'
import { AdminApiService } from '../../services/admin/adminService'
import type { SandboxEffective, SandboxingConfig } from '../../types/admin'
import { errorMessage } from '../../utils/errors'
import {
  defaultSandboxingConfig,
  isFilesystemOn,
  isNetworkAllowed,
  isNetworkDecided,
  normalizeSandboxingConfig,
} from '../../utils/sandboxing'

// Host-settings endpoint answers 404 on first boot before defaults are written;
// that is not an operator-facing failure.
const NOT_FOUND_IGNORED = 'Not Found'

export function useHostSandboxingEditor() {
  const loading = ref(false)
  const saving = ref(false)
  const loadError = ref('')
  const saveError = ref('')

  const draft = ref<SandboxingConfig>(defaultSandboxingConfig())
  const effective = ref<SandboxEffective | null>(null)
  // Stable snapshot of the last loaded/saved policy (normalizeSandboxingConfig
  // emits fixed key order, so stringify is deterministic for the dirty check).
  const originalKey = ref('')

  const draftKey = () => JSON.stringify(draft.value)
  const hasChanges = computed(() => draftKey() !== originalKey.value)

  const networkAllowed = computed(() => isNetworkAllowed(draft.value))
  const networkDecided = computed(() => isNetworkDecided(draft.value))
  const filesystemOn = computed(() => isFilesystemOn(draft.value))

  function adopt(settings: { sandboxing?: Partial<SandboxingConfig>; effective?: SandboxEffective | null }): void {
    draft.value = normalizeSandboxingConfig(settings.sandboxing)
    effective.value = settings.effective ?? null
    originalKey.value = draftKey()
  }

  async function load(): Promise<void> {
    loading.value = true
    loadError.value = ''
    try {
      adopt(await AdminApiService.fetchHostSettings())
    } catch (e) {
      const message = errorMessage(e)
      loadError.value = message === NOT_FOUND_IGNORED ? '' : message
      // Failed load must not present a phantom "unsaved changes" state.
      originalKey.value = draftKey()
    } finally {
      loading.value = false
    }
  }

  async function save(): Promise<boolean> {
    saving.value = true
    saveError.value = ''
    try {
      adopt(await AdminApiService.updateHostSettings({ sandboxing: normalizeSandboxingConfig(draft.value) }))
      return true
    } catch (e) {
      saveError.value = errorMessage(e)
      return false
    } finally {
      saving.value = false
    }
  }

  /** Apply a partial edit to the draft, keeping keys stable/ordered. */
  function patch(changes: Partial<SandboxingConfig>): void {
    draft.value = normalizeSandboxingConfig({ ...draft.value, ...changes })
  }

  return {
    loading,
    saving,
    loadError,
    saveError,
    draft,
    effective,
    hasChanges,
    networkAllowed,
    networkDecided,
    filesystemOn,
    load,
    save,
    patch,
  }
}
