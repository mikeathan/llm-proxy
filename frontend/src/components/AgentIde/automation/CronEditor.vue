<script setup lang="ts">
import { ref, watch } from "vue"
import cronstrue from "cronstrue"

const props = defineProps<{
  modelValue: string
  triggerType: string
}>()

const emit = defineEmits<{
  (e: "update:modelValue", v: string): void
}>()

const cronType = ref("custom")
const cronEvery = ref(1)
const cronUnit = ref("hours")

watch([cronType, cronEvery, cronUnit], () => {
  if (cronType.value === "custom") return
  let val = ""
  if (cronType.value === "every") {
    if (cronUnit.value === "minutes") {
      val = `*/${cronEvery.value} * * * *`
    } else if (cronUnit.value === "hours") {
      val = `0 */${cronEvery.value} * * *`
    } else if (cronUnit.value === "days") {
      val = `0 0 */${cronEvery.value} * *`
    }
  }
  emit("update:modelValue", val)
})

watch(
  () => props.triggerType,
  (newVal) => {
    if (newVal === "cron") {
      cronType.value = "custom"
    }
  },
)

const cronDescription = ref("")
watch(
  () => props.modelValue,
  (newVal) => {
    if (props.triggerType === "cron" && newVal) {
      try {
        cronDescription.value = cronstrue.toString(newVal)
      } catch {
        cronDescription.value = "Invalid cron expression"
      }
    } else {
      cronDescription.value = ""
    }
  },
  { immediate: true },
)
</script>

<template>
  <div class="trigger-content">
    <div class="flex gap-2">
      <select v-model="cronType" class="select-input">
        <option value="every">Simple Frequency</option>
        <option value="custom">Custom Expression</option>
      </select>
    </div>

    <div v-if="cronType === 'every'" class="cron-simple-row">
      <span class="cron-simple-label">Run every</span>
      <input type="number" v-model="cronEvery" min="1" class="cron-number-input" />
      <select v-model="cronUnit" class="cron-unit-select">
        <option value="minutes">Minutes</option>
        <option value="hours">Hours</option>
        <option value="days">Days</option>
      </select>
    </div>

    <div class="cron-input-group">
      <input
        :value="modelValue"
        @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)"
        placeholder="* * * * *"
        :readonly="cronType !== 'custom'"
        class="text-input font-mono"
      />
      <div class="cron-preview">{{ cronDescription }}</div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.trigger-content {
  @apply space-y-3;
}

.cron-simple-row {
  @apply flex items-center gap-2;
}

.cron-simple-label {
  @apply text-sm text-gray-400;
}

.cron-number-input {
  @apply w-20 bg-gray-900 text-sm text-white px-3 py-2 rounded border border-gray-700 text-center;
}

.cron-unit-select {
  @apply bg-gray-900 text-sm text-white px-3 py-2 rounded border border-gray-700 w-32;
}

.cron-input-group {
  @apply space-y-1;
}

.cron-preview {
  @apply mt-1 text-xs text-blue-400 min-h-[16px];
}

.select-input {
  @apply w-full bg-gray-900 border border-gray-700 rounded px-3 py-2 text-sm text-white 
         focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-colors disabled:opacity-50;
}

.text-input {
  @apply w-full bg-gray-900 border border-gray-700 rounded px-3 py-2 text-sm text-white 
         focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-colors;
}
</style>
