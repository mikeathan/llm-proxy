<template>
  <header class="header-container">
    <div class="header-inner">
      <h1 class="header-title">
        <svg
          class="header-icon"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M13 10V3L4 14h7v7l9-11h-7z"
          ></path>
        </svg>
        LLM Proxy Admin
      </h1>

      <nav class="nav-links">
        <button
          @click="$emit('update:activeTab', 'dashboard')"
          :class="['nav-button', activeTab === 'dashboard' ? 'nav-button-active' : 'nav-button-inactive']"
        >
          Dashboard
        </button>
        <button
          @click="$emit('update:activeTab', 'settings')"
          :class="['nav-button', activeTab === 'settings' ? 'nav-button-active' : 'nav-button-inactive']"
        >
          Settings
        </button>
        <button
          @click="$emit('update:activeTab', 'logs')"
          :class="['nav-button', activeTab === 'logs' ? 'nav-button-active' : 'nav-button-inactive']"
        >
          Process Logs
        </button>
        <button
          @click="$emit('update:activeTab', 'workspaces')"
          :class="['nav-button', activeTab === 'workspaces' ? 'nav-button-active' : 'nav-button-inactive']"
        >
          Workspaces
        </button>
        <button
          @click="$emit('update:activeTab', 'workspaces')"
          :class="['nav-button', activeTab === 'workspaces' ? 'nav-button-active' : 'nav-button-inactive']"
        >
          Workspaces
        </button>
      </nav>
    </div>
  </header>
</template>

<script setup lang="ts">
defineProps<{
  activeTab: string;
}>();

defineEmits<{
  (e: "update:activeTab", tab: string): void;
}>();
</script>

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
