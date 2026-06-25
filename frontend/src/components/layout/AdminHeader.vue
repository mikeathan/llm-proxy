<script setup lang="ts">
import { ref, onMounted } from "vue";
import type { AppTab } from "../../types";
import Icon from "../icons/Icon.vue";

defineProps<{
  activeTab: AppTab;
}>();

defineEmits<{
  (e: "update:activeTab", tab: AppTab): void;
}>();

const version = ref<string | null>(null);

onMounted(async () => {
  try {
    const res = await fetch("/admin/api/version");
    if (res.ok) {
      const data = await res.json();
      version.value = data.version ?? null;
    }
  } catch {
    // Non-critical — silently skip
  }
});
</script>

<template>
  <header class="header-container">
    <div class="header-inner">
      <h1 class="header-title">
        <Icon name="lightning" size="md" class="header-icon" />
        LLM Proxy Admin
        <span v-if="version" class="version-badge">{{ version }}</span>
      </h1>

      <nav class="nav-links">
        <button
          @click="$emit('update:activeTab', 'dashboard')"
          :class="[
            'nav-button',
            activeTab === 'dashboard'
              ? 'nav-button-active'
              : 'nav-button-inactive',
          ]"
        >
          Dashboard
        </button>
        <button
          @click="$emit('update:activeTab', 'settings')"
          :class="[
            'nav-button',
            activeTab === 'settings'
              ? 'nav-button-active'
              : 'nav-button-inactive',
          ]"
        >
          Settings
        </button>
        <button
          @click="$emit('update:activeTab', 'logs')"
          :class="[
            'nav-button',
            activeTab === 'logs' ? 'nav-button-active' : 'nav-button-inactive',
          ]"
        >
          Process Logs
        </button>
        <button
          @click="$emit('update:activeTab', 'agent-ide')"
          :class="[
            'nav-button',
            activeTab === 'agent-ide'
              ? 'nav-button-active'
              : 'nav-button-inactive',
          ]"
        >
          Agent IDE
        </button>
      </nav>
    </div>
  </header>
</template>

<style scoped lang="postcss">
.header-container {
  @apply bg-gray-800 border-b border-gray-700 p-4 sticky top-0 z-10;
}
.header-inner {
  @apply max-w-7xl mx-auto flex flex-col md:flex-row justify-between items-center gap-4;
}
.header-title {
  @apply text-xl font-bold text-white flex items-center gap-2;
}
.header-icon {
  @apply w-6 h-6 text-blue-500;
}
.version-badge {
  @apply text-xs font-mono font-normal text-gray-400 bg-gray-700 border border-gray-600 px-2 py-0.5 rounded-full;
}
.nav-links {
  @apply flex gap-2;
}
.nav-button {
  @apply px-4 py-2 rounded-md text-sm font-medium transition-colors;
}
.nav-button-active {
  @apply bg-blue-600 text-white;
}
.nav-button-inactive {
  @apply bg-gray-800 text-gray-400 hover:bg-gray-700;
}
</style>
