<script setup lang="ts">
import { ref } from 'vue'
import SystemStatus from '../components/SystemStatus.vue'
import ModelManager from '../components/ModelManager.vue'
import { useModels } from '../composables/useModels'
import { useMetrics } from '../composables/useMetrics'
import type { NewModelForm } from '../types/model'

const {
  state,
  activeModel,
  availableModels,
  startModel,
  stopModel,
  addModel,
  updateModel,
  removeModel,
} = useModels()

const { metrics } = useMetrics()

const newModel = ref<NewModelForm>({ name: '', filename: '', port: 0, args: '' })

const handleAddModel = (): void => {
  if (!newModel.value.name || !newModel.value.filename) return
  addModel({
    ...newModel.value,
    args: newModel.value.args.split(' ').filter(Boolean),
  })
  newModel.value = { name: '', filename: '', port: 0, args: '' }
}
</script>

<template>
  <div class="space-y-6">
    <SystemStatus
      :activeModel="activeModel"
      :metrics="metrics"
      @stopModel="stopModel"
    />
    <ModelManager
      :state="state"
      :availableModels="availableModels"
      v-model:newModel="newModel"
      @startModel="startModel"
      @stopModel="stopModel"
      @removeModel="removeModel"
      @updateModel="updateModel"
      @addModel="handleAddModel"
    />
  </div>
</template>
