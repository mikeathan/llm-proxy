import { ref, computed, watch } from "vue"
import cronstrue from "cronstrue"
import type { Model } from "../../types/model"
import type { ProviderItem } from "../../types/admin"
import type { Automation } from "../../types/dispatcher"

export interface AutomationFormData {
  name: string
  triggerType: string
  triggerValue: string
  taskFile: string
  strategy: string
  model: string
}

export function useAutomationForm(
  models: Model[],
  providers: Record<string, ProviderItem>,
  editAutomation: Automation | null,
  onFetchFiles: (ws: string) => void,
) {
  const selectedWorkspace = ref("")

  watch(selectedWorkspace, (newVal) => {
    if (newVal) {
      onFetchFiles(newVal)
    }
  })

  const form = ref<AutomationFormData>({
    name: "",
    triggerType: "cron",
    triggerValue: "",
    taskFile: "",
    strategy: "persistent",
    model: "",
  })

  const modelSource = ref<"local" | "cloud">("local")
  const selectedProviderKey = ref("")

  const resetForm = () => {
    form.value = {
      name: "",
      triggerType: "cron",
      triggerValue: "",
      taskFile: "",
      strategy: "persistent",
      model: "",
    }
    selectedWorkspace.value = ""
    selectedProviderKey.value = ""
    modelSource.value = "local"
  }

  const syncModelSource = () => {
    if (editAutomation?.model) {
      const modelName = editAutomation.model
      const modelObj = models.find((m) => m.name === modelName)
      if (modelObj) {
        let newKey = ""
        if (modelObj.provider === "local") {
          newKey = "local"
        } else {
          const keyName = modelObj.provider_config?.api_key_name || ""
          newKey = `${modelObj.provider}/${keyName}`
        }
        if (selectedProviderKey.value !== newKey) {
          selectedProviderKey.value = newKey
        }
      }
    }
  }

  watch(
    () => editAutomation?.id,
    () => {
      if (editAutomation) {
        selectedWorkspace.value = editAutomation.workspace
        syncModelSource()
        form.value = {
          name: editAutomation.name,
          triggerType: editAutomation.trigger || "cron",
          triggerValue: editAutomation.trigger_value || "",
          taskFile: editAutomation.task_file,
          strategy: editAutomation.strategy,
          model: editAutomation.model || "",
        }
      } else {
        resetForm()
      }
    },
    { immediate: true },
  )

  watch(
    () => models,
    () => {
      if (editAutomation) {
        syncModelSource()
      }
    },
    { deep: true },
  )

  const filteredModels = computed(() => {
    if (selectedProviderKey.value === "local") {
      return models.filter((m) => m.provider === "local")
    }

    if (!selectedProviderKey.value) return []
    const parts = selectedProviderKey.value.split("/")
    const provider = parts[0]
    const keyName = parts[1] || ""

    return models.filter(
      (m) =>
        m.provider === provider && (m.provider_config?.api_key_name || "") === keyName,
    )
  })

  const cloudProvidersWithKeys = computed(() => {
    const result: {
      providerName: string
      keys: { name: string; id: string; keyVal: string }[]
    }[] = []

    for (const [name, p] of Object.entries(providers)) {
      if (name === "local") continue

      const keys: { name: string; id: string; keyVal: string }[] = []

      if (p.api_keys && p.api_keys.length > 0) {
        p.api_keys.forEach((k) => {
          keys.push({ name: k.name, id: k.id, keyVal: k.name })
        })
      }

      if (keys.length === 0) continue

      result.push({
        providerName: name,
        keys: keys.map((k) => ({ name: k.name, id: k.id, keyVal: k.keyVal })),
      })
    }
    return result
  })

  watch(selectedProviderKey, (_, oldVal) => {
    if (oldVal !== undefined) {
      if (editAutomation && form.value.model) {
        const isStillValid = filteredModels.value.some((m) => m.name === form.value.model)
        if (isStillValid) return
      }

      if (filteredModels.value.length > 0 && filteredModels.value[0]) {
        form.value.model = filteredModels.value[0].name
      } else {
        form.value.model = ""
      }
    }
  })

  watch(selectedWorkspace, () => {
    if (!editAutomation) {
      form.value.taskFile = ""
    }
  })

  const cronType = ref("custom")
  const cronEvery = ref(1)
  const cronUnit = ref("hours")

  watch([cronType, cronEvery, cronUnit], () => {
    if (cronType.value === "custom") return

    if (cronType.value === "every") {
      if (cronUnit.value === "minutes") {
        form.value.triggerValue = `*/${cronEvery.value} * * * *`
      } else if (cronUnit.value === "hours") {
        form.value.triggerValue = `0 */${cronEvery.value} * * *`
      } else if (cronUnit.value === "days") {
        form.value.triggerValue = `0 0 */${cronEvery.value} * *`
      }
    }
  })

  watch(
    () => form.value.triggerType,
    (newVal, oldVal) => {
      if (oldVal !== undefined && !editAutomation) {
        form.value.triggerValue = ""
      }
      if (newVal === "cron") {
        cronType.value = "custom"
      }
    },
  )

  const cronDescription = ref("")
  watch(
    () => form.value.triggerValue,
    (newVal) => {
      if (form.value.triggerType === "cron" && newVal) {
        try {
          cronDescription.value = cronstrue.toString(newVal)
        } catch {
          cronDescription.value = "Invalid cron expression"
        }
      } else {
        cronDescription.value = ""
      }
    },
  )

  const handleSubmit = (): AutomationFormData | null => {
    if (!selectedWorkspace.value || !form.value.name) return null

    return {
      name: form.value.name,
      triggerType: form.value.triggerType,
      triggerValue: form.value.triggerValue,
      taskFile: form.value.taskFile,
      strategy: form.value.strategy,
      model: form.value.model,
    }
  }

  return {
    selectedWorkspace,
    form,
    modelSource,
    selectedProviderKey,
    filteredModels,
    cloudProvidersWithKeys,
    cronType,
    cronEvery,
    cronUnit,
    cronDescription,
    resetForm,
    syncModelSource,
    handleSubmit,
  }
}
