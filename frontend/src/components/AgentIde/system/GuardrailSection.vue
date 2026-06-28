<script setup lang="ts">
defineProps<{
  title: string
  enabled: boolean
}>()

const emit = defineEmits<{
  (e: "toggle", v: boolean): void
}>()
</script>

<template>
  <div class="config-card">
    <div class="card-header">
      <h3 class="card-title">{{ title }}</h3>
      <slot name="toggle">
        <label class="switch-row">
          <input
            type="checkbox"
            class="switch-input"
            :checked="enabled"
            @change="emit('toggle', ($event.target as HTMLInputElement).checked)"
          />
        </label>
      </slot>
    </div>
    <div v-if="enabled" class="card-body">
      <slot />
    </div>
  </div>
</template>

<style scoped lang="postcss">
.config-card {
  @apply bg-gray-800/50 rounded-xl border border-gray-700/50 p-5 shadow-xl flex flex-col;
}

.card-header {
  @apply flex items-center justify-between mb-4 pb-3 border-b border-gray-700/50;
}

.card-title {
  @apply text-[10px] font-black text-blue-400 uppercase tracking-widest;
}

.card-body {
  @apply space-y-4;
}

.switch-row {
  @apply flex items-center gap-3 cursor-pointer select-none;
}

.switch-input {
  @apply w-4 h-4 rounded bg-gray-900 border-gray-700 text-blue-600;
}
</style>
