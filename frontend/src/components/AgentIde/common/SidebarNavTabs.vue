<script setup lang="ts">
import Icon from "../../icons/Icon.vue"

defineProps<{
  modelValue: "explorer" | "automations" | "recordings" | "memory" | "activity"
  recordingsEnabled: boolean
}>()

const emit = defineEmits<{
  (e: "update:modelValue", v: "explorer" | "automations" | "recordings" | "memory" | "activity"): void
}>()
</script>

<template>
  <div class="sidebar-header">
    <div class="sidebar-tabs-icon" :class="recordingsEnabled ? 'grid-cols-3' : 'grid-cols-2'">
      <button
        @click="emit('update:modelValue', 'explorer')"
        class="sidebar-tab-icon"
        :class="modelValue === 'explorer' ? 'sidebar-tab-icon--active' : 'sidebar-tab-icon--inactive'"
      >
        <Icon name="lightning" size="sm" />
        <span>Explorer</span>
      </button>
      <button
        @click="emit('update:modelValue', 'automations')"
        class="sidebar-tab-icon"
        :class="modelValue === 'automations' ? 'sidebar-tab-icon--active' : 'sidebar-tab-icon--inactive'"
      >
        <Icon name="play" size="sm" />
        <span>Automations</span>
      </button>
      <button
        v-if="recordingsEnabled"
        @click="emit('update:modelValue', 'recordings')"
        class="sidebar-tab-icon"
        :class="modelValue === 'recordings' ? 'sidebar-tab-icon--active' : 'sidebar-tab-icon--inactive'"
      >
        <Icon name="refresh" size="sm" />
        <span>Recordings</span>
      </button>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.sidebar-header {
  @apply p-2 px-3 border-b border-gray-700;
}

.sidebar-tabs-icon {
  @apply grid gap-1 bg-gray-900 rounded p-0.5;
}

.sidebar-tab-icon {
  @apply flex items-center justify-center gap-1.5 h-7 px-2 rounded transition-colors 
         text-[10px] font-medium text-gray-400 truncate
         disabled:opacity-30 disabled:cursor-not-allowed;
}

.sidebar-tab-icon--active {
  @apply bg-gray-700 text-white;
}

.sidebar-tab-icon--inactive {
  @apply hover:text-gray-200;
}
</style>
