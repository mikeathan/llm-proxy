<script setup lang="ts">
import { ref } from 'vue'
import InlineConfirm from '../../ui/InlineConfirm.vue'

const props = defineProps<{
  workspaces: { id: string }[]
  workspaceFiles: Record<string, string[]>
  selectedWorkspace: string | null
  selectedFile: { workspace: string, filename: string } | null
  loading: boolean
}>()

const emit = defineEmits<{
  (e: 'select-workspace', id: string): void
  (e: 'create-workspace', name: string): void
  (e: 'delete-workspace', id: string): void
  (e: 'open-file', workspace: string, filename: string): void
  (e: 'create-file', workspace: string, filename: string): void
  (e: 'delete-file', workspace: string, filename: string): void
  (e: 'manage-guardrails', id: string): void
}>()

const newWorkspaceName = ref('')
const newFileName = ref('')

const confirmingDeleteWs = ref<string | null>(null)
const confirmingDeleteFile = ref<{ ws: string, file: string } | null>(null)

const confirmDeleteWorkspace = (id: string) => {
  emit('delete-workspace', id)
  confirmingDeleteWs.value = null
}

const confirmDeleteFile = (ws: string, file: string) => {
  emit('delete-file', ws, file)
  confirmingDeleteFile.value = null
}

const handleCreateWorkspace = () => {
  if (!newWorkspaceName.value) return
  emit('create-workspace', newWorkspaceName.value)
  newWorkspaceName.value = ''
}

const handleCreateFile = (workspace: string) => {
  if (!newFileName.value) return
  emit('create-file', workspace, newFileName.value)
  newFileName.value = ''
}
</script>

<template>
  <div class="explorer-shell">
    <!-- New Workspace Action Bar -->
    <div class="action-bar">
      <div class="input-wrapper group">
        <input 
          v-model="newWorkspaceName" 
          placeholder="Create new workspace..." 
          class="action-input" 
          @keyup.enter="handleCreateWorkspace" 
        />
        <button 
          @click="handleCreateWorkspace" 
          class="btn-add-action"
          title="Create Workspace"
        >
          <span class="btn-plus-icon">+</span>
        </button>
      </div>
    </div>
    
    <div v-if="loading" class="loading-state">Loading...</div>
    <div v-else>
      <div v-for="ws in workspaces" :key="ws.id" class="workspace-item">
        <div class="group">
          <button
            @click="emit('select-workspace', ws.id)"
            class="workspace-row"
          >
            <span class="workspace-name">📁 {{ ws.id }}</span>
            <div class="row-controls">
              <button
                @click.stop="emit('manage-guardrails', ws.id)"
                class="btn-manage-row"
                title="Manage Workspace Guardrails"
              >
                🛡️
              </button>
              <button
                v-if="confirmingDeleteWs !== ws.id"
                @click.stop="confirmingDeleteWs = ws.id"
                class="btn-delete-row"
                title="Delete workspace"
              >
                🗑️
              </button>
              <span class="row-arrow">{{ selectedWorkspace === ws.id ? '▼' : '▶' }}</span>
            </div>
          </button>
        </div>
        
        <div v-if="confirmingDeleteWs === ws.id" class="px-2">
          <InlineConfirm
            :message="`Delete workspace '${ws.id}'?`"
            @confirm="confirmDeleteWorkspace(ws.id)"
            @cancel="confirmingDeleteWs = null"
            class="!mx-0 !my-1"
          />
        </div>
        
        <div v-if="selectedWorkspace === ws.id && confirmingDeleteWs !== ws.id" class="file-section">
          <!-- New File Action Bar -->
          <div class="file-action-bar">
            <div class="input-wrapper group">
              <input 
                v-model="newFileName" 
                placeholder="Create new file..." 
                class="action-input action-input--file" 
                @keyup.enter="handleCreateFile(ws.id)" 
              />
              <button 
                @click="handleCreateFile(ws.id)" 
                class="btn-add-action btn-add-action--file"
                title="Create File"
              >
                <span class="btn-plus-icon btn-plus-icon--file">+</span>
              </button>
            </div>
          </div>
          
          <div v-for="file in workspaceFiles[ws.id]" :key="file">
            <div class="file-row group">
              <button
                @click="emit('open-file', ws.id, file)"
                class="btn-file-open"
                :class="{ 'btn-file-open--selected': selectedFile?.workspace === ws.id && selectedFile?.filename === file }"
              >
                <span>{{ file.endsWith('.md') ? '📝' : '📄' }}</span>
                {{ file }}
              </button>
              <button
                v-if="confirmingDeleteFile?.file !== file || confirmingDeleteFile?.ws !== ws.id"
                @click.stop="confirmingDeleteFile = { ws: ws.id, file }"
                class="btn-file-delete"
                title="Delete file"
              >
                ×
              </button>
            </div>
            
            <div v-if="confirmingDeleteFile?.file === file && confirmingDeleteFile?.ws === ws.id" class="px-2 pr-4 ml-6">
              <InlineConfirm
                :message="`Delete '${file}'?`"
                @confirm="confirmDeleteFile(ws.id, file)"
                @cancel="confirmingDeleteFile = null"
                class="!mx-0 !my-1"
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.explorer-shell {
  @apply flex flex-col h-full bg-gray-900/20;
}

.action-bar {
  @apply p-4 border-b border-white/5 bg-gray-900/40;
}

.input-wrapper {
  @apply relative;
}

.action-input {
  @apply w-full bg-black/40 text-[11px] text-gray-200 pl-3 pr-10 py-2 rounded-lg border border-white/10 
         focus:outline-none focus:border-blue-500/50 focus:ring-1 focus:ring-blue-500/20 transition-all placeholder:text-gray-600;
}

.action-input--file {
  @apply text-[10px] text-gray-300 pr-8 py-1.5 rounded-md border-white/5 placeholder:text-gray-700 font-mono;
}

.btn-add-action {
  @apply absolute right-1 top-1 bottom-1 px-3 bg-blue-600 hover:bg-blue-500 text-white rounded-md 
         transition-colors flex items-center justify-center shadow-lg active:scale-95;
}

.btn-add-action--file {
  @apply right-0.5 top-0.5 bottom-0.5 px-2 bg-gray-700 hover:bg-gray-600 text-gray-200 rounded-sm;
}

.btn-plus-icon {
  @apply text-sm font-bold;
}

.btn-plus-icon--file {
  @apply text-xs flex items-center justify-center;
}

.loading-state {
  @apply p-4 text-gray-500 text-sm;
}

.workspace-item {
  @apply border-b border-gray-700;
}

.workspace-row {
  @apply w-full px-4 py-2.5 text-left text-sm text-gray-200 hover:bg-gray-700 flex justify-between items-center;
}

.workspace-name {
  @apply font-medium;
}

.row-controls {
  @apply flex items-center gap-2;
}

.btn-delete-row {
  @apply text-red-400 hover:text-red-300 text-xs opacity-0 group-hover:opacity-100;
}

.row-arrow {
  @apply text-xs text-gray-500;
}

.file-section {
  @apply bg-gray-900/50 pb-2;
}

.file-action-bar {
  @apply px-4 py-3;
}

.file-row {
  @apply w-full px-8 py-1.5 text-left text-xs transition-colors flex items-center justify-between;
}

.btn-file-open {
  @apply flex-1 flex items-center gap-2 text-gray-400 hover:text-gray-200;
}

.btn-file-open--selected {
  @apply text-white;
}

.btn-file-delete {
  @apply opacity-0 group-hover:opacity-100 text-red-400 hover:text-red-300 px-1;
}
</style>
