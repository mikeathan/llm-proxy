<script setup lang="ts">
import { ref } from 'vue'

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
  <div>
    <div class="p-3 border-b border-gray-750 flex gap-2">
      <input 
        v-model="newWorkspaceName" 
        placeholder="New workspace name" 
        class="flex-1 bg-gray-900 text-xs text-white px-2 py-1.5 rounded border border-gray-700 focus:outline-none focus:border-blue-500" 
        @keyup.enter="handleCreateWorkspace" 
      />
      <button 
        @click="handleCreateWorkspace" 
        class="bg-blue-600 hover:bg-blue-700 text-white px-2 py-1.5 rounded text-xs"
      >
        +
      </button>
    </div>
    
    <div v-if="loading" class="p-4 text-gray-500 text-sm">Loading...</div>
    <div v-else>
      <div v-for="ws in workspaces" :key="ws.id" class="group border-b border-gray-750">
        <button
          @click="emit('select-workspace', ws.id)"
          class="w-full px-4 py-2.5 text-left text-sm text-gray-200 hover:bg-gray-750 flex justify-between items-center"
        >
          <span class="font-medium">📁 {{ ws.id }}</span>
          <div class="flex items-center gap-2">
            <button
              @click.stop="emit('delete-workspace', ws.id)"
              class="text-red-400 hover:text-red-300 text-xs opacity-0 group-hover:opacity-100"
              title="Delete workspace"
            >
              🗑️
            </button>
            <span class="text-xs text-gray-500">{{ selectedWorkspace === ws.id ? '▼' : '▶' }}</span>
          </div>
        </button>
        
        <div v-if="selectedWorkspace === ws.id" class="bg-gray-900/50 pb-2">
          <div class="px-4 py-2 flex gap-2">
            <input 
              v-model="newFileName" 
              placeholder="New file name" 
              class="flex-1 bg-gray-800 text-xs text-white px-2 py-1 rounded border border-gray-700 focus:outline-none focus:border-blue-500" 
              @keyup.enter="handleCreateFile(ws.id)" 
            />
            <button 
              @click="handleCreateFile(ws.id)" 
              class="bg-blue-600 hover:bg-blue-700 text-white px-2 py-1 rounded text-xs"
            >
              +
            </button>
          </div>
          
          <div
            v-for="file in workspaceFiles[ws.id]"
            :key="file"
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
              @click.stop="emit('delete-file', ws.id, file)"
              class="opacity-0 group-hover:opacity-100 text-red-400 hover:text-red-300 px-1"
              title="Delete file"
            >
              ×
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
