<script setup lang="ts">
import Icon from "../../icons/Icon.vue";

withDefaults(
  defineProps<{
    collapsed: boolean;
    title?: string;
    position?: "left" | "right";
  }>(),
  { title: "", position: "left" },
);

const emit = defineEmits<{
  (e: "toggle"): void;
}>();
</script>

<template>
  <div
    class="collapsible-panel"
    :class="[`collapsible-panel--${position}`, { 'is-collapsed': collapsed }]"
  >
    <div class="panel-content">
      <div v-if="title" class="panel-header">
        <h3 class="panel-title">{{ title }}</h3>
        <div class="flex items-center gap-2">
          <slot name="header-actions" />
          <button @click="emit('toggle')" class="panel-close-btn" title="Collapse panel">
            <Icon :name="position === 'left' ? 'chevron-double-left' : 'chevron-double-right'" size="sm" />
          </button>
        </div>
      </div>
      <slot />
    </div>
  </div>
</template>

<style scoped lang="postcss">
.collapsible-panel {
  @apply relative flex h-full transition-all duration-300;
}

/* When expanded, the panel content has a fixed width */
.panel-content {
  @apply flex flex-col h-full w-64 overflow-hidden bg-gray-800/50 border-gray-700 transition-all duration-300;
}

.collapsible-panel--left .panel-content {
  @apply border-r;
}
.collapsible-panel--right .panel-content {
  @apply border-l;
}

/* When collapsed, width goes to 0 and border is hidden */
.is-collapsed .panel-content {
  @apply w-0 border-transparent opacity-0 px-0;
}

.panel-header {
  @apply flex items-center justify-between px-4 py-3 border-b border-gray-700 shrink-0 min-w-max;
}

.panel-title {
  @apply text-xs font-bold text-gray-400 uppercase tracking-widest;
}

.panel-close-btn {
  @apply p-1.5 rounded-md hover:bg-gray-700 text-gray-500 hover:text-gray-300 transition-colors flex items-center justify-center focus:outline-none;
}
</style>
