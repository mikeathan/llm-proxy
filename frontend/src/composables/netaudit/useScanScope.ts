import { computed, ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { useConfirm } from '../ui/useConfirm'
import { useNetauditReadiness } from './useNetauditReadiness'
import { errorMessage } from '../../utils/errors'
import { ACTION_ALLOW_UNCONFINED } from '../../constants/netaudit'
import type { ScanScope, ScanSuggestion, ScopeUpdate } from '../../types/netaudit'

const LOAD_FAILED = 'The allowed networks could not be read.'
const SAVE_FAILED = 'The change could not be saved.'

// One shared scope: the settings tab and the readiness banner (which can allow unconfined scans) both change it, and
// each sees the other's change.
const scope = ref<ScanScope | null>(null)
const loading = ref(false)
const saving = ref(false)
const loadError = ref('')
const saveError = ref('')

/**
 * The networks scans may look at. There is no draft: every action saves at once (the server refuses a bad entry
 * whole and names it), so what is shown is always what is saved.
 */
export function useScanScope() {
  const { confirm } = useConfirm()
  const readiness = useNetauditReadiness()

  const ranges = computed(() => scope.value?.ranges ?? [])
  const unusedSuggestions = computed<ScanSuggestion[]>(() => {
    const taken = new Set(ranges.value.map((r) => r.cidr))
    return (scope.value?.suggestions ?? []).filter((s) => !taken.has(s.cidr))
  })

  async function load(): Promise<void> {
    loading.value = true
    try {
      scope.value = await NetauditService.scope()
      loadError.value = ''
    } catch (err) {
      loadError.value = errorMessage(err, LOAD_FAILED)
    } finally {
      loading.value = false
    }
  }

  async function save(update: ScopeUpdate): Promise<boolean> {
    saving.value = true
    try {
      scope.value = await NetauditService.updateScope(update)
      saveError.value = ''
      await readiness.refresh()
      return true
    } catch (err) {
      saveError.value = errorMessage(err, SAVE_FAILED)
      return false
    } finally {
      saving.value = false
    }
  }

  /** How long a scan may spend on one device (0 = standard). */
  const setDeviceTimeout = (minutes: number) => save({ device_timeout_minutes: minutes })

  const entries = () => ranges.value.map((r) => r.cidr)

  /** Allows one more range, typed by hand or taken from a suggestion; the ranges already allowed stay. */
  const addRange = (text: string) => save({ entries: [...entries(), text.trim()] })

  const removeRange = (cidr: string) => save({ entries: entries().filter((c) => c !== cidr) })

  /** "Use my network": the first network this machine is attached to that is not allowed yet. */
  async function useMyNetwork(): Promise<boolean> {
    const first = unusedSuggestions.value[0]
    return first ? addRange(first.cidr) : false
  }

  async function setAllowUnconfined(on: boolean): Promise<boolean> {
    if (on) {
      const ok = await confirm({
        title: 'Allow scans without file confinement?',
        message: 'Nothing stops nmap from reading or writing outside its own folder. Only do this on a machine you are developing on.',
        type: 'error',
        confirmText: 'Allow',
      })
      if (!ok) return false
    }
    return save({ allow_unconfined: on })
  }

  /** Meets a readiness "confirm" fix; an action this page does not know is ignored. */
  async function confirmFix(action: string): Promise<boolean> {
    return action === ACTION_ALLOW_UNCONFINED ? setAllowUnconfined(true) : false
  }

  return { scope, ranges, unusedSuggestions, loading, saving, loadError, saveError, load, addRange, removeRange, useMyNetwork, setDeviceTimeout, setAllowUnconfined, confirmFix }
}
