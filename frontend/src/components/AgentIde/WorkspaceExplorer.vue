<script setup lang="ts">
import { ref } from 'vue'
import InlineConfirm from '../ui/InlineConfirm.vue'

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
  <div class="flex flex-col h-full bg-gray-900/20">
    <!-- New Workspace Action Bar -->
    <div class="p-4 border-b border-white/5 bg-gray-900/40">
      <div class="relative group">
        <input 
          v-model="newWorkspaceName" 
          placeholder="Create new workspace..." 
          class="w-full bg-black/40 text-[11px] text-gray-200 pl-3 pr-10 py-2 rounded-lg border border-white/10 focus:outline-none focus:border-blue-500/50 focus:ring-1 focus:ring-blue-500/20 transition-all placeholder:text-gray-600" 
          @keyup.enter="handleCreateWorkspace" 
        />
        <button 
          @click="handleCreateWorkspace" 
          class="absolute right-1 top-1 bottom-1 px-3 bg-blue-600 hover:bg-blue-500 text-white rounded-md transition-colors flex items-center justify-center shadow-lg active:scale-95"
          title="Create Workspace"
        >
          <span class="text-sm font-bold">+</span>
        </button>
      </div>
    </div>
    
    <div v-if="loading" class="p-4 text-gray-500 text-sm">Loading...</div>
    <div v-else>
      <div v-for="ws in workspaces" :key="ws.id" class="border-b border-gray-750">
        <div class="group">
          <button
            @click="emit('select-workspace', ws.id)"
            class="w-full px-4 py-2.5 text-left text-sm text-gray-200 hover:bg-gray-750 flex justify-between items-center"
          >
            <span class="font-medium">📁 {{ ws.id }}</span>
            <div class="flex items-center gap-2">
              <button
                v-if="confirmingDeleteWs !== ws.id"
                @click.stop="confirmingDeleteWs = ws.id"
                class="text-red-400 hover:text-red-300 text-xs opacity-0 group-hover:opacity-100"
                title="Delete workspace"
              >
                🗑️
              </button>
              <span class="text-xs text-gray-500">{{ selectedWorkspace === ws.id ? '▼' : '▶' }}</span>
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
        
        <div v-if="selectedWorkspace === ws.id && confirmingDeleteWs !== ws.id" class="bg-gray-900/50 pb-2">
          <!-- New File Action Bar -->
          <div class="px-4 py-3">
            <div class="relative group">
              <input 
                v-model="newFileName" 
                placeholder="Create new file..." 
                class="w-full bg-black/40 text-[10px] text-gray-300 pl-3 pr-8 py-1.5 rounded-md border border-white/5 focus:outline-none focus:border-blue-500/50 transition-all placeholder:text-gray-700 font-mono" 
                @keyup.enter="handleCreateFile(ws.id)" 
              />
              <button 
                @click="handleCreateFile(ws.id)" 
                class="absolute right-0.5 top-0.5 bottom-0.5 px-2 bg-gray-700 hover:bg-gray-600 text-gray-200 rounded-sm transition-colors flex items-center justify-center shadow-lg active:scale-95"
                title="Create File"
              >
                <span class="text-xs font-bold">+</span>
              </button>
            </div>
          </div>
          
          <div v-for="file in workspaceFiles[ws.id]" :key="file">
            <div
              class="group w-full px-8 py-1.5 text-left text-xs transition-colors flex items-center justify-between"
            >
              <button
                @click="emit('open-file', ws.id, file)"
                :class="[
                  'flex-1 flex items-center gap-2',
                  selectedFile?.workspace === ws.id && selectedFile?.filename === file
                    ? 'text-white'
                    : 'text-gray-400 hover:text-gray-200'
                ]"
              >
                <span>{{ file.endsWith('.md') ? '📝' : '📄' }}</span>
                {{ file }}
              </button>
              <button
                v-if="confirmingDeleteFile?.file !== file || confirmingDeleteFile?.ws !== ws.id"
                @click.stop="confirmingDeleteFile = { ws: ws.id, file }"
                class="opacity-0 group-hover:opacity-100 text-red-400 hover:text-red-300 px-1"
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
