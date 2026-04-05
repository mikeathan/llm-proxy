<script setup lang="ts">
import { LOG_LEVELS } from "../../constants/api";

defineProps<{ modelValue: string }>();
defineEmits<{ (e: "update", level: string): void }>();

const levels = LOG_LEVELS;
</script>

<template>
  <div class="log-level-panel">
    <span class="llp-label">Log Level</span>
    <div class="llp-buttons">
      <button
        v-for="lvl in levels"
        :key="lvl"
        type="button"
        @click="$emit('update', lvl)"
        :class="[
          'llp-btn',
          modelValue === lvl ? 'llp-btn-active' : 'llp-btn-inactive',
        ]"
      >
        {{ lvl }}
      </button>
    </div>
  </div>
</template>
<style scoped lang="postcss">
.log-level-panel {
  @apply flex items-center gap-3 flex-wrap;
}
.llp-label {
  @apply text-xs font-medium text-gray-400 shrink-0;
}
.llp-buttons {
  @apply flex gap-1.5;
}
.llp-btn {
  @apply px-3 py-1 rounded text-xs font-medium transition-colors;
}
.llp-btn-active {
  @apply bg-blue-600 text-white ring-1 ring-blue-400;
}
.llp-btn-inactive {
  @apply bg-gray-700 text-gray-300 hover:bg-gray-600;
}
</style>
