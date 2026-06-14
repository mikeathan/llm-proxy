<script setup lang="ts">
import Icon from "../icons/Icon.vue";

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
  <div class="collapsible-panel" :class="`collapsible-panel--${position}`">
    <button
      class="panel-toggle"
      @click="emit('toggle')"
      :title="collapsed ? `Show ${title || 'panel'}` : `Hide ${title || 'panel'}`"
    >
      <Icon
        v-if="collapsed"
        :name="position === 'left' ? 'chevron-double-right' : 'chevron-double-left'"
        size="sm"
      />
      <Icon
        v-else
        :name="position === 'left' ? 'chevron-double-left' : 'chevron-double-right'"
        size="sm"
      />
    </button>
    <div v-show="!collapsed" class="panel-content">
      <div v-if="title" class="panel-header">
        <h3 class="panel-title">{{ title }}</h3>
        <slot name="header-actions" />
      </div>
      <slot />
    </div>
  </div>
</template>

<style scoped lang="postcss">
.collapsible-panel {
  @apply flex;
}

.panel-toggle {
  @apply w-8 shrink-0 flex items-center justify-center bg-gray-800/30 hover:bg-gray-700 text-gray-500 hover:text-gray-300 transition-colors cursor-pointer;
  @apply border-r border-gray-700 focus:outline-none;
}

.collapsible-panel--right .panel-toggle {
  @apply border-r-0 border-l border-gray-700;
}

.panel-content {
  @apply flex flex-col overflow-hidden;
}

.panel-header {
  @apply flex items-center justify-between px-4 py-3 border-b border-gray-700 bg-gray-800/50 shrink-0;
}

.panel-title {
  @apply text-xs font-bold text-gray-400 uppercase tracking-widest;
}
</style>
