<script setup lang="ts">
import NotificationDot from '../../common/NotificationDot.vue'

defineProps<{
  modelValue: "explorer" | "workspace" | "monitor"
  workspaceMiddleTab: "pulse" | "chat"
  canOpenAssistant: boolean
  chatRunning?: boolean
  runningAssistantCount?: number
}>()

const emit = defineEmits<{
  (e: "update:modelValue", v: "explorer" | "workspace" | "monitor"): void
  (e: "toggle-chat"): void
}>()

function setPanel(v: "explorer" | "workspace" | "monitor") {
  emit("update:modelValue", v)
}
</script>

<template>
  <div class="mobile-tabs">
    <button
      @click="setPanel('explorer')"
      :class="['mobile-tab', modelValue === 'explorer' ? 'mobile-tab--active' : '']"
    >
      Explorer
    </button>
    <button
      @click="setPanel('workspace')"
      :class="['mobile-tab', modelValue === 'workspace' && workspaceMiddleTab === 'pulse' ? 'mobile-tab--active' : '']"
    >
      Workspace
    </button>
    <button
      @click="setPanel('monitor')"
      :class="['mobile-tab', modelValue === 'monitor' ? 'mobile-tab--active' : '']"
    >
      Monitor
    </button>
    <button
      @click="emit('toggle-chat')"
      :disabled="!canOpenAssistant"
      :class="['mobile-tab', workspaceMiddleTab === 'chat' && modelValue === 'workspace' ? 'mobile-tab--active' : '', { 'mobile-tab--running': chatRunning }]"
      title="Open Workspace Assistant"
    >
      Chat
      <NotificationDot v-if="chatRunning" :count="runningAssistantCount" />
    </button>
  </div>
</template>

<style scoped lang="postcss">
.mobile-tabs {
  @apply flex lg:hidden gap-1 bg-gray-800/50 rounded-xl p-1 shrink-0 border border-white/5;
}

.mobile-tab {
  @apply relative flex-1 py-2 px-3 text-xs font-semibold rounded-lg transition-colors text-gray-400;
}

.mobile-tab--active {
  @apply bg-blue-600 text-white shadow-md;
}

.mobile-tab--running {
  color: var(--color-live, #22c55e);
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--color-live, #22c55e) 40%, transparent);
}
</style>
