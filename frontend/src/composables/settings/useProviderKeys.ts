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
  const timers = new Set<ReturnType<typeof setTimeout>>()

  async function load(provider: ProviderType) {
    try {
      keys.value = { ...keys.value, [provider]: await AdminApiService.fetchProviderKeys(provider) }
    } catch (e) {
      toast.error(`Could not load the ${provider} API keys: ${errorMessage(e)}`)
    }
  }

  async function replace(provider: ProviderType, save: () => Promise<APIKeyItem[]>, failure: string) {
    try {
      keys.value = { ...keys.value, [provider]: await save() }
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
      const timer = setTimeout(() => {
        timers.delete(timer)
        if (testStatus.value[provider]?.success === res.message) clearTest(provider)
      }, TEST_SUCCESS_MS)
      timers.add(timer)
    } catch (e) {
      testStatus.value = { ...testStatus.value, [provider]: { loading: false, error: errorMessage(e) } }
    }
  }

  function clearTest(provider: ProviderType) {
    testStatus.value = { ...testStatus.value, [provider]: { loading: false } }
  }

  onUnmounted(() => {
    timers.forEach(clearTimeout)
    timers.clear()
  })

  return { keys, testStatus, load, save, clearAll, test, clearTest }
}
