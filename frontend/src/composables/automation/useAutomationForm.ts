import { ref, computed, watch, type Ref } from "vue"
import { useModels } from "../models/useModels"
import type { Model } from "../../types/model"
import type { ProviderItem } from "../../types/admin"
import type { Automation } from "../../types/dispatcher"
import type { AutomationFormData, TriggerType } from "../../types/automation"
import type { ChoiceOption } from "../../types/ui"
import { loopStrategyOptions as buildLoopStrategyOptions } from "../../utils/model/modelUtils"

export function useAutomationForm(
  editAutomation: Ref<Automation | null>,
  onFetchFiles: (workspace: string) => void,
) {
  // App-wide admin store singleton: live computeds, so a late adminState load
  // just recomputes the derivations below. No watches.
  const { state } = useModels()
  const models = computed<Model[]>(() => state.value?.models ?? [])
  const providers = computed<Record<string, ProviderItem>>(
    () => state.value?.config.providers ?? {},
  )

  const selectedWorkspace = ref("")

  function emptyForm(): AutomationFormData {
    return {
      name: "",
      triggerType: "cron",
      triggerValue: "",
      taskFile: "",
      strategy: "persistent",
      model: "",
      loopStrategy: "",
      networkGrant: "",
      memoryMode: "",
      notifyConnector: "",
      notifyDedup: false,
      notifyDedupDays: "",
      notifySendEmpty: false,
      skipIfBusy: false,
    }
  }

  const form = ref<AutomationFormData>(emptyForm())

  // ---- derived model routing ---------------------------------------------
  function modelsForKey(key: string): Model[] {
    if (key === "local") return models.value.filter((m) => m.provider === "local")
    if (!key) return []
    const [provider, keyName] = key.split("/")
    return models.value.filter(
      (m) =>
        m.provider === provider &&
        (m.provider_config?.api_key_name || "") === (keyName || ""),
    )
  }

  const selectedProviderKey = computed({
    get: () => {
      const model = models.value.find((m) => m.name === form.value.model)
      if (!model) return ""
      return model.provider === "local"
        ? "local"
        : `${model.provider}/${model.provider_config?.api_key_name || ""}`
    },
    set: (key: string) => {
      form.value.model = modelsForKey(key)[0]?.name ?? ""
    },
  })

  const filteredModels = computed(() => modelsForKey(selectedProviderKey.value))

  // Backend-driven loop-strategy option list (config.loop_strategy_options
  // from the strategy registry). Falls back to the known values when the
  // backend hasn't surfaced a list.
  const loopStrategyOptions = computed(() =>
    buildLoopStrategyOptions(state.value?.config?.loop_strategy_options),
  )

  const cloudProvidersWithKeys = computed(() => {
    const result: {
      providerName: string
      keys: { name: string; id: string; keyVal: string }[]
    }[] = []

    for (const [name, p] of Object.entries(providers.value)) {
      if (name === "local") continue

      const keys = (p.api_keys ?? []).map((k) => ({
        name: k.name,
        id: k.id,
        keyVal: k.name,
      }))

      if (keys.length === 0) continue

      result.push({ providerName: name, keys })
    }
    return result
  })

  // Connectors an automation can deliver through, from the shared config. The
  // connector already on the form stays selectable even if it was since
  // removed, so editing never silently drops it.
  const connectorOptions = computed<ChoiceOption[]>(() => {
    const configured = state.value?.config?.communication?.connectors ?? {}
    const options = Object.entries(configured).map(([name, c]) => ({
      value: name,
      label: c.enabled ? name : `${name} (disabled)`,
    }))
    const current = form.value.notifyConnector
    if (current && !(current in configured)) {
      options.push({ value: current, label: `${current} (not configured)` })
    }
    return options
  })
  // True only once the admin state has loaded and it lists no connector, so the
  // "add one" note never flashes while the state is still loading.
  const noConnectors = computed(
    () => !!state.value && Object.keys(state.value.config?.communication?.connectors ?? {}).length === 0,
  )

  // ---- workspace ---------------------------------------------------------
  watch(selectedWorkspace, (ws) => {
    if (ws) onFetchFiles(ws)
    if (!editAutomation.value) form.value.taskFile = ""
  })

  // ---- populate / reset --------------------------------------------------
  // Keyed on identity: a refreshed copy of the same automation (the owner polls)
  // must not overwrite the user's unsaved edits.
  watch(
    () => editAutomation.value?.id,
    () => {
      const target = editAutomation.value
      if (!target) {
        resetForm()
        return
      }
      selectedWorkspace.value = target.workspace
      form.value = {
        name: target.name,
        triggerType: (target.trigger as TriggerType) || "cron",
        triggerValue: target.trigger_value || "",
        taskFile: target.task_file,
        strategy: target.strategy,
        model: target.model || "",
        loopStrategy: target.loop_strategy || "",
        networkGrant: target.network_grant || "",
        memoryMode: target.memory_mode === "hot" ? "hot" : "", // "off" and unset are the same choice
        notifyConnector: target.notify?.connector ?? "",
        notifyDedup: !!target.notify?.dedup,
        notifyDedupDays: target.notify?.dedup_days ? String(target.notify.dedup_days) : "",
        notifySendEmpty: !!target.notify?.send_empty,
        skipIfBusy: !!target.skip_if_busy,
      }
    },
    { immediate: true },
  )

  function resetForm() {
    form.value = emptyForm()
    selectedWorkspace.value = ""
  }

  // ---- trigger behaviour -------------------------------------------------
  watch(
    () => form.value.triggerType,
    (_newVal, oldVal) => {
      if (oldVal !== undefined && !editAutomation.value) form.value.triggerValue = ""
    },
  )

  const handleSubmit = (): AutomationFormData | null => {
    if (!selectedWorkspace.value || !form.value.name) return null

    return { ...form.value }
  }

  return {
    selectedWorkspace,
    form,
    selectedProviderKey,
    filteredModels,
    cloudProvidersWithKeys,
    loopStrategyOptions,
    connectorOptions,
    noConnectors,
    handleSubmit,
    resetForm,
  }
}
