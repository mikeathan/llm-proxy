<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'



const state = ref<any>(null)
const metrics = ref<any>(null)
const logs = ref<any>(null)
const mcpServers = ref<any[]>([])
const logLevel = ref<string>('INFO')
const newMcpServer = ref({ name: '', url: '' })

// Polling interval
let pollInterval: any = null

const activeModel = computed(() => state.value?.active)

const availableModels = computed(() => state.value?.available || [])

// UI State

const newModel = ref<any>({ name: '', filename: '', port: 0, args: '' })
const activeTab = ref('dashboard') // dashboard, settings, logs

// Configuration edit state
const editConfig = ref<any>({
  model_dir: '',
  model_host: '',
  llama_binary: '',
  service_client_id: '',
  service_client_secret: ''
})


async function fetchLogLevel() {
  try {
    const res = await fetch('/admin/api/log-level')
    if (res.ok) {
      const data = await res.json()
      logLevel.value = data.level || 'INFO'
    }
  } catch (e) {
    console.error(e)
  }
}

async function updateLogLevel(level: string) {
  try {
    await fetch('/admin/api/log-level', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ level })
    })
    logLevel.value = level
  } catch (e) {
    console.error(e)
  }
}
async function fetchState() {
  try {
    const res = await fetch('/admin/api/state?available=1')
    state.value = await res.json()
    if (!editConfig.value.model_dir && state.value?.config) {
      editConfig.value = { ...state.value.config }
    }
  } catch (e) {
    console.error(e)
  }
}

async function fetchMetrics() {
  try {
    const res = await fetch('/admin/api/metrics')
    metrics.value = await res.json()
  } catch (e) {
    console.error(e)
  }
}

async function fetchLogs() {
  try {
    const res = await fetch('/admin/api/logs')
    logs.value = await res.json()
  } catch (e) {
    console.error(e)
  }
}

async function fetchMCPServers() {
  try {
    const res = await fetch('/admin/api/mcp')
    if (res.ok) {
      mcpServers.value = await res.json() || []
    }
  } catch (e) {
    console.error(e)
  }
}

async function startModel(name: string) {
  try {
    await fetch('/admin/api/start', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name })
    })
    await fetchState()
  } catch (e) {
    console.error(e)
  }
}

async function stopModel() {
  try {
    await fetch('/admin/api/stop', { method: 'POST' })
    await fetchState()
  } catch (e) {
    console.error(e)
  }
}

async function addModel() {
  if (!newModel.value.name || !newModel.value.filename) return
  try {
    await fetch('/admin/api/models', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        ...newModel.value,
        args: newModel.value.args.split(' ').filter((a: string) => a)
      })
    })
    newModel.value = { name: '', filename: '', port: 0, args: '' }
    await fetchState()
  } catch (e) {
    console.error(e)
  }
}

async function removeModel(name: string) {
  if (!confirm(`Are you sure you want to remove ${name}?`)) return
  try {
    await fetch(`/admin/api/models?name=${encodeURIComponent(name)}`, { method: 'DELETE' })
    await fetchState()
  } catch (e) {
    console.error(e)
  }
}

async function updateConfig() {
  try {
    await fetch('/admin/api/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(editConfig.value)
    })
    await fetchState()
    alert('Configuration saved')
  } catch (e) {
    console.error(e)
  }
}

async function addMCPServer() {
  if (!newMcpServer.value.name || !newMcpServer.value.url) return

  try {
    const res = await fetch('/admin/api/mcp', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(newMcpServer.value)
    })
    if (res.ok) {
      newMcpServer.value = { name: '', url: '' }
      await fetchMCPServers()
    } else {
      console.error(await res.text())
    }
  } catch (e) {
    console.error(e)
  }
}

async function removeMCPServer(name: string) {
  try {
    const res = await fetch(`/admin/api/mcp?name=${encodeURIComponent(name)}`, {
      method: 'DELETE'
    })
    if (res.ok) {
      await fetchMCPServers()
    } else {
      console.error(await res.text())
    }
  } catch (e) {
    console.error(e)
  }
}

async function toggleMCPServer(server: any) {
  try {
    const res = await fetch('/admin/api/mcp', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ...server, enabled: !server.enabled })
    })
    if (res.ok) {
      await fetchMCPServers()
    } else {
      console.error(await res.text())
    }
  } catch (e) {
    console.error(e)
  }
}

function selectAvailableModel(model: any) {
  newModel.value = {
    name: model.name,
    filename: model.filename,
    port: state.value?.next_port || 8080,
    args: ''
  }
}

onMounted(() => {
  fetchState()
  fetchMetrics()
  fetchLogs()
  fetchMCPServers()
  fetchLogLevel()
  pollInterval = setInterval(() => {
    fetchState()
    fetchMetrics()
    fetchLogs()
  }, 5000)
})

onUnmounted(() => {
  if (pollInterval) clearInterval(pollInterval)
})


</script>

<template>
  <div class="min-h-screen bg-gray-900 text-gray-300 font-sans">
    <header class="bg-gray-800 border-b border-gray-700 p-4 sticky top-0 z-10">
      <div class="max-w-7xl mx-auto flex flex-col md:flex-row justify-between items-center gap-4">
        <h1 class="text-xl font-bold text-white flex items-center gap-2">
          <svg class="w-6 h-6 text-blue-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"></path></svg>
          LLM Proxy Admin
        </h1>

        <nav class="flex gap-2">
          <button @click="activeTab = 'dashboard'" :class="['px-4 py-2 rounded-md text-sm font-medium transition-colors', activeTab === 'dashboard' ? 'bg-blue-600 text-white' : 'bg-gray-800 text-gray-400 hover:bg-gray-700']">Dashboard</button>
          <button @click="activeTab = 'settings'" :class="['px-4 py-2 rounded-md text-sm font-medium transition-colors', activeTab === 'settings' ? 'bg-blue-600 text-white' : 'bg-gray-800 text-gray-400 hover:bg-gray-700']">Settings</button>
          <button @click="activeTab = 'logs'" :class="['px-4 py-2 rounded-md text-sm font-medium transition-colors', activeTab === 'logs' ? 'bg-blue-600 text-white' : 'bg-gray-800 text-gray-400 hover:bg-gray-700']">Process Logs</button>
        </nav>
      </div>
    </header>

    <main class="max-w-7xl mx-auto p-4 md:p-6 space-y-6">
      <div v-if="!state" class="flex justify-center items-center py-20">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500"></div>
      </div>

      <!-- DASHBOARD TAB -->
      <template v-else-if="activeTab === 'dashboard'">
        <!-- System Status Header -->
        <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
          <!-- Active Model Status -->
          <div class="bg-gray-800 rounded-lg shadow border border-gray-700 p-5 flex flex-col">
            <h3 class="text-sm font-medium text-gray-400 uppercase tracking-wider mb-2">Active Model</h3>
            <div v-if="activeModel" class="flex flex-col gap-2">
              <div class="flex items-center gap-2">
                <span :class="['w-3 h-3 rounded-full', activeModel.ready ? 'bg-green-500' : 'bg-yellow-500 animate-pulse']"></span>
                <span class="font-bold text-white text-lg truncate" :title="activeModel.name">{{ activeModel.name }}</span>
              </div>
              <div class="text-xs text-gray-400">{{ activeModel.endpoint }}</div>
              <button @click="stopModel" class="mt-auto px-3 py-1.5 bg-red-600 hover:bg-red-700 text-white text-xs font-medium rounded self-start transition-colors">Stop Model</button>
            </div>
            <div v-else class="text-gray-500 italic mt-2">No active model running</div>
          </div>

          <!-- Host Metrics -->
          <div class="bg-gray-800 rounded-lg shadow border border-gray-700 p-5">
            <h3 class="text-sm font-medium text-gray-400 uppercase tracking-wider mb-2">Host Metrics</h3>
            <div v-if="metrics" class="space-y-3">
              <div>
                <div class="flex justify-between text-xs mb-1">
                  <span>CPU Load</span>
                  <span class="text-white">{{ metrics.load_percent?.toFixed(1) || 0 }}%</span>
                </div>
                <div class="w-full bg-gray-700 rounded-full h-1.5">
                  <div class="bg-blue-500 h-1.5 rounded-full" :style="{ width: `${metrics.load_percent || 0}%` }"></div>
                </div>
              </div>
              <div>
                <div class="flex justify-between text-xs mb-1">
                  <span>Memory</span>
                  <span class="text-white">{{ (metrics.mem_used_mb/1024).toFixed(1) }} / {{ (metrics.mem_total_mb/1024).toFixed(1) }} GB</span>
                </div>
                <div class="w-full bg-gray-700 rounded-full h-1.5">
                  <div class="bg-blue-500 h-1.5 rounded-full" :style="{ width: `${(metrics.mem_used_mb / metrics.mem_total_mb) * 100}%` }"></div>
                </div>
              </div>
            </div>
          </div>

          <!-- GPU Metrics -->
          <div class="bg-gray-800 rounded-lg shadow border border-gray-700 p-5">
            <h3 class="text-sm font-medium text-gray-400 uppercase tracking-wider mb-2">GPU Status</h3>
            <div v-if="metrics?.gpu" class="space-y-3">
              <div>
                <div class="flex justify-between text-xs mb-1">
                  <span>VRAM ({{ metrics.gpu.name || metrics.gpu.vendor }})</span>
                  <span class="text-white">{{ (metrics.gpu.memory_used_mb/1024).toFixed(1) }} / {{ (metrics.gpu.memory_total_mb/1024).toFixed(1) }} GB</span>
                </div>
                <div class="w-full bg-gray-700 rounded-full h-1.5">
                  <div class="bg-purple-500 h-1.5 rounded-full" :style="{ width: `${metrics.gpu.memory_utilization_percent}%` }"></div>
                </div>
              </div>
              <div class="flex justify-between text-xs mt-2">
                <span>Core Utilization</span>
                <span class="text-white">{{ metrics.gpu.utilization_percent }}%</span>
              </div>
              <div class="flex justify-between text-xs">
                <span>Temperature</span>
                <span :class="{'text-red-400': metrics.gpu.temperature_c > 80, 'text-white': metrics.gpu.temperature_c <= 80}">{{ metrics.gpu.temperature_c }}°C</span>
              </div>
            </div>
            <div v-else class="text-gray-500 text-sm mt-2">
              <span v-if="metrics?.gpu_error">{{ metrics.gpu_error }}</span>
              <span v-else>No GPU detected</span>
            </div>
          </div>

          <!-- LLM Throughput -->
          <div class="bg-gray-800 rounded-lg shadow border border-gray-700 p-5 flex flex-col justify-center items-center">
            <h3 class="text-sm font-medium text-gray-400 uppercase tracking-wider mb-2 self-start w-full">Throughput</h3>
            <div class="text-4xl font-bold text-white tracking-tight">
              {{ metrics?.llm_tokens_per_sec?.toFixed(1) || '0.0' }}
            </div>
            <div class="text-xs text-gray-500 mt-1">tokens / sec</div>
          </div>
        </div>

        <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <!-- Configured Models -->
          <div class="bg-gray-800 rounded-lg shadow border border-gray-700 p-5 flex flex-col h-[500px]">
            <h2 class="text-lg font-semibold text-white mb-4 flex justify-between items-center">
              Configured Models
              <span class="text-xs bg-gray-700 px-2 py-1 rounded text-gray-300">{{ state.models.length }} Models</span>
            </h2>
            <div class="overflow-y-auto flex-1 pr-2">
              <div v-if="state.models.length === 0" class="text-center text-gray-500 py-10">
                No models configured. Select an available model below to add one.
              </div>
              <div class="space-y-3">
                <div v-for="model in state.models" :key="model.name"
                     :class="['p-4 rounded-lg border transition-colors flex items-center justify-between', model.active ? 'bg-blue-900/20 border-blue-500/50' : 'bg-gray-900/50 border-gray-700 hover:border-gray-600']">
                  <div>
                    <div class="font-medium text-white flex items-center gap-2">
                      {{ model.name }}
                      <span v-if="model.active" class="px-2 py-0.5 text-[10px] uppercase font-bold rounded-full bg-blue-500 text-white">Active</span>
                    </div>
                    <div class="text-xs text-gray-500 mt-1">Port: {{ model.port }} &bull; File: {{ model.filename }}</div>
                  </div>
                  <div class="flex gap-2">
                    <button v-if="!model.active" @click="startModel(model.name)" class="px-3 py-1.5 bg-gray-700 hover:bg-gray-600 text-white text-xs font-medium rounded transition-colors">Start</button>
                    <button @click="removeModel(model.name)" class="px-3 py-1.5 border border-red-500/30 text-red-400 hover:bg-red-500/10 text-xs font-medium rounded transition-colors" :disabled="model.active">Remove</button>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Add / Discovered Models -->
          <div class="bg-gray-800 rounded-lg shadow border border-gray-700 p-5 flex flex-col h-[500px]">
            <h2 class="text-lg font-semibold text-white mb-4">Add Model</h2>

            <form @submit.prevent="addModel" class="mb-6 space-y-3 p-4 bg-gray-900/50 rounded border border-gray-700">
              <div class="grid grid-cols-2 gap-3">
                <div>
                  <label class="block text-xs font-medium text-gray-400 mb-1">Name</label>
                  <input v-model="newModel.name" type="text" required placeholder="e.g. qwen2.5" class="w-full bg-gray-800 border border-gray-600 rounded px-3 py-1.5 text-sm text-white focus:border-blue-500 focus:outline-none">
                </div>
                <div>
                  <label class="block text-xs font-medium text-gray-400 mb-1">Filename</label>
                  <input v-model="newModel.filename" type="text" required placeholder="e.g. model.gguf" class="w-full bg-gray-800 border border-gray-600 rounded px-3 py-1.5 text-sm text-white focus:border-blue-500 focus:outline-none">
                </div>
              </div>
              <div class="grid grid-cols-3 gap-3">
                <div class="col-span-1">
                  <label class="block text-xs font-medium text-gray-400 mb-1">Port</label>
                  <input v-model.number="newModel.port" type="number" class="w-full bg-gray-800 border border-gray-600 rounded px-3 py-1.5 text-sm text-white focus:border-blue-500 focus:outline-none">
                </div>
                <div class="col-span-2">
                  <label class="block text-xs font-medium text-gray-400 mb-1">Extra Args (space separated)</label>
                  <input v-model="newModel.args" type="text" placeholder="-c 4096 --ngl 99" class="w-full bg-gray-800 border border-gray-600 rounded px-3 py-1.5 text-sm text-white focus:border-blue-500 focus:outline-none">
                </div>
              </div>
              <button type="submit" class="w-full bg-blue-600 hover:bg-blue-500 text-white py-2 rounded text-sm font-medium transition-colors mt-2">Add to Configuration</button>
            </form>

            <h3 class="text-sm font-semibold text-gray-300 mb-3">Discovered in Directory</h3>
            <div class="overflow-y-auto flex-1 pr-2">
              <div v-if="availableModels.length === 0" class="text-center text-gray-500 py-4 text-sm">
                No new .gguf files found in model directory.
              </div>
              <div class="space-y-2">
                <div v-for="model in availableModels" :key="model.filename" class="p-3 bg-gray-700/30 rounded border border-gray-700 flex justify-between items-center group hover:bg-gray-700/50 transition-colors">
                  <div class="truncate mr-4">
                    <div class="text-sm text-white font-medium truncate" :title="model.name">{{ model.name }}</div>
                    <div class="text-xs text-gray-500 truncate" :title="model.filename">{{ model.filename }}</div>
                  </div>
                  <button @click="selectAvailableModel(model)" class="px-2.5 py-1 bg-gray-600 hover:bg-gray-500 text-white text-xs rounded transition-colors opacity-0 group-hover:opacity-100 focus:opacity-100">Select</button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </template>

      <!-- SETTINGS TAB -->
      <template v-else-if="activeTab === 'settings'">
        <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <!-- General Settings -->
          <div class="bg-gray-800 rounded-lg shadow border border-gray-700 p-6 space-y-4">
            <h2 class="text-lg font-semibold text-white mb-4 border-b border-gray-700 pb-2">Global Settings</h2>

            <form @submit.prevent="updateConfig" class="space-y-4">
              <div>
                <label class="block text-sm font-medium text-gray-300 mb-1">Model Directory</label>
                <div class="text-xs text-gray-500 mb-1">Absolute or relative path to scan for .gguf files</div>
                <input v-model="editConfig.model_dir" type="text" class="w-full bg-gray-900 border border-gray-600 rounded px-3 py-2 text-white focus:border-blue-500 focus:outline-none">
              </div>

              <div>
                <label class="block text-sm font-medium text-gray-300 mb-1">Llama Binary Path</label>
                <div class="text-xs text-gray-500 mb-1">Path to llama-server executable</div>
                <input v-model="editConfig.llama_binary" type="text" class="w-full bg-gray-900 border border-gray-600 rounded px-3 py-2 text-white focus:border-blue-500 focus:outline-none">
              </div>

              <div>
                <label class="block text-sm font-medium text-gray-300 mb-1">Model Host IP</label>
                <div class="text-xs text-gray-500 mb-1">IP address the underlying server binds to (default: 127.0.0.1)</div>
                <input v-model="editConfig.model_host" type="text" class="w-full bg-gray-900 border border-gray-600 rounded px-3 py-2 text-white focus:border-blue-500 focus:outline-none">
              </div>


              <div class="pt-2 border-t border-gray-700 mt-2">
                <label class="block text-sm font-medium text-gray-300 mb-1">System Log Level</label>
                <div class="text-xs text-gray-500 mb-2">Change the verbosity of proxy logging in the terminal</div>
                <div class="flex gap-2">
                  <button type="button" @click="updateLogLevel('DEBUG')" :class="['px-3 py-1.5 rounded text-xs font-medium transition-colors', logLevel === 'DEBUG' ? 'bg-blue-600 text-white' : 'bg-gray-700 text-gray-300 hover:bg-gray-600']">DEBUG</button>
                  <button type="button" @click="updateLogLevel('INFO')" :class="['px-3 py-1.5 rounded text-xs font-medium transition-colors', logLevel === 'INFO' ? 'bg-blue-600 text-white' : 'bg-gray-700 text-gray-300 hover:bg-gray-600']">INFO</button>
                  <button type="button" @click="updateLogLevel('WARN')" :class="['px-3 py-1.5 rounded text-xs font-medium transition-colors', logLevel === 'WARN' ? 'bg-blue-600 text-white' : 'bg-gray-700 text-gray-300 hover:bg-gray-600']">WARN</button>
                  <button type="button" @click="updateLogLevel('ERROR')" :class="['px-3 py-1.5 rounded text-xs font-medium transition-colors', logLevel === 'ERROR' ? 'bg-blue-600 text-white' : 'bg-gray-700 text-gray-300 hover:bg-gray-600']">ERROR</button>
                </div>
              </div>

              <div class="pt-4 border-t border-gray-700 mt-4">
                <button type="submit" class="bg-blue-600 hover:bg-blue-500 text-white px-4 py-2 rounded font-medium transition-colors">Save Settings</button>
              </div>
            </form>
          </div>

          <!-- MCP Servers -->
          <div class="bg-gray-800 rounded-lg shadow border border-gray-700 p-6 flex flex-col">
            <h2 class="text-lg font-semibold text-white mb-4 border-b border-gray-700 pb-2">MCP Servers</h2>

            <div class="mb-5 flex gap-2">
              <input v-model="newMcpServer.name" type="text" placeholder="Server Name" class="bg-gray-900 border border-gray-600 text-white rounded px-3 py-2 flex-1 text-sm focus:outline-none focus:border-blue-500">
              <input v-model="newMcpServer.url" type="text" placeholder="Server URL" class="bg-gray-900 border border-gray-600 text-white rounded px-3 py-2 flex-1 text-sm focus:outline-none focus:border-blue-500">
              <button @click="addMCPServer" class="bg-blue-600 hover:bg-blue-500 text-white px-4 py-2 rounded text-sm font-medium transition-colors">Add</button>
            </div>

            <div class="overflow-y-auto flex-1 border border-gray-700 rounded-lg">
              <table class="w-full text-left text-sm">
                <thead class="bg-gray-900/80 sticky top-0">
                  <tr class="text-gray-400 border-b border-gray-700">
                    <th class="px-4 py-2 font-medium">Name</th>
                    <th class="px-4 py-2 font-medium">URL</th>
                    <th class="px-4 py-2 font-medium text-right">Actions</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-700">
                  <tr v-if="mcpServers.length === 0">
                    <td colspan="3" class="px-4 py-6 text-center text-gray-500">No MCP servers configured</td>
                  </tr>
                  <tr v-for="server in mcpServers" :key="server.name" class="hover:bg-gray-700/30">
                    <td class="px-4 py-3 text-white font-medium flex items-center gap-2">
                      {{ server.name }}
                      <button @click="toggleMCPServer(server)" :class="['w-2 h-2 rounded-full cursor-pointer', server.enabled ? 'bg-green-500' : 'bg-gray-600']" :title="server.enabled ? 'Enabled' : 'Disabled'"></button>
                    </td>
                    <td class="px-4 py-3 text-gray-400 font-mono text-xs">{{ server.url }}</td>
                    <td class="px-4 py-3 text-right">
                      <button @click="removeMCPServer(server.name)" class="text-red-400 hover:text-red-300 text-xs">Delete</button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </template>

      <!-- LOGS TAB -->
      <template v-else-if="activeTab === 'logs'">
        <div class="bg-gray-800 rounded-lg shadow border border-gray-700 p-0 flex flex-col h-[700px] overflow-hidden">
          <div class="p-4 border-b border-gray-700 flex justify-between items-center bg-gray-800 z-10">
            <h2 class="text-lg font-semibold text-white flex items-center gap-2">
              Model Process Logs
              <span v-if="logs?.running" class="flex h-2 w-2 relative">
                <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"></span>
                <span class="relative inline-flex rounded-full h-2 w-2 bg-green-500"></span>
              </span>
            </h2>
            <div class="flex gap-3 text-sm">
              <span v-if="logs?.running" class="text-gray-400">Model: <span class="text-white">{{ logs?.name }}</span></span>
              <span v-else class="text-yellow-500">Process stopped</span>
            </div>
          </div>

          <div class="flex-1 bg-[#1e1e1e] p-4 overflow-y-auto font-mono text-xs text-gray-300 leading-relaxed whitespace-pre-wrap select-text break-words">
            {{ logs?.logs || 'No logs available.' }}
          </div>
        </div>
      </template>

    </main>
  </div>
</template>
