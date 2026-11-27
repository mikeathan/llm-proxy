import { onUnmounted, ref } from "vue"
import { AdminApiService } from "../../services/admin/adminService"
import { useToast } from "../useToast"
import { errorMessage } from "../../utils/errors"
import type { APIKeyItem, ProviderKeyTest, ProviderTestStatus, ProviderType } from "../../types/admin"

// A successful connection test is shown this long, then cleared.
const TEST_SUCCESS_MS = 5000

/**
 * The API keys of each cloud provider section in Settings. Keys are stored
 * encrypted by the backend and saved as soon as they change (they are not part
 * of the settings form); the server answers with masked values, which replace
 * the local list. `onKeysChanged` runs after every change, since models can
 * reference (and be removed with) a key.
 */
export function useProviderKeys(onKeysChanged: () => Promise<void>, baseUrlOf: (provider: ProviderType) => string | undefined) {
  const toast = useToast()
  const keys = ref<Partial<Record<ProviderType, APIKeyItem[]>>>({})
  const testStatus = ref<Partial<Record<ProviderType, ProviderTestStatus>>>({})
  // Whether the last test of a provider's key failed: outlives testStatus, which an open editor clears, so the rail can show a warning.
  const failed = ref<Partial<Record<ProviderType, boolean>>>({})
  const timers = new Set<ReturnType<typeof setTimeout>>()

  // The answer is awaited before the record is copied: several providers load at once (the Models page), and copying first let the
  // last answer overwrite the others with the empty record it started from.
  async function load(provider: ProviderType) {
    try {
      const loaded = await AdminApiService.fetchProviderKeys(provider)
      keys.value = { ...keys.value, [provider]: loaded }
    } catch (e) {
      toast.error(`Could not load the ${provider} API keys: ${errorMessage(e)}`)
    }
  }

  async function replace(provider: ProviderType, save: () => Promise<APIKeyItem[]>, failure: string) {
    try {
      const saved = await save()
      keys.value = { ...keys.value, [provider]: saved }
      failed.value = { ...failed.value, [provider]: false }
      await onKeysChanged()
    } catch (e) {
      toast.error(`${failure}: ${errorMessage(e)}`)
    }
  }

  const save = (provider: ProviderType, next: APIKeyItem[]) =>
    replace(provider, () => AdminApiService.saveProviderKeys(provider, next), "Could not save the API keys")

  const clearAll = (provider: ProviderType) =>
    replace(provider, () => AdminApiService.deleteAllProviderKeys(provider), "Could not remove the API keys")

  async function test(provider: ProviderType, payload: ProviderKeyTest) {
    testStatus.value = { ...testStatus.value, [provider]: { loading: true } }
    try {
      const res = await AdminApiService.testConnection(provider, payload.key, payload.id, payload.base_url || baseUrlOf(provider))
      testStatus.value = { ...testStatus.value, [provider]: { loading: false, success: res.message } }
      failed.value = { ...failed.value, [provider]: false }
      const timer = setTimeout(() => {
        timers.delete(timer)
        if (testStatus.value[provider]?.success === res.message) clearTest(provider)
      }, TEST_SUCCESS_MS)
      timers.add(timer)
    } catch (e) {
      testStatus.value = { ...testStatus.value, [provider]: { loading: false, error: errorMessage(e) } }
      failed.value = { ...failed.value, [provider]: true }
    }
  }

  function clearTest(provider: ProviderType) {
    testStatus.value = { ...testStatus.value, [provider]: { loading: false } }
  }

  onUnmounted(() => {
    timers.forEach(clearTimeout)
    timers.clear()
  })

  return { keys, testStatus, failed, load, save, clearAll, test, clearTest }
}
