import { ref, computed, watch, type Ref } from "vue"
import { useModels } from "../models/useModels"
import type { Model } from "../../types/model"
import type { ProviderItem } from "../../types/admin"
import type { Automation } from "../../types/dispatcher"

export type TriggerType = "cron" | "interval" | "manual"

export interface AutomationFormData {
  name: string
  triggerType: TriggerType
  triggerValue: string
  taskFile: string
  strategy: string
  model: string
}

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

  // ---- workspace ---------------------------------------------------------
  watch(selectedWorkspace, (ws) => {
    if (ws) onFetchFiles(ws)
    if (!editAutomation.value) form.value.taskFile = ""
  })

  // ---- populate / reset --------------------------------------------------
  watch(
    editAutomation,
    (target) => {
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
    handleSubmit,
    resetForm,
  }
}
