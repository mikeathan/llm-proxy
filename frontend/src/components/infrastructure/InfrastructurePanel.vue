<script setup lang="ts">
import { useProcesses } from '../../composables/automation/useProcesses'
import { AdminApiService } from '../../services/adminService'
import { useModels } from '../../composables/models/useModels'
import { useConfirm } from '../../composables/ui/useConfirm'
import { useToast } from '../../composables/useToast'

const { processes, refresh } = useProcesses()
const { refresh: refreshModels } = useModels()
const { confirm } = useConfirm()
const toast = useToast()

const handleKill = async (pid: number, model: string | undefined) => {
  const ok = await confirm({
    title: 'Stop Process',
    message: `Are you sure you want to stop process ${pid}${model ? ` (${model})` : ''}?`,
    confirmText: 'Stop',
    type: 'error',
  })
  if (!ok) return

  try {
    await AdminApiService.stopProcess(pid)
    toast.show('Process stopped', 'success')
    await refresh()
    await refreshModels()
  } catch (e: any) {
    toast.show(e.message || 'Failed to stop process', 'error')
  }
}
</script>

<template>
  <div>
    <h2 class="text-xl font-bold text-white mb-1">Local Model Processes</h2>
    <p class="text-sm text-gray-400 mb-6">
      Running llama-server processes on this machine. Stopping an orphaned
      process frees GPU memory without affecting proxy functionality.
    </p>

    <div v-if="processes.length === 0" class="text-center py-12 text-gray-500 bg-gray-800 rounded-lg">
      No local model processes running.
    </div>

    <div v-else class="bg-gray-800 rounded-lg overflow-hidden">
      <table class="w-full text-sm">
        <thead>
          <tr class="border-b border-gray-700 text-gray-400 uppercase text-xs tracking-wider">
            <th class="text-left px-4 py-3">Status</th>
            <th class="text-left px-4 py-3">PID</th>
            <th class="text-left px-4 py-3">Model</th>
            <th class="text-left px-4 py-3">Port</th>
            <th class="text-left px-4 py-3">Uptime</th>
            <th class="text-right px-4 py-3">Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in processes" :key="p.pid" class="border-b border-gray-700/50 hover:bg-gray-750">
            <td class="px-4 py-3">
              <span v-if="p.active" class="inline-flex items-center gap-1.5 text-emerald-400">
                <span class="w-2 h-2 rounded-full bg-emerald-400"></span>
                Active (Managed)
              </span>
              <span v-else class="inline-flex items-center gap-1.5 text-amber-400">
                <span class="w-2 h-2 rounded-full bg-amber-400"></span>
                Orphan
              </span>
            </td>
            <td class="px-4 py-3 font-mono text-gray-300">{{ p.pid }}</td>
            <td class="px-4 py-3">{{ p.model || '-' }}</td>
            <td class="px-4 py-3 font-mono">{{ p.port ? ':' + p.port : '-' }}</td>
            <td class="px-4 py-3 text-gray-400">{{ p.uptime }}</td>
            <td class="px-4 py-3 text-right">
              <button
                @click="handleKill(p.pid, p.model)"
                class="px-3 py-1.5 text-xs font-medium rounded bg-red-600/20 text-red-400 hover:bg-red-600/40 transition-colors"
              >
                Stop
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
