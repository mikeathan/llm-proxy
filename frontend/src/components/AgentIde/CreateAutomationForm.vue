<script setup lang="ts">
import { ref, watch } from 'vue'
import cronstrue from 'cronstrue'

const props = defineProps<{
  workspaces: { id: string }[]
  workspaceFiles: Record<string, string[]>
}>()

const emit = defineEmits<{
  (e: 'create-automation', workspace: string, data: any): void
  (e: 'fetch-files', workspace: string): void
}>()

const selectedWorkspace = ref('')

watch(selectedWorkspace, (newVal) => {
  if (newVal) {
    emit('fetch-files', newVal)
  }
})
const newAutomation = ref({ 
  name: '', 
  triggerType: 'cron', 
  triggerValue: '', 
  taskFile: '', 
  strategy: 'persistent' 
})

// Reset form data when workspace changes to avoid orphaned file selections
watch(selectedWorkspace, () => {
  newAutomation.value.taskFile = ''
})

// Friendly cron generator state
const cronType = ref('custom')
const cronEvery = ref(1)
const cronUnit = ref('hours')

watch([cronType, cronEvery, cronUnit], () => {
  if (cronType.value === 'custom') return
  
  if (cronType.value === 'every') {
    if (cronUnit.value === 'minutes') {
      newAutomation.value.triggerValue = `*/${cronEvery.value} * * * *`
    } else if (cronUnit.value === 'hours') {
      newAutomation.value.triggerValue = `0 */${cronEvery.value} * * *`
    } else if (cronUnit.value === 'days') {
      newAutomation.value.triggerValue = `0 0 */${cronEvery.value} * *`
    }
  }
})

// Clear trigger value when switching types to avoid interval displaying cron value
watch(() => newAutomation.value.triggerType, () => {
  newAutomation.value.triggerValue = ''
  if (newAutomation.value.triggerType === 'cron') {
    cronType.value = 'custom'
  }
})

const cronDescription = ref('')
watch(() => newAutomation.value.triggerValue, (newVal) => {
  if (newAutomation.value.triggerType === 'cron' && newVal) {
    try {
      cronDescription.value = cronstrue.toString(newVal)
    } catch {
      cronDescription.value = 'Invalid cron expression'
    }
  } else {
    cronDescription.value = ''
  }
})

const handleCreate = () => {
  if (!selectedWorkspace.value || !newAutomation.value.name) return
  
  emit('create-automation', selectedWorkspace.value, {
    name: newAutomation.value.name,
    trigger: { 
      type: newAutomation.value.triggerType, 
      value: newAutomation.value.triggerValue 
    },
    task_file: newAutomation.value.taskFile,
    strategy: newAutomation.value.strategy
  })
  
  // Reset form
  newAutomation.value = { 
    name: '', 
    triggerType: 'cron', 
    triggerValue: '', 
    taskFile: '', 
    strategy: 'persistent' 
  }
  cronType.value = 'custom'
}
</script>

<template>
  <div class="p-4 border-b border-gray-750 bg-gray-800">
    <div class="text-sm font-semibold text-gray-200 mb-3">Create Automation</div>
    
    <div class="space-y-3">
      <!-- Workspace Selection -->
      <div>
        <label class="block text-xs font-medium text-gray-400 mb-1">Workspace</label>
        <select 
          v-model="selectedWorkspace" 
          class="w-full bg-gray-900 text-sm text-white px-3 py-2 rounded border border-gray-700 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-colors"
        >
          <option value="" disabled>Select Workspace...</option>
          <option v-for="ws in workspaces" :key="ws.id" :value="ws.id">{{ ws.id }}</option>
        </select>
      </div>

      <!-- Container for rest of form, disabled if no workspace -->
      <div :class="{ 'opacity-50 pointer-events-none': !selectedWorkspace }" class="space-y-3 transition-opacity duration-200">
        <!-- General Info -->
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-xs font-medium text-gray-400 mb-1">Name</label>
            <input 
              v-model="newAutomation.name" 
              placeholder="e.g. daily-sync" 
              class="w-full bg-gray-900 text-sm text-white px-3 py-2 rounded border border-gray-700 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-colors" 
              :disabled="!selectedWorkspace"
            />
          </div>
          <div>
            <label class="block text-xs font-medium text-gray-400 mb-1">Task File</label>
            <select 
              v-model="newAutomation.taskFile" 
              class="w-full bg-gray-900 text-sm text-white px-3 py-2 rounded border border-gray-700 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-colors"
              :disabled="!selectedWorkspace"
            >
              <option value="" disabled>{{ selectedWorkspace ? 'Select File...' : 'Select workspace first' }}</option>
              <option 
                v-for="file in (selectedWorkspace && workspaceFiles[selectedWorkspace] ? workspaceFiles[selectedWorkspace] : [])" 
                :key="file" 
                :value="file"
              >
                {{ file }}
              </option>
            </select>
          </div>
        </div>

        <!-- Trigger Configuration -->
        <div class="bg-gray-900/50 p-3 rounded-lg border border-gray-700/50">
          <div class="flex items-center justify-between mb-3">
            <label class="text-xs font-medium text-gray-400">Trigger Setup</label>
            <select 
              v-model="newAutomation.triggerType" 
              class="bg-gray-800 text-xs text-white px-2 py-1 rounded border border-gray-700 w-32"
              :disabled="!selectedWorkspace"
            >
              <option value="cron">Schedule (Cron)</option>
              <option value="interval">Interval</option>
              <option value="manual">Manual Only</option>
            </select>
          </div>

          <div v-if="newAutomation.triggerType === 'cron'" class="space-y-3">
            <div class="flex gap-2">
              <select v-model="cronType" class="bg-gray-900 text-sm text-white px-3 py-2 rounded border border-gray-700 flex-1">
                <option value="every">Simple Frequency</option>
                <option value="custom">Custom Expression</option>
              </select>
            </div>
            
            <div v-if="cronType === 'every'" class="flex items-center gap-2">
              <span class="text-sm text-gray-400">Run every</span>
              <input type="number" v-model="cronEvery" min="1" class="w-20 bg-gray-900 text-sm text-white px-3 py-2 rounded border border-gray-700 text-center" />
              <select v-model="cronUnit" class="bg-gray-900 text-sm text-white px-3 py-2 rounded border border-gray-700 w-32">
                <option value="minutes">Minutes</option>
                <option value="hours">Hours</option>
                <option value="days">Days</option>
              </select>
            </div>

            <div>
              <input 
                v-model="newAutomation.triggerValue" 
                placeholder="* * * * *" 
                :readonly="cronType !== 'custom'"
                class="w-full bg-gray-900 font-mono text-sm text-white px-3 py-2 rounded border border-gray-700 disabled:opacity-50" 
              />
              <div class="mt-1 text-xs text-blue-400 min-h-[16px]">{{ cronDescription }}</div>
            </div>
          </div>

          <div v-else-if="newAutomation.triggerType === 'interval'">
            <input 
              v-model="newAutomation.triggerValue" 
              placeholder="e.g. 5m, 1h, 24h" 
              class="w-full bg-gray-900 text-sm text-white px-3 py-2 rounded border border-gray-700" 
            />
            <div class="mt-1 text-xs text-gray-500">Go duration format (m = minutes, h = hours)</div>
          </div>
          
          <div v-else class="text-xs text-gray-500 py-2">
            This automation will only run when triggered manually via the UI or API.
          </div>
        </div>

        <button 
          @click="handleCreate" 
          :disabled="!selectedWorkspace || !newAutomation.name || !newAutomation.taskFile || (newAutomation.triggerType !== 'manual' && !newAutomation.triggerValue)" 
          class="w-full bg-blue-600 hover:bg-blue-700 disabled:bg-gray-700 disabled:text-gray-500 disabled:cursor-not-allowed text-white py-2 rounded font-medium transition-colors"
        >
          Create Automation
        </button>
      </div>
    </div>
  </div>
</template>
