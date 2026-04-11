<script setup lang="ts">
import { computed } from 'vue'
import type { NewModelForm } from '../../types'

const props = defineProps<{
  modelValue: NewModelForm
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: NewModelForm): void
}>()

const localModel = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val)
})
</script>

<template>
  <div class="grid grid-cols-1 gap-3">
    <div>
      <label class="form-label">Filename (.gguf)</label>
      <input v-model="localModel.filename" type="text" required placeholder="qwen2.5-3b.gguf" class="form-input">
    </div>
  </div>

  <div class="form-grid-3 mt-3">
    <div class="form-col-1">
      <label class="form-label">Port</label>
      <input v-model.number="localModel.port" type="number" class="form-input">
    </div>
    <div class="form-col-2">
      <label class="form-label">Extra Args</label>
      <input v-model="localModel.args" type="text" placeholder="-c 4096" class="form-input font-mono">
    </div>
  </div>
</template>

<style scoped lang="postcss">
.form-grid-3 {
  @apply grid grid-cols-1 sm:grid-cols-3 gap-3;
}
.form-col-1 {
  @apply sm:col-span-1;
}
.form-col-2 {
  @apply sm:col-span-2;
}
.form-label {
  @apply block text-[10px] uppercase font-bold text-gray-500 mb-1.5 tracking-wider;
}
.form-input {
  @apply w-full bg-gray-800 border border-gray-700 rounded-md px-3 py-2 text-sm text-white focus:border-blue-600 focus:ring-1 focus:ring-blue-600 outline-none transition-all;
}
</style>
