<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { useDispatcher } from '../composables/useDispatcher'
import type { Automation } from '../types/dispatcher'
import { DispatcherService } from '../services/dispatcherService'

const {
  automations,
  metrics,
  workspaces,
  workspaceFiles,
  loading,
  error,
  fetchAutomations,
  fetchMetrics,
  triggerAutomation,
  fetchWorkspaces,
  fetchWorkspaceFiles,
  createWorkspace,
} = useDispatcher()

const leftTab = ref<'explorer' | 'automations'>('explorer')
const selectedAutomation = ref<Automation | null>(null)
const selectedWorkspace = ref<string | null>(null)
const selectedFile = ref<{ workspace: string, filename: string } | null>(null)
const fileContent = ref<string>('')
const loadingFile = ref(false)
const savingFile = ref(false)
const newWorkspaceName = ref('')
const newFileName = ref('')

const triggering = ref(false)
const lastTriggerResult = ref<string | null>(null)

onMounted(() => {
  fetchAutomations()
  fetchMetrics()
  fetchWorkspaces()
})

const groupedByWorkspace = computed(() => {
  const groups: Record<string, Automation[]> = {}
  for (const auto of automations.value) {
    if (!groups[auto.workspace]) {
      groups[auto.workspace] = []
    }
    groups[auto.workspace]!.push(auto)
  }
  return groups
})

const selectAutomation = (auto: Automation) => {
  selectedAutomation.value = auto
  selectedFile.value = null
  lastTriggerResult.value = null
}

const selectWorkspace = async (wsId: string) => {
  selectedWorkspace.value = selectedWorkspace.value === wsId ? null : wsId
  if (selectedWorkspace.value) {
    await fetchWorkspaceFiles(wsId)
  }
}

const openFile = async (workspace: string, filename: string) => {
  selectedFile.value = { workspace, filename }
  selectedAutomation.value = null
  loadingFile.value = true
  fileContent.value = ''
  try {
    fileContent.value = await DispatcherService.readWorkspaceFile(workspace, filename)
  } catch (err) {
    console.error('Error loading file', err)
    fileContent.value = 'Error loading file content.'
  } finally {
    loadingFile.value = false
  }
}

const saveFile = async () => {
  if (!selectedFile.value) return
  savingFile.value = true
  try {
    await DispatcherService.writeWorkspaceFile(selectedFile.value.workspace, selectedFile.value.filename, fileContent.value)
    // Refresh automations if config was updated
    if (selectedFile.value.filename === 'config.yaml') {
      setTimeout(fetchAutomations, 500) // Give backend time to hot-reload
    }
  } catch (err) {
    console.error('Error saving file', err)
    alert('Error saving file')
  } finally {
    savingFile.value = false
  }
}

const handleCreateWorkspace = async () => {
  if (!newWorkspaceName.value) return
  await createWorkspace(newWorkspaceName.value)
  newWorkspaceName.value = ''
}

const handleCreateFile = async (workspace: string) => {
  if (!newFileName.value) return
  try {
    await DispatcherService.writeWorkspaceFile(workspace, newFileName.value, '')
    newFileName.value = ''
    await fetchWorkspaceFiles(workspace)
  } catch (err) {
    console.error('Error creating file', err)
    alert('Error creating file')
  }
}

const handleTrigger = async () => {
  if (!selectedAutomation.value) return
  triggering.value = true
  lastTriggerResult.value = null
  try {
    await triggerAutomation(selectedAutomation.value.workspace, selectedAutomation.value.name)
    lastTriggerResult.value = `Triggered ${selectedAutomation.value.name} successfully`
  } catch {
    lastTriggerResult.value = `Failed to trigger ${selectedAutomation.value.name}`
  } finally {
    triggering.value = false
  }
}
</script>

<template>
  <div class="h-[calc(100vh-8rem)] flex gap-4">
    <!-- Left Pane: Sidebar -->
    <div class="w-72 flex flex-col bg-gray-800 rounded-lg overflow-hidden">
      <div class="p-4 border-b border-gray-700 flex flex-col gap-3">
        <div class="flex bg-gray-900 rounded p-1">
          <button
            @click="leftTab = 'explorer'"
            :class="['flex-1 py-1 text-xs font-medium rounded', leftTab === 'explorer' ? 'bg-gray-700 text-white' : 'text-gray-400 hover:text-gray-200']"
          >
            Explorer
          </button>
          <button
            @click="leftTab = 'automations'"
            :class="['flex-1 py-1 text-xs font-medium rounded', leftTab === 'automations' ? 'bg-gray-700 text-white' : 'text-gray-400 hover:text-gray-200']"
          >
            Automations
          </button>
        </div>
      </div>
      
      <div class="flex-1 overflow-y-auto">
        <!-- Explorer Tab -->
        <div v-if="leftTab === 'explorer'">
          <div class="p-3 border-b border-gray-750 flex gap-2">
            <input v-model="newWorkspaceName" placeholder="New workspace name" class="flex-1 bg-gray-900 text-xs text-white px-2 py-1.5 rounded border border-gray-700 focus:outline-none focus:border-blue-500" @keyup.enter="handleCreateWorkspace" />
            <button @click="handleCreateWorkspace" class="bg-blue-600 hover:bg-blue-700 text-white px-2 py-1.5 rounded text-xs">+</button>
          </div>
          
          <div v-if="loading" class="p-4 text-gray-500 text-sm">Loading...</div>
          <div v-else>
            <div v-for="ws in workspaces" :key="ws.id" class="border-b border-gray-750">
              <button
                @click="selectWorkspace(ws.id)"
                class="w-full px-4 py-2.5 text-left text-sm text-gray-200 hover:bg-gray-750 flex justify-between items-center"
              >
                <span class="font-medium">📁 {{ ws.id }}</span>
                <span class="text-xs text-gray-500">{{ selectedWorkspace === ws.id ? '▼' : '▶' }}</span>
              </button>
              
              <div v-if="selectedWorkspace === ws.id" class="bg-gray-900/50 pb-2">
                <div class="px-4 py-2 flex gap-2">
                  <input v-model="newFileName" placeholder="New file name" class="flex-1 bg-gray-800 text-xs text-white px-2 py-1 rounded border border-gray-700 focus:outline-none focus:border-blue-500" @keyup.enter="handleCreateFile(ws.id)" />
                  <button @click="handleCreateFile(ws.id)" class="bg-blue-600 hover:bg-blue-700 text-white px-2 py-1 rounded text-xs">+</button>
                </div>
                
                <button
                  v-for="file in workspaceFiles[ws.id]"
                  :key="file"
                  @click="openFile(ws.id, file)"
                  :class="[
                    'w-full px-8 py-1.5 text-left text-xs transition-colors',
                    selectedFile?.workspace === ws.id && selectedFile?.filename === file
                      ? 'bg-blue-600 text-white'
                      : 'text-gray-400 hover:text-gray-200 hover:bg-gray-700'
                  ]"
                >
                  📄 {{ file }}
                </button>
              </div>
            </div>
          </div>
        </div>

        <!-- Automations Tab -->
        <div v-else-if="leftTab === 'automations'">
          <div v-if="loading" class="p-4 text-gray-500 text-sm">Loading...</div>
          <div v-else-if="error" class="p-4 text-red-400 text-sm">{{ error }}</div>
          <div v-else>
            <div v-for="(autos, workspace) in groupedByWorkspace" :key="workspace">
              <div class="px-4 py-2 bg-gray-750 text-xs font-semibold text-gray-400 uppercase">
                {{ workspace }}
              </div>
              <button
                v-for="auto in autos"
                :key="auto.id"
                @click="selectAutomation(auto)"
                :class="[
                  'w-full px-4 py-2.5 text-left text-sm transition-colors',
                  selectedAutomation?.id === auto.id
                    ? 'bg-blue-600 text-white'
                    : 'text-gray-300 hover:bg-gray-700'
                ]"
              >
                <div class="font-medium">{{ auto.name }}</div>
                <div class="text-xs opacity-70 mt-0.5">
                  {{ auto.trigger }} · {{ auto.strategy }}
                </div>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Middle Pane: Details / Editor -->
    <div class="flex-1 flex flex-col bg-gray-800 rounded-lg overflow-hidden">
      <div v-if="!selectedAutomation && !selectedFile" class="flex-1 flex items-center justify-center text-gray-500">
        Select a file or automation to view details
      </div>
      
      <!-- Editor View -->
      <template v-else-if="selectedFile">
        <div class="p-3 border-b border-gray-700 flex justify-between items-center bg-gray-900">
          <div class="flex items-center gap-2">
            <span class="text-gray-400 text-xs">📁 {{ selectedFile.workspace }}</span>
            <span class="text-gray-500">/</span>
            <span class="font-medium text-sm text-gray-200">📄 {{ selectedFile.filename }}</span>
          </div>
          <button 
            @click="saveFile" 
            :disabled="savingFile || loadingFile"
            class="bg-green-600 hover:bg-green-700 disabled:opacity-50 text-white px-3 py-1.5 rounded text-xs font-medium"
          >
            {{ savingFile ? 'Saving...' : 'Save File' }}
          </button>
        </div>
        <div class="flex-1 relative">
          <div v-if="loadingFile" class="absolute inset-0 flex items-center justify-center bg-gray-800/80 z-10">
            <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500"></div>
          </div>
          <textarea 
            v-model="fileContent" 
            class="w-full h-full bg-gray-900 text-gray-300 font-mono text-sm p-4 focus:outline-none resize-none leading-relaxed"
            spellcheck="false"
          ></textarea>
        </div>
      </template>

      <!-- Automation Details View -->
      <template v-else-if="selectedAutomation">
        <div class="p-4 border-b border-gray-700">
          <h2 class="font-semibold text-lg">{{ selectedAutomation.name }}</h2>
          <p class="text-sm text-gray-400 mt-1">
            Workspace: <span class="text-gray-300">{{ selectedAutomation.workspace }}</span>
          </p>
          <div class="flex gap-4 mt-2 text-xs text-gray-500">
            <span>Trigger: {{ selectedAutomation.trigger }}</span>
            <span>Strategy: {{ selectedAutomation.strategy }}</span>
            <span>Task: {{ selectedAutomation.task_file }}</span>
          </div>
        </div>
        <div class="flex-1 p-4 overflow-y-auto">
          <div v-if="lastTriggerResult" :class="[
            'p-3 rounded text-sm mb-4',
            lastTriggerResult.includes('Failed') ? 'bg-red-900/50 text-red-300' : 'bg-green-900/50 text-green-300'
          ]">
            {{ lastTriggerResult }}
          </div>
          <div class="text-gray-400 text-sm">
            <p>Automation execution output will appear here after running.</p>
          </div>
        </div>
      </template>
    </div>

    <!-- Right Pane: Trigger + Metrics -->
    <div class="w-64 flex flex-col gap-4">
      <!-- Trigger Control -->
      <div class="bg-gray-800 rounded-lg p-4">
        <h3 class="font-semibold text-sm text-gray-300 mb-3">Trigger Automation</h3>
        <button
          @click="handleTrigger"
          :disabled="!selectedAutomation || triggering"
          :class="[
            'w-full py-2 px-4 rounded font-medium text-sm transition-colors',
            !selectedAutomation || triggering
              ? 'bg-gray-700 text-gray-500 cursor-not-allowed'
              : 'bg-blue-600 hover:bg-blue-700 text-white'
          ]"
        >
          {{ triggering ? 'Triggering...' : 'Run Now' }}
        </button>
        <p v-if="!selectedAutomation" class="text-xs text-gray-500 mt-2">
          Select an automation first
        </p>
      </div>

      <!-- Dispatcher Metrics -->
      <div class="bg-gray-800 rounded-lg p-4 flex-1">
        <h3 class="font-semibold text-sm text-gray-300 mb-3">System Metrics</h3>
        <div v-if="metrics" class="space-y-2 text-sm">
          <div class="flex justify-between">
            <span class="text-gray-400">Total Runs</span>
            <span class="text-gray-200">{{ metrics.total_executions }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-gray-400">Successful</span>
            <span class="text-green-400">{{ metrics.successful }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-gray-400">Failed</span>
            <span class="text-red-400">{{ metrics.failed }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-gray-400">Skipped</span>
            <span class="text-yellow-400">{{ metrics.skipped }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-gray-400">Avg Latency</span>
            <span class="text-gray-200">{{ Math.round(metrics.total_latency_ms / Math.max(metrics.total_executions, 1)) }}ms</span>
          </div>
        </div>
        <div v-else class="text-gray-500 text-sm">No metrics available</div>
      </div>
    </div>
  </div>
</template>
