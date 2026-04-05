<script setup lang="ts">
import { ref, computed } from 'vue'
import SystemStatus from '../components/dashboard/SystemStatus.vue'
import ModelManager from '../components/ModelManager.vue'
import { useModels } from '../composables/useModels'
import { useMetrics } from '../composables/useMetrics'
import { normaliseFormArgs } from '../utils/models'

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
  <div class="space-y-6">
    <!-- Top System Metrics (Host/GPU) -->
    <SystemStatus
      v-if="activeTab === 'local'"
      :activeModel="activeModel"
      :metrics="metrics"
      @stopModel="stopModel"
      class="animate-in fade-in slide-in-from-top-4 duration-500"
    />

    <!-- Cloud Metrics Placeholder -->
    <div v-else class="grid grid-cols-1 md:grid-cols-3 gap-6 animate-in fade-in slide-in-from-top-4 duration-500">
      <div class="stat-card">
        <h3 class="stat-label">Cloud Provider Status</h3>
        <div class="flex items-center gap-2 mt-1">
          <div class="h-2.5 w-2.5 rounded-full bg-green-500 shadow-[0_0_8px_rgba(34,197,94,0.6)]"></div>
          <span class="text-xl font-bold text-white">All Systems Operational</span>
        </div>
      </div>
      <div class="stat-card">
        <h3 class="stat-label">API Latency (avg)</h3>
        <span class="text-2xl font-bold text-white">124ms</span>
      </div>
      <div class="stat-card">
        <h3 class="stat-label">Cloud Models Active</h3>
        <span class="text-2xl font-bold text-white">{{ cloudModelsCount }}</span>
      </div>
    </div>

    <!-- Provider Tab Switcher -->
    <div class="flex items-center gap-1 bg-gray-800 p-1 rounded-lg w-fit border border-gray-700">
      <button
        @click="activeTab = 'local'"
        :class="['tab-btn', activeTab === 'local' ? 'tab-active' : '']"
      >
        Local Instances
        <span class="tab-count">{{ localModelsCount }}</span>
      </button>
      <button
        @click="activeTab = 'cloud'"
        :class="['tab-btn', activeTab === 'cloud' ? 'tab-active' : '']"
      >
        Cloud Providers
        <span class="tab-count">{{ cloudModelsCount }}</span>
      </button>
    </div>

    <!-- Main Content Area -->
    <div class="relative min-h-[400px]">
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
.tab-btn {
  @apply px-4 py-2 rounded-md text-sm font-medium transition-all text-gray-400 hover:text-white flex items-center gap-2;
}

.tab-active {
  @apply bg-gray-700 text-white shadow-sm;
}

.tab-count {
  @apply text-[10px] bg-gray-900 border border-gray-700 text-gray-400 px-1.5 py-0.5 rounded-full;
}

.tab-active .tab-count {
  @apply text-blue-400 border-blue-900/50;
}

.stat-card {
  @apply bg-gray-800 rounded-lg p-5 border border-gray-700 shadow-lg;
}

.stat-label {
  @apply text-xs font-bold text-gray-500 uppercase tracking-wider mb-1;
}
</style>
