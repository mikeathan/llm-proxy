<script setup lang="ts">
import { ref, computed, onMounted, inject } from 'vue'
import SystemStatus from './SystemStatus.vue'
import { useModels } from '../../composables/models/useModels'
import { useMetrics } from '../../composables/system/useMetrics'
import { PROVIDER_STYLES } from '../../constants/providers'
import type { SettingsTab } from '../../types'

const {
  state,
  activeModel,
  startModel,
  stopModel,
  refresh,
} = useModels()

const { metrics } = useMetrics()

const setActiveSettingsTab = inject<(tab: SettingsTab) => void>('setActiveSettingsTab')

const totalModels = computed(() => state.value?.models.length || 0)
const activeCount = computed(() => state.value?.models.filter(m => m.active).length || 0)
const localModelsCount = computed(() => state.value?.models.filter(m => m.provider === 'local').length || 0)
const cloudModelsCount = computed(() => state.value?.models.filter(m => m.provider !== 'local').length || 0)

const activeFilter = ref<'local' | 'cloud'>('local')

const filteredModels = computed(() => {
  if (activeFilter.value === 'local') {
    return state.value?.models.filter(m => m.provider === 'local') || []
  }
  return state.value?.models.filter(m => m.provider !== 'local') || []
})

const isAnyLocalModelStarting = computed(() => {
  // Logic to determine if a local model is in transitional state
  // For now, we just check if ANY local model is active
  return state.value?.models.some(m => m.provider === 'local' && m.active)
})

function goToSettings(m: any) {
  if (setActiveSettingsTab) {
    if (m.provider === 'local') {
      setActiveSettingsTab('local-models')
    } else {
      setActiveSettingsTab(m.provider as SettingsTab)
    }
  }
}

onMounted(() => {
  refresh()
})
</script>

<template>
  <div class="dashboard-shell">
    <!-- Top System Metrics -->
    <SystemStatus
      :activeModel="activeModel"
      :metrics="metrics"
      @stopModel="stopModel"
    />

    <!-- Quick Stats -->
    <div class="metric-grid">
      <div class="stat-card">
        <h3 class="stat-label">Total Models</h3>
        <span class="stat-value">{{ totalModels }}</span>
      </div>
      <div class="stat-card">
        <h3 class="stat-label">Active</h3>
        <span class="stat-value text-green-400">{{ activeCount }}</span>
      </div>
      <div class="stat-card" @click="activeFilter = 'local'" :class="{ 'ring-2 ring-blue-500/50 cursor-pointer': activeFilter === 'local' }">
        <h3 class="stat-label">Local</h3>
        <span class="stat-value text-blue-400">{{ localModelsCount }}</span>
      </div>
      <div class="stat-card" @click="activeFilter = 'cloud'" :class="{ 'ring-2 ring-purple-500/50 cursor-pointer': activeFilter === 'cloud' }">
        <h3 class="stat-label">Cloud</h3>
        <span class="stat-value text-purple-400">{{ cloudModelsCount }}</span>
      </div>
    </div>

    <!-- Models Section -->
    <div class="models-section">
      <div class="section-header">
        <h2 class="section-title">Models Configuration</h2>
        <div class="tab-switcher">
          <button 
            @click="activeFilter = 'local'"
            :class="['tab-btn', activeFilter === 'local' ? 'tab-btn--active' : 'tab-btn--inactive']"
          >
            Local Engines
            <span class="count-pill">{{ localModelsCount }}</span>
          </button>
          <button 
            @click="activeFilter = 'cloud'"
            :class="['tab-btn', activeFilter === 'cloud' ? 'tab-btn--active' : 'tab-btn--inactive']"
          >
            Cloud Models
            <span class="count-pill">{{ cloudModelsCount }}</span>
          </button>
        </div>
      </div>

      <div v-if="!filteredModels.length" class="empty-state">
        No {{ activeFilter }} models configured yet.
        <button @click="setActiveSettingsTab?.(activeFilter === 'local' ? 'local-models' : 'openai')" class="settings-link">
          Go to Settings
        </button> to add one.
      </div>
      
      <div v-else class="models-grid">
        <div
          v-for="m in filteredModels"
          :key="m.name"
          :class="['model-card', m.active ? 'model-card--active' : 'model-card--inactive']"
        >
          <div class="model-card-header">
            <div class="model-identity">
              <span
                :class="[
                  'provider-badge',
                  PROVIDER_STYLES[m.provider as keyof typeof PROVIDER_STYLES] || '',
                ]"
              >
                {{ m.provider }}
              </span>
              <span class="model-name">{{ m.name }}</span>
            </div>
            <div class="model-status">
              <span
                class="status-dot"
                :class="m.active ? 'status-dot--online' : 'status-dot--offline'"
              ></span>
              <span class="status-text" :class="m.active ? 'text-green-400' : 'text-gray-500'">
                {{ m.active ? 'Active' : 'Idle' }}
              </span>
            </div>
          </div>
          
          <div class="model-card-body">
            <span class="model-id-label">{{ m.provider === 'local' ? 'FILE' : 'MODEL ID' }}</span>
            <span class="model-id-value">{{ m.model_id || m.filename || '—' }}</span>
          </div>

          <div class="model-card-footer">
            <template v-if="m.provider === 'local'">
              <button
                @click="m.active ? stopModel() : startModel(m.name)"
                :disabled="!m.active && isAnyLocalModelStarting"
                :class="[
                  'btn-control',
                  m.active ? 'btn-control--stop' : 'btn-control--start',
                  (!m.active && isAnyLocalModelStarting) ? 'opacity-50 cursor-not-allowed grayscale' : ''
                ]"
              >
                {{ m.active ? 'Stop' : 'Start' }}
              </button>
            </template>
            <button @click="goToSettings(m)" class="btn-settings-link">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" class="w-3.5 h-3.5"><path d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"></path><path d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path></svg>
              Settings
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.dashboard-shell {
  @apply space-y-6;
}

.metric-grid {
  @apply grid grid-cols-2 md:grid-cols-4 gap-4;
}

.stat-card {
  @apply bg-gray-800 rounded-lg p-5 border border-gray-700 shadow-lg transition-all;
}

.stat-label {
  @apply text-xs font-bold text-gray-500 uppercase tracking-wider mb-1;
}

.stat-value {
  @apply text-2xl font-bold text-white;
}

.section-header {
  @apply flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6;
}

.section-title {
  @apply text-lg font-bold text-white;
}

.tab-switcher {
  @apply flex items-center bg-gray-900/50 p-1 rounded-xl border border-gray-700/50;
}

.tab-btn {
  @apply flex items-center gap-2 px-4 py-2 text-xs font-bold rounded-lg transition-all active:scale-95;
}

.tab-btn--active {
  @apply bg-gray-700 text-white shadow-lg;
}

.tab-btn--inactive {
  @apply text-gray-500 hover:text-gray-300;
}

.count-pill {
  @apply bg-gray-900/80 px-1.5 py-0.5 rounded text-[10px] text-gray-400;
}

.models-section {
  @apply bg-gray-800 rounded-2xl border border-gray-700/50 p-6 shadow-2xl;
}

.empty-state {
  @apply text-center text-sm text-gray-500 italic py-16 bg-gray-900/20 rounded-xl border border-dashed border-gray-700;
}

.settings-link {
  @apply text-blue-400 hover:text-blue-300 underline font-medium;
}

.models-grid {
  @apply grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-5;
}

.model-card {
  @apply rounded-xl border p-5 flex flex-col gap-4 transition-all duration-300;
}

.model-card--active {
  @apply bg-blue-900/10 border-blue-500/40 shadow-[0_8px_30px_rgb(0,0,0,0.12)] scale-[1.02];
}

.model-card--inactive {
  @apply bg-gray-900/40 border-gray-700/50 hover:border-gray-600 hover:bg-gray-900/60;
}

.model-card-header {
  @apply flex items-center justify-between;
}

.model-identity {
  @apply flex items-center gap-2.5 min-w-0 flex-1;
}

.provider-badge {
  @apply px-2 py-0.5 rounded text-[9px] uppercase font-black tracking-widest border shrink-0;
}

.model-name {
  @apply text-sm font-extrabold text-white truncate tracking-tight;
}

.model-status {
  @apply flex items-center gap-2 shrink-0 bg-black/20 px-2 py-1 rounded-full;
}

.status-dot {
  @apply w-1.5 h-1.5 rounded-full;
}

.status-dot--online {
  @apply bg-green-500 shadow-[0_0_8px_rgba(34,197,94,0.6)];
}

.status-dot--offline {
  @apply bg-gray-600;
}

.status-text {
  @apply text-[9px] font-black uppercase tracking-widest;
}

.model-card-body {
  @apply flex flex-col gap-1 py-2 border-y border-gray-700/30;
}

.model-id-label {
  @apply text-[9px] font-bold text-gray-600 uppercase tracking-tighter;
}

.model-id-value {
  @apply text-[11px] text-gray-400 font-mono truncate;
}

.model-card-footer {
  @apply flex items-center justify-between mt-1;
}

.btn-control {
  @apply text-[10px] font-black uppercase tracking-widest px-4 py-2 rounded-lg transition-all active:scale-95 min-w-[80px];
}

.btn-control--start {
  @apply bg-blue-600 text-white shadow-lg shadow-blue-900/20 hover:bg-blue-500;
}

.btn-control--stop {
  @apply bg-red-600/20 text-red-400 border border-red-500/30 hover:bg-red-600 hover:text-white;
}

.btn-settings-link {
  @apply flex items-center gap-1.5 text-[10px] font-bold text-gray-500 hover:text-white transition-colors uppercase tracking-widest;
}
</style>
