<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { useWorkspaces } from '../composables/useWorkspaces'
import type { Workspace } from '../types/workspace'
import { useModels } from '../composables/useModels'

const { workspaces, loading, fetchWorkspaces, saveWorkspace, deleteWorkspace, triggerHeartbeat } = useWorkspaces()
const { state: modelsState, refresh: refreshModels } = useModels()

const showForm = ref(false)
const editingWorkspace = ref<Workspace | null>(null)

// Ensure models are loaded
onMounted(() => {
  fetchWorkspaces()
  if (!modelsState.value?.models?.length) {
    refreshModels()
  }
})

const getAvailableModels = computed(() => {
  return modelsState.value?.models?.map(m => m.name) || []
})

const openCreateForm = () => {
  editingWorkspace.value = {
    id: `agent-${Date.now()}`,
    config: { cron_schedule: '@every 1h', model: getAvailableModels.value[0] || '', temperature: 0.7 },
    state: { last_output: '', last_error: '', next_run_predicted: '', is_running: false },
    heartbeat: 'You are an autonomous agent. Summarize the latest news...'
  }
  showForm.value = true
}

const editWorkspace = (ws: Workspace) => {
  editingWorkspace.value = JSON.parse(JSON.stringify(ws)) // clone
  showForm.value = true
}

const handleSave = async () => {
  if (editingWorkspace.value) {
    await saveWorkspace(editingWorkspace.value)
    showForm.value = false
    editingWorkspace.value = null
  }
}

const handleDelete = async (id: string) => {
  if (confirm(`Are you sure you want to delete workspace ${id}?`)) {
    await deleteWorkspace(id)
  }
}

const handleTrigger = async (id: string) => {
  await triggerHeartbeat(id)
}
</script>

<template>
  <div class="space-y-6">
    <div class="flex justify-between items-center">
      <h1 class="text-2xl font-bold text-white">Agent Workspaces</h1>
      <button @click="openCreateForm" class="bg-blue-600 hover:bg-blue-500 text-white px-4 py-2 rounded shadow transition-colors">
        + New Workspace
      </button>
    </div>

    <!-- Loading State -->
    <div v-if="loading" class="text-gray-400">Loading workspaces...</div>

    <!-- Workspace List -->
    <div v-else class="grid grid-cols-1 xl:grid-cols-2 gap-6">
      <div v-for="ws in workspaces" :key="ws.id" class="bg-gray-800 border border-gray-700 rounded-lg p-5 shadow flex flex-col h-full">
        <div class="flex justify-between items-start mb-4">
          <div>
            <h2 class="text-lg font-semibold text-white flex items-center gap-2">
              {{ ws.id }}
              <span v-if="ws.state.is_running" class="inline-flex h-2.5 w-2.5 rounded-full bg-green-500 animate-pulse" title="Running"></span>
            </h2>
            <p class="text-sm text-gray-400 font-mono mt-1">Schedule: {{ ws.config.cron_schedule }}</p>
          </div>
          <div class="flex gap-2">
            <button @click="handleTrigger(ws.id)" class="text-xs bg-gray-700 hover:bg-gray-600 text-gray-200 px-3 py-1.5 rounded transition-colors" :disabled="ws.state.is_running">
              {{ ws.state.is_running ? 'Running...' : 'Run Now' }}
            </button>
            <button @click="editWorkspace(ws)" class="text-xs bg-gray-700 hover:bg-gray-600 text-gray-200 px-3 py-1.5 rounded transition-colors">
              Edit
            </button>
            <button @click="handleDelete(ws.id)" class="text-xs bg-red-900/50 hover:bg-red-800 text-red-200 px-3 py-1.5 rounded transition-colors">
              Delete
            </button>
          </div>
        </div>

        <div class="grid grid-cols-2 gap-4 mb-4 text-sm">
          <div class="bg-gray-900 rounded p-3">
            <span class="block text-gray-500 mb-1">Model</span>
            <span class="text-gray-300 font-mono text-xs">{{ ws.config.model || 'Default' }}</span>
          </div>
          <div class="bg-gray-900 rounded p-3">
            <span class="block text-gray-500 mb-1">Next Run</span>
            <span class="text-gray-300">{{ ws.state.next_run_predicted && ws.state.next_run_predicted !== '0001-01-01T00:00:00Z' ? new Date(ws.state.next_run_predicted).toLocaleString() : 'N/A' }}</span>
          </div>
        </div>

        <div class="flex-1 min-h-[100px] bg-gray-900 border border-gray-700 rounded p-3 overflow-y-auto">
          <div v-if="ws.state.last_error" class="text-red-400 text-sm whitespace-pre-wrap font-mono">
            Error: {{ ws.state.last_error }}
          </div>
          <div v-else-if="ws.state.last_output" class="text-gray-300 text-sm whitespace-pre-wrap font-mono">
            {{ ws.state.last_output }}
          </div>
          <div v-else class="text-gray-600 text-sm italic h-full flex items-center justify-center">
            No execution history yet.
          </div>
        </div>
      </div>
      
      <div v-if="workspaces.length === 0" class="col-span-full py-12 text-center text-gray-500 bg-gray-800 rounded-lg border border-gray-700 border-dashed">
        No workspaces found. Create one to get started.
      </div>
    </div>

    <!-- Edit/Create Modal -->
    <div v-if="showForm && editingWorkspace" class="fixed inset-0 bg-black/70 flex items-center justify-center p-4 z-50 overflow-y-auto">
      <div class="bg-gray-800 border border-gray-700 rounded-lg p-6 max-w-3xl w-full my-8">
        <h2 class="text-xl font-bold text-white mb-6">{{ workspaces.some((w: Workspace) => w.id === editingWorkspace!.id) ? 'Edit' : 'Create' }} Workspace</h2>
        
        <div class="space-y-4">
          <div>
            <label class="block text-sm font-medium text-gray-300 mb-1">Workspace ID</label>
            <input v-model="editingWorkspace.id" :disabled="workspaces.some((w: Workspace) => w.id === editingWorkspace!.id)" type="text" class="w-full bg-gray-900 border border-gray-600 text-white rounded px-3 py-2 disabled:opacity-50">
          </div>
          
          <div class="grid grid-cols-2 gap-4">
            <div>
              <label class="block text-sm font-medium text-gray-300 mb-1">Cron Schedule</label>
              <input v-model="editingWorkspace.config.cron_schedule" type="text" placeholder="@every 1h" class="w-full bg-gray-900 border border-gray-600 text-white rounded px-3 py-2">
            </div>
            <div>
              <label class="block text-sm font-medium text-gray-300 mb-1">Model</label>
              <select v-model="editingWorkspace.config.model" class="w-full bg-gray-900 border border-gray-600 text-white rounded px-3 py-2">
                <option value="">Default</option>
                <option v-for="model in getAvailableModels" :key="model" :value="model">{{ model }}</option>
              </select>
            </div>
          </div>

          <div>
            <label class="block text-sm font-medium text-gray-300 mb-1">Heartbeat Prompt (Markdown)</label>
            <textarea v-model="editingWorkspace.heartbeat" rows="8" class="w-full bg-gray-900 border border-gray-600 text-white rounded px-3 py-2 font-mono text-sm"></textarea>
          </div>
        </div>

        <div class="mt-6 flex justify-end gap-3">
          <button @click="showForm = false" class="px-4 py-2 text-gray-300 hover:text-white transition-colors">Cancel</button>
          <button @click="handleSave" class="bg-blue-600 hover:bg-blue-500 text-white px-6 py-2 rounded shadow transition-colors">Save</button>
        </div>
      </div>
    </div>

  </div>
</template>
