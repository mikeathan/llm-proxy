<script setup lang="ts">
import { ref, computed } from 'vue'
import SystemStatus from './SystemStatus.vue'
import ModelManager from '../ModelManager.vue'
import { useModels } from '../../composables/useModels'
import { useMetrics } from '../../composables/useMetrics'
import { normaliseFormArgs } from '../../utils/models'

const {
  state,
  activeModel,
  availableModels,
  startModel,
  stopModel,
  addModel,
  updateModel,
  removeModel,
} = useModels()

const { metrics } = useMetrics()

const activeTab = ref<'local' | 'cloud'>('local')

const handleAddModel = (model: any): void => {
  if (!model.name) return
  if (model.provider === 'local' && !model.filename) return
  if (model.provider !== 'local' && !model.model_id) return

  addModel({ ...model, args: normaliseFormArgs(model.args) })
}

const localModelsCount = computed(() => state.value?.models.filter(m => m.provider === 'local').length || 0)
const cloudModelsCount = computed(() => state.value?.models.filter(m => m.provider !== 'local').length || 0)
</script>

<template>
  <div class="dashboard-shell">
    <!-- Top System Metrics (Host/GPU) -->
    <SystemStatus
      v-if="activeTab === 'local'"
      :activeModel="activeModel"
      :metrics="metrics"
      @stopModel="stopModel"
      class="system-status-wrapper"
    />

    <!-- Cloud Metrics Placeholder -->
    <div v-else class="metric-grid">
      <div class="stat-card">
        <h3 class="stat-label">Cloud Provider Status</h3>
        <div class="status-row">
          <div class="status-dot status-dot--active"></div>
          <span class="status-text">All Systems Operational</span>
        </div>
      </div>
      <div class="stat-card">
        <h3 class="stat-label">API Latency (avg)</h3>
        <span class="stat-value">124ms</span>
      </div>
      <div class="stat-card">
        <h3 class="stat-label">Cloud Models Active</h3>
        <span class="stat-value">{{ cloudModelsCount }}</span>
      </div>
    </div>

    <!-- Provider Tab Switcher -->
    <div class="tab-switcher">
      <button
        @click="activeTab = 'local'"
        class="tab-btn"
        :class="activeTab === 'local' ? 'tab-btn--active' : 'tab-btn--inactive'"
      >
        Local Instances
        <span
          class="tab-count"
          :class="activeTab === 'local' ? 'tab-count--active' : 'tab-count--inactive'"
        >
          {{ localModelsCount }}
        </span>
      </button>
      <button
        @click="activeTab = 'cloud'"
        class="tab-btn"
        :class="activeTab === 'cloud' ? 'tab-btn--active' : 'tab-btn--inactive'"
      >
        Cloud Providers
        <span
          class="tab-count"
          :class="activeTab === 'cloud' ? 'tab-count--active' : 'tab-count--inactive'"
        >
          {{ cloudModelsCount }}
        </span>
      </button>
    </div>

    <!-- Main Content Area -->
    <div class="content-area">
      <transition
        enter-active-class="transition duration-300 ease-out"
        enter-from-class="transform translate-y-4 opacity-0"
        enter-to-class="transform translate-y-0 opacity-100"
        leave-active-class="transition duration-200 ease-in"
        leave-from-class="transform translate-y-0 opacity-100"
        leave-to-class="transform translate-y-4 opacity-0"
      >
        <div :key="activeTab">
          <ModelManager
            :state="state"
            :filterProvider="activeTab === 'local' ? 'local' : 'cloud'"
            :availableModels="availableModels"
            @startModel="startModel"
            @stopModel="stopModel"
            @removeModel="removeModel"
            @updateModel="updateModel"
            @addModel="handleAddModel"
          />
        </div>
      </transition>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.dashboard-shell {
  @apply space-y-6;
}

.system-status-wrapper {
  @apply animate-in fade-in slide-in-from-top-4 duration-500;
}

.metric-grid {
  @apply grid grid-cols-1 md:grid-cols-3 gap-6 animate-in fade-in slide-in-from-top-4 duration-500;
}

.status-row {
  @apply flex items-center gap-2 mt-1;
}

.status-text {
  @apply text-xl font-bold text-white;
}

.stat-card {
  @apply bg-gray-800 rounded-lg p-5 border border-gray-700 shadow-lg;
}

.stat-label {
  @apply text-xs font-bold text-gray-500 uppercase tracking-wider mb-1;
}

.stat-value {
  @apply text-2xl font-bold text-white;
}

.status-dot {
  @apply h-2.5 w-2.5 rounded-full;
}

.status-dot--active {
  @apply bg-green-500 shadow-[0_0_8px_rgba(34,197,94,0.6)];
}

.tab-switcher {
  @apply flex items-center gap-1 bg-gray-800 p-1 rounded-lg w-fit border border-gray-700;
}

.tab-btn {
  @apply px-4 py-2 rounded-md text-sm font-medium transition-all flex items-center gap-2;
}

.tab-btn--active {
  @apply bg-gray-700 text-white shadow-sm;
}

.tab-btn--inactive {
  @apply text-gray-400 hover:text-white;
}

.tab-count {
  @apply text-[10px] bg-gray-900 border px-1.5 py-0.5 rounded-full;
}

.tab-count--active {
  @apply text-blue-400 border-blue-900/50;
}

.tab-count--inactive {
  @apply text-gray-400 border-gray-700;
}

.content-area {
  @apply relative min-h-[400px];
}
</style>
