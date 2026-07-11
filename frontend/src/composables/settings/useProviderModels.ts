import { ref, computed, watch, onMounted } from 'vue'
import { useModels } from '../models/useModels'
import { useConfirm } from '../ui/useConfirm'
import {
  getDefaultModelSettings,
  deriveModelName,
  createEmptyModelForm,
  computeDefaultsFromContext,
} from '../../utils/model/modelUtils'
import type { ModelForm } from '../../utils/model/modelUtils'
import type { APIKeyItem, ProviderType } from '../../types/admin'
import type { AvailableModel, Model, ProviderModelInfo } from '../../types/model'

export function useProviderModels(
  props: {
    provider: ProviderType
    apiKeys: APIKeyItem[]
    models: Model[]
    availableModels?: AvailableModel[]
  },
  emit: (event: 'refresh') => void,
) {
  const {
    state,
    addModel,
    updateModel,
    removeModel,
    removeAllModels,
    fetchProviderModels,
  } = useModels()
  const { confirm } = useConfirm()

  const agentDefaults = computed(() => {
    const pd = state.value?.config?.provider_defaults?.[props.provider]
    if (pd) return pd
    return state.value?.config?.agent_defaults ?? {
      max_steps: 25,
      context_budget: 8000,
      max_tokens: 3072,
      temperature: 0.1,
      reasoning_budget: 0,
      timeout_minutes: 30,
      tool_call_format: '',
      prefill: false,
    }
  })

  const providerModels = ref<ProviderModelInfo[]>([])
  const isLoadingModels = ref(false)
  const editingModel = ref<Partial<Model> | null>(null)
  const isAddingNew = ref(false)
  const modelForm = ref<ModelForm>(createEmptyModelForm(props.provider, props.models, agentDefaults.value))
  const filterText = ref('')
  const lastDerivedName = ref('')

  const filteredProviderModels = computed(() => {
    if (!filterText.value) return providerModels.value
    const q = filterText.value.toLowerCase()
    return providerModels.value.filter((m) => m.id.toLowerCase().includes(q))
  })

  const groupsByKey = computed(() => {
    const groups: { keyName: string; models: Model[] }[] = []
    if (props.provider === 'local') {
      const localModels = props.models.filter(m => m.provider === 'local')
      if (localModels.length > 0) {
        groups.push({ keyName: 'Local Models', models: localModels })
      }
      return groups
    }
    for (const key of props.apiKeys) {
      groups.push({
        keyName: key.name,
        models: props.models.filter(m => m.provider === props.provider && m.provider_config?.api_key_name === key.name),
      })
    }
    const noKeyModels = props.models.filter(m => {
      if (m.provider !== props.provider) return false
      const keyName = m.provider_config?.api_key_name
      return !keyName || !props.apiKeys.some(k => k.name === keyName)
    })
    if (noKeyModels.length > 0) {
      groups.push({ keyName: '', models: noKeyModels })
    }
    return groups
  })

  watch(
    () => props.apiKeys,
    () => {
      if (modelForm.value.key) {
        const stillExists = props.apiKeys.some(
          (k) => k.id === modelForm.value.key || k.name === modelForm.value.key,
        )
        if (!stillExists) modelForm.value.key = ''
      }
    },
    { deep: true },
  )

  watch(() => modelForm.value.id, (id) => {
    if (!id || !isAddingNew.value) return
    const derived = deriveModelName(id)
    if (!modelForm.value.name || modelForm.value.name === lastDerivedName.value) {
      modelForm.value.name = derived
      lastDerivedName.value = derived
    }
    const selected = providerModels.value.find(m => m.id === id)
    const ctx = selected?.meta?.n_ctx || selected?.meta?.n_ctx_train || selected?.limits?.context
    const defaults = computeDefaultsFromContext(ctx)
    if (defaults) {
      modelForm.value.context_budget = defaults.context_budget
      modelForm.value.max_tokens = defaults.max_tokens
    }
  })

  watch(() => modelForm.value.key, (keyName) => {
    if (isAddingNew.value && props.provider !== 'local') {
      loadModels(keyName)
    }
  })

  let loadModelsReqId = 0

  async function loadModels(apiKeyName?: string) {
    if (props.provider === 'local') return
    const mine = ++loadModelsReqId
    isLoadingModels.value = true
    providerModels.value = []
    filterText.value = ''
    try {
      const list = await fetchProviderModels(props.provider, apiKeyName || modelForm.value.key)
      if (mine !== loadModelsReqId) return
      providerModels.value = list
    } finally {
      if (mine === loadModelsReqId) isLoadingModels.value = false
    }
  }

  function startAdd() {
    const defaults = getDefaultModelSettings(props.provider, agentDefaults.value)
    modelForm.value = createEmptyModelForm(props.provider, props.models, agentDefaults.value)
    editingModel.value = {
      name: '',
      provider: props.provider,
      filename: '',
      model_id: '',
      args: [],
      prefill: defaults.prefill,
      provider_config: { api_key_name: '' },
    }
    lastDerivedName.value = ''
    filterText.value = ''
    isAddingNew.value = true
    if (props.provider !== 'local') {
      loadModels()
    }
  }

  function scanAndAdd(keyName: string) {
    startAdd()
    modelForm.value.key = keyName
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  function cancelEdit() {
    editingModel.value = null
    isAddingNew.value = false
  }

  async function saveNewModel() {
    const { name, key, id, filename, port, args, ...tuning } = modelForm.value
    const finalName = name || deriveModelName(id, filename)
    if (props.provider === 'local') {
      if (!filename) return
      await addModel({
        name: finalName,
        provider: 'local',
        filename,
        port,
        args: args ? args.split(/\s+/).filter(Boolean) : [],
        ...tuning,
      })
    } else {
      if (!id || !modelForm.value.key) return
      const selected = providerModels.value.find(m => m.id === id)
      await addModel({
        name: finalName,
        provider: props.provider,
        model_id: id,
        provider_config: { api_key_name: key },
        ...tuning,
        ...(selected?.pricing ? { pricing: selected.pricing } : {}),
        ...(selected?.limits ? { limits: selected.limits } : {}),
        ...(selected?.meta ? { meta: selected.meta } : {}),
      })
    }
    cancelEdit()
    emit('refresh')
  }

  const alreadyConfiguredFilenames = computed(() => {
    if (props.provider !== 'local') return new Set<string>()
    return new Set(
      props.models
        .filter((m) => m.provider === 'local')
        .map((m) => m.filename)
        .filter(Boolean) as string[],
    )
  })

  onMounted(() => {
    if (props.provider === 'local') {
      emit('refresh')
    }
  })

  function addDiscoveredModel(m: AvailableModel) {
    if (alreadyConfiguredFilenames.value.has(m.filename)) return
    isAddingNew.value = true
    const name = m.metadata?.name || m.name
    modelForm.value = createEmptyModelForm('local', props.models, agentDefaults.value)
    modelForm.value.name = name
    modelForm.value.filename = m.filename
    editingModel.value = {
      name,
      provider: 'local',
      filename: m.filename,
      args: [],
      prefill: modelForm.value.prefill,
      provider_config: { api_key_name: '' },
    }
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  async function handleClearAll() {
    const confirmed = await confirm({
      title: 'Clear All Models',
      message: `Are you sure you want to remove ALL models for ${props.provider}? This cannot be undone.`,
      type: 'error',
      confirmText: 'Clear All',
      cancelText: 'Cancel',
    })
    if (!confirmed) return
    await removeAllModels(props.provider)
    emit('refresh')
  }

  const editingArgsStr = computed({
    get: () => (editingModel.value?.args || []).join(' '),
    set: (val: string) => {
      if (editingModel.value) {
        editingModel.value.args = val.split(/\s+/).filter(Boolean)
      }
    },
  })

  function handleEdit(model: Model) {
    editingModel.value = JSON.parse(JSON.stringify(model))
    isAddingNew.value = false
  }

  async function saveEdit() {
    if (!editingModel.value?.name) return
    await updateModel(editingModel.value)
    editingModel.value = null
    emit('refresh')
  }

  async function handleRemove(name: string) {
    const confirmed = await confirm({
      title: 'Remove Model',
      message: `Remove model "${name}"?`,
      type: 'error',
      confirmText: 'Remove',
      cancelText: 'Cancel',
    })
    if (!confirmed) return
    await removeModel(name)
    emit('refresh')
  }

  const isSubmitDisabled = computed(() => {
    if (isAddingNew.value) {
      if (props.provider === 'local') {
        return !modelForm.value.filename
      }
      return !modelForm.value.id || !modelForm.value.key
    }
    return !editingModel.value?.name
  })

  return {
    state,
    addModel,
    updateModel,
    removeModel,
    removeAllModels,
    fetchProviderModels,
    providerModels,
    isLoadingModels,
    editingModel,
    isAddingNew,
    modelForm,
    filterText,
    lastDerivedName,
    agentDefaults,
    filteredProviderModels,
    groupsByKey,
    alreadyConfiguredFilenames,
    editingArgsStr,
    isSubmitDisabled,
    loadModels,
    startAdd,
    scanAndAdd,
    cancelEdit,
    saveNewModel,
    addDiscoveredModel,
    handleClearAll,
    handleEdit,
    saveEdit,
    handleRemove,
  }
}
