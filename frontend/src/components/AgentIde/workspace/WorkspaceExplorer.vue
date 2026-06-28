<script setup lang="ts">
import { ref, nextTick } from 'vue'
import InlineConfirm from '../../ui/InlineConfirm.vue'
import Icon from '../../icons/Icon.vue'

const props = defineProps<{
  workspaces: { id: string }[]
  workspaceFiles: Record<string, string[]>
  selectedWorkspace: string | null
  selectedFile: { workspace: string, filename: string } | null
  loading: boolean
  workspaceExternalAccess?: Record<string, boolean>
  chatActive?: boolean
  memoryActive?: boolean
}>()

const emit = defineEmits<{
  (e: 'select-workspace', id: string): void
  (e: 'create-workspace', name: string): void
  (e: 'delete-workspace', id: string): void
  (e: 'open-file', workspace: string, filename: string): void
  (e: 'create-file', workspace: string, filename: string): void
  (e: 'delete-file', workspace: string, filename: string): void
  (e: 'manage-guardrails', id: string): void
  (e: 'open-memory'): void
  (e: 'open-playbooks'): void
  (e: 'open-chat'): void
}>()

const newWorkspaceName = ref('')
const newFileName = ref('')
const newWorkspaceOpen = ref(false)
const newWsInput = ref<HTMLInputElement | null>(null)

const confirmingDeleteWs = ref<string | null>(null)
const confirmingDeleteFile = ref<{ ws: string, file: string } | null>(null)

const toggleNewWorkspace = () => {
  newWorkspaceOpen.value = !newWorkspaceOpen.value
  if (newWorkspaceOpen.value) {
    nextTick(() => newWsInput.value?.focus())
  }
}

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
  newWorkspaceOpen.value = false
}

const handleCreateFile = (workspace: string) => {
  if (!newFileName.value) return
  emit('create-file', workspace, newFileName.value)
  newFileName.value = ''
}
</script>

<template>
  <div class="explorer-shell">
    <!-- Section Label + Actions -->
    <div class="explorer-section-row">
      <div class="explorer-section-left">
        <span class="explorer-section-label">EXPLORER</span>
      </div>
      <div class="explorer-section-actions">
        <button @click="emit('open-memory')" :disabled="!selectedWorkspace" class="action-pill" :class="{ 'action-pill--active': memoryActive }" title="Memory">
          <Icon name="search" size="sm" />
          <span>Memory</span>
        </button>
        <button @click="emit('open-playbooks')" :disabled="!selectedWorkspace" class="action-pill" title="Playbooks">
          <Icon name="document" size="sm" />
          <span>Playbooks</span>
        </button>
        <button @click="emit('open-chat')" :disabled="!selectedWorkspace" class="action-pill" :class="{ 'action-pill--active': chatActive }" title="Chat">
          <Icon name="lightning" size="sm" />
          <span>Chat</span>
        </button>
        <div class="action-sep"></div>
        <button @click="toggleNewWorkspace" class="action-pill action-pill--add" :class="{ 'action-pill--active': newWorkspaceOpen }" title="New workspace">
          <Icon name="plus" size="sm" />
          <span>New</span>
        </button>
      </div>
    </div>

    <!-- Collapsible New Workspace Input -->
    <Transition name="slide-down">
      <div v-if="newWorkspaceOpen" class="action-bar">
        <div class="input-wrapper group">
          <input
            ref="newWsInput"
            v-model="newWorkspaceName"
            placeholder="New workspace name..."
            class="action-input"
            @keyup.enter="handleCreateWorkspace"
            @blur="newWorkspaceOpen = false"
          />
          <button @click="handleCreateWorkspace" class="btn-add-action" title="Create Workspace">
            <Icon name="plus" size="sm" />
          </button>
        </div>
      </div>
    </Transition>
    
    <!-- Loading Skeleton (Only if empty) -->
    <div v-if="loading && workspaces.length === 0" class="loading-state">
      <div class="skeleton-row animate-pulse" v-for="i in 3" :key="i"></div>
    </div>
    
    <div v-else class="explorer-content" :class="{ 'opacity-60 pointer-events-none': loading }">
      <div v-for="ws in workspaces" :key="ws.id" class="workspace-item" :class="{ 'workspace-item--active': selectedWorkspace === ws.id }">
        <div class="group relative">
          <button
            @click="emit('select-workspace', ws.id)"
            class="workspace-row"
          >
            <div class="flex items-center gap-2 overflow-hidden">
              <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="text-blue-400 shrink-0"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"></path></svg>
              <span class="workspace-name truncate">{{ ws.id }}</span>
            </div>
            
            <div class="row-controls">
              <!-- External Access Warning -->
              <span
                v-if="workspaceExternalAccess?.[ws.id]"
                class="external-hazard-dot"
                title="External file system access enabled"
              >⚠</span>
              <!-- Guardrails Icon -->
              <button
                @click.stop="emit('manage-guardrails', ws.id)"
                class="icon-btn icon-btn--guard"
                :class="{ 'icon-btn--guard-hazard': workspaceExternalAccess?.[ws.id] }"
                title="Workspace Guardrails"
              >
                <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"></path></svg>
              </button>

              <!-- Delete Workspace Icon -->
              <button
                v-if="confirmingDeleteWs !== ws.id"
                @click.stop="confirmingDeleteWs = ws.id"
                class="icon-btn icon-btn--danger"
                title="Delete workspace"
              >
                <Icon name="trash" size="sm" />
              </button>
              
              <svg 
                xmlns="http://www.w3.org/2000/svg" 
                width="12" height="12" 
                viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round" 
                class="text-gray-600 transition-transform duration-200"
                :class="{ 'rotate-90': selectedWorkspace === ws.id }"
              >
                <polyline points="9 18 15 12 9 6"></polyline>
              </svg>
            </div>
          </button>
        </div>
        
        <!-- Delete Confirmation -->
        <Transition name="fade-slide">
          <div v-if="confirmingDeleteWs === ws.id" class="px-2">
            <InlineConfirm
              :message="`Delete workspace '${ws.id}'?`"
              @confirm="confirmDeleteWorkspace(ws.id)"
              @cancel="confirmingDeleteWs = null"
              class="!mx-0 !my-1"
            />
          </div>
        </Transition>
        
        <!-- Files List -->
        <Transition name="expand">
          <div v-if="selectedWorkspace === ws.id && confirmingDeleteWs !== ws.id" class="file-section">
            <div class="file-action-bar">
              <div class="input-wrapper group">
                <input 
                  v-model="newFileName" 
                  placeholder="New file (e.g. data.py)" 
                  class="action-input action-input--file" 
                  @keyup.enter="handleCreateFile(ws.id)" 
                />
                <button 
                  @click="handleCreateFile(ws.id)" 
                  class="btn-add-action btn-add-action--file"
                  title="Create File"
                >
                  <Icon name="plus" size="sm" />
                </button>
              </div>
            </div>
            
            <div class="files-container">
              <div v-for="file in workspaceFiles[ws.id]" :key="file">
                <div class="file-row group">
                  <button
                    @click="emit('open-file', ws.id, file)"
                    class="btn-file-open"
                    :class="{ 'btn-file-open--selected': selectedFile?.workspace === ws.id && selectedFile?.filename === file }"
                  >
                    <Icon v-if="file.endsWith('.md')" name="document" size="sm" class="text-blue-300/60" />
                    <Icon v-else name="document" size="sm" class="text-gray-500" />
                    <span class="truncate">{{ file }}</span>
                  </button>
                  <button
                    v-if="confirmingDeleteFile?.file !== file || confirmingDeleteFile?.ws !== ws.id"
                    @click.stop="confirmingDeleteFile = { ws: ws.id, file }"
                    class="btn-file-delete"
                    title="Delete file"
                  >
                    <Icon name="close" size="sm" />
                  </button>
                </div>
                
                <Transition name="fade-slide">
                  <div v-if="confirmingDeleteFile?.file === file && confirmingDeleteFile?.ws === ws.id" class="px-2 pr-4 ml-6">
                    <InlineConfirm
                      :message="`Delete '${file}'?`"
                      @confirm="confirmDeleteFile(ws.id, file)"
                      @cancel="confirmingDeleteFile = null"
                      class="!mx-0 !my-1"
                    />
                  </div>
                </Transition>
              </div>
            </div>
          </div>
        </Transition>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.explorer-shell {
  @apply flex flex-col h-full bg-gray-900/40 text-gray-400 select-none;
}

.action-bar {
  @apply p-4 border-b border-white/5;
}

.input-wrapper {
  @apply relative;
}

.action-input {
  @apply w-full bg-white/[0.03] text-[11px] text-gray-100 pl-3 pr-10 py-2.5 rounded-lg border border-white/5 
         focus:outline-none focus:border-blue-500/40 focus:ring-4 focus:ring-blue-500/5 transition-all placeholder:text-gray-600;
}

.action-input--file {
  @apply text-[10px] text-gray-200 pr-8 py-2 rounded-md border-white/10 placeholder:text-gray-700 font-mono;
}

.btn-add-action {
  @apply absolute right-1.5 top-1.5 bottom-1.5 px-3 bg-blue-600/80 hover:bg-blue-600 text-white rounded-md 
         transition-all flex items-center justify-center shadow-lg active:scale-95;
}

.btn-add-action--file {
  @apply right-1 top-1 bottom-1 px-2.5 bg-white/10 hover:bg-white/20 text-gray-300 rounded-md;
}

.loading-state {
  @apply p-4 space-y-3;
}

.skeleton-row {
  @apply h-8 w-full bg-white/5 rounded-md;
}

.explorer-content {
  @apply px-3 py-3 space-y-2 transition-opacity duration-300;
}

.workspace-item {
  @apply rounded-2xl border border-white/5 bg-gray-800/40 transition-colors duration-200 overflow-hidden;
}

.workspace-item--active {
  @apply border-blue-500/30 bg-gray-800/60;
}

.workspace-row {
  @apply w-full px-4 py-3 text-left text-sm text-gray-300 hover:bg-white/[0.03] flex justify-between items-center transition-all;
}

.workspace-name {
  @apply font-semibold text-[13px] tracking-tight;
}

.row-controls {
  @apply flex items-center gap-1.5 ml-2;
}

.icon-btn {
  @apply p-1.5 rounded-md transition-all duration-200 opacity-0 group-hover:opacity-100 scale-90 group-hover:scale-100 hover:bg-white/10;
}

.icon-btn--guard {
  @apply text-blue-400/70 hover:text-blue-400;
}

.icon-btn--guard-hazard {
  @apply text-amber-400/80 hover:text-amber-300;
}

.external-hazard-dot {
  @apply text-[10px] opacity-0 group-hover:opacity-100 transition-opacity duration-200 cursor-help;
}

.icon-btn--danger {
  @apply text-red-400/50 hover:text-red-400 hover:bg-red-500/10;
}

.file-section {
  @apply bg-black/30 border-t border-white/5;
}

.file-action-bar {
  @apply px-4 py-3;
}

.files-container {
  @apply pb-2;
}

.file-row {
  @apply w-full px-4 ml-2 pr-4 py-2 text-left text-[11px] transition-all flex items-center justify-between hover:bg-white/[0.02] rounded-l-md;
}

.btn-file-open {
  @apply flex-1 flex items-center gap-2.5 text-gray-500 hover:text-gray-300 transition-colors min-w-0;
}

.btn-file-open--selected {
  @apply text-blue-300 font-medium;
}

.btn-file-delete {
  @apply opacity-0 group-hover:opacity-100 text-gray-600 hover:text-red-400 px-1.5 transition-all hover:scale-110;
}

/* ── Section Row ── */
.explorer-section-row {
  @apply flex items-end justify-between px-4 py-2 pb-2 border-b border-white/5;
}

.explorer-section-left {
  @apply flex items-center;
}

.explorer-section-label {
  @apply text-[9px] font-bold text-gray-500 uppercase tracking-[0.2em];
}

.explorer-section-actions {
  @apply flex items-end gap-1;
}

.action-sep {
  @apply w-px self-stretch bg-white/5 mx-0.5;
}

.action-pill {
  @apply flex flex-col items-center gap-0.5 py-1 px-1.5 rounded min-w-0
         text-gray-500 hover:text-gray-200 hover:bg-white/5 transition-all
         text-[8px] font-medium leading-tight
         disabled:opacity-25 disabled:cursor-not-allowed disabled:hover:bg-transparent disabled:hover:text-gray-500;
}

.action-pill--add {
  @apply text-gray-400 hover:text-blue-400 hover:bg-blue-600/10;
}

.action-pill--active {
  @apply text-blue-400 bg-blue-600/10;
}

/* Transitions */
.fade-slide-enter-active, .fade-slide-leave-active {
  transition: all 0.2s ease-out;
}
.fade-slide-enter-from, .fade-slide-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}

.expand-enter-active, .expand-leave-active {
  transition: all 0.25s ease-in-out;
  max-height: 500px;
  overflow: hidden;
}
.expand-enter-from, .expand-leave-to {
  max-height: 0;
  opacity: 0;
}

.slide-down-enter-active, .slide-down-leave-active {
  transition: all 150ms ease;
  overflow: hidden;
}
.slide-down-enter-from, .slide-down-leave-to {
  opacity: 0;
  transform: translateY(-8px);
  max-height: 0;
}
</style>
