<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { useDispatcher } from '../composables/useDispatcher'
import type { Automation } from '../types/dispatcher'
import { DispatcherService } from '../services/dispatcherService'
import ConfirmDialog from '../components/ConfirmDialog.vue'

import WorkspaceExplorer from '../components/AgentIde/WorkspaceExplorer.vue'
import CreateAutomationForm from '../components/AgentIde/CreateAutomationForm.vue'
import AutomationsPanel from '../components/AgentIde/AutomationsPanel.vue'
import FileEditor from '../components/AgentIde/FileEditor.vue'
import SystemMetricsPanel from '../components/AgentIde/SystemMetricsPanel.vue'

const confirmDialog = ref<InstanceType<typeof ConfirmDialog> | null>(null)
const pendingAction = ref<(() => Promise<void>) | null>(null)
const dialogProps = ref({ title: '', message: '', type: 'warning' as const })

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
  deleteWorkspaceFile,
  deleteWorkspace,
  createAutomation,
} = useDispatcher()

const leftTab = ref<'explorer' | 'automations'>('explorer')
const selectedAutomation = ref<Automation | null>(null)
const selectedWorkspace = ref<string | null>(null)
const selectedFile = ref<{ workspace: string, filename: string } | null>(null)
const fileContent = ref<string>('')
const loadingFile = ref(false)
const savingFile = ref(false)

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

const handleSelectAutomation = (auto: Automation) => {
  selectedAutomation.value = auto
  selectedFile.value = null
  lastTriggerResult.value = null
}

const handleSelectWorkspace = async (wsId: string) => {
  selectedWorkspace.value = selectedWorkspace.value === wsId ? null : wsId
  if (selectedWorkspace.value) {
    await fetchWorkspaceFiles(wsId)
  }
}

const handleOpenFile = async (workspace: string, filename: string) => {
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

const handleSaveFile = async () => {
  if (!selectedFile.value) return
  savingFile.value = true
  try {
    await DispatcherService.writeWorkspaceFile(selectedFile.value.workspace, selectedFile.value.filename, fileContent.value)
    if (selectedFile.value.filename === 'config.yaml') {
      setTimeout(fetchAutomations, 500)
    }
  } catch (err) {
    console.error('Error saving file', err)
    alert('Error saving file')
  } finally {
    savingFile.value = false
  }
}

const handleCreateWorkspace = async (name: string) => {
  await createWorkspace(name)
}

const handleCreateFile = async (workspace: string, filename: string) => {
  try {
    await DispatcherService.writeWorkspaceFile(workspace, filename, '')
    await fetchWorkspaceFiles(workspace)
  } catch (err) {
    console.error('Error creating file', err)
    alert('Error creating file')
  }
}

const handleDeleteWorkspace = async (wsId: string) => {
  dialogProps.value = {
    title: 'Delete Workspace',
    message: `Are you sure you want to delete workspace "${wsId}"? This action cannot be undone.`,
    type: 'warning'
  }
  pendingAction.value = async () => {
    await deleteWorkspace(wsId)
    await fetchWorkspaces()
  }
  confirmDialog.value?.open()
}

const handleDeleteFile = async (wsId: string, file: string) => {
  dialogProps.value = {
    title: 'Delete File',
    message: `Are you sure you want to delete file "${file}" from workspace "${wsId}"?`,
    type: 'warning'
  }
  pendingAction.value = async () => {
    await deleteWorkspaceFile(wsId, file)
    await fetchWorkspaceFiles(wsId)
  }
  confirmDialog.value?.open()
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

const handleCreateAutomation = async (workspace: string, data: any) => {
  try {
    await createAutomation(workspace, data)
  } catch (err) {
    console.error('Error creating automation', err)
    alert('Error creating automation')
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
        <WorkspaceExplorer 
          v-if="leftTab === 'explorer'"
          :workspaces="workspaces"
          :workspaceFiles="workspaceFiles"
          :selectedWorkspace="selectedWorkspace"
          :selectedFile="selectedFile"
          :loading="loading"
          @select-workspace="handleSelectWorkspace"
          @create-workspace="handleCreateWorkspace"
          @delete-workspace="handleDeleteWorkspace"
          @open-file="handleOpenFile"
          @create-file="handleCreateFile"
          @delete-file="handleDeleteFile"
        />

        <!-- Automations Tab -->
        <div v-else-if="leftTab === 'automations'">
          <CreateAutomationForm
            :workspaces="workspaces"
            :workspaceFiles="workspaceFiles"
            @create-automation="handleCreateAutomation"
            @fetch-files="fetchWorkspaceFiles"
          />

          <div v-if="loading" class="p-4 text-gray-500 text-sm">Loading...</div>
          <div v-else-if="error" class="p-4 text-red-400 text-sm">{{ error }}</div>
          <AutomationsPanel 
            v-else
            :groupedAutomations="groupedByWorkspace"
            :selectedAutomationId="selectedAutomation?.id"
            @select-automation="handleSelectAutomation"
          />
        </div>
      </div>
    </div>

    <!-- Middle Pane: Details / Editor -->
    <div class="flex-1 flex flex-col bg-gray-800 rounded-lg overflow-hidden">
      <div v-if="!selectedAutomation && !selectedFile" class="flex-1 flex items-center justify-center text-gray-500">
        Select a file or automation to view details
      </div>
      
      <!-- Editor View -->
      <FileEditor 
        v-else-if="selectedFile"
        :file="selectedFile"
        :content="fileContent"
        :loading="loadingFile"
        :saving="savingFile"
        @update:content="fileContent = $event"
        @save="handleSaveFile"
      />

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

    <ConfirmDialog
      ref="confirmDialog"
      v-bind="dialogProps"
      @confirm="pendingAction?.()"
    />

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
      <SystemMetricsPanel :metrics="metrics" />
    </div>
  </div>
</template>
