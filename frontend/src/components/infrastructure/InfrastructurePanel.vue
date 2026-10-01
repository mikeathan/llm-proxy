<script setup lang="ts">
import { useProcesses } from '../../composables/automation/useProcesses'
import { AdminApiService } from '../../services/admin/adminService'
import { useModels } from '../../composables/models/useModels'
import { useConfirm } from '../../composables/ui/useConfirm'
import { useToast } from '../../composables/useToast'
import { errorMessage } from '../../utils/errors'
import type { ProcessInfo } from '../../types/admin'
import type { DataTableColumn } from '../../types/ui'
import Panel from '../common/layout/Panel.vue'
import DataTable from '../common/display/DataTable.vue'
import StatusTag from '../common/display/StatusTag.vue'
import BaseButton from '../common/buttons/BaseButton.vue'

// Settings · Model processes: llama-server processes on this machine, polled
// while the section is showing. Stopping an orphan frees GPU memory.
const NO_VALUE = '—'

const { processes, loaded, error, refresh } = useProcesses()
const { refresh: refreshModels } = useModels()
const { confirm } = useConfirm()
const toast = useToast()

const processLabel = (p: ProcessInfo) => `process ${p.pid}${p.model ? ` (${p.model})` : ''}`

const handleKill = async (p: ProcessInfo) => {
  const ok = await confirm({
    title: `Stop ${processLabel(p)}?`,
    message: p.active
      ? 'This is the running model: requests to it fail until it is started again.'
      : 'The orphaned process is killed and its GPU memory freed.',
    confirmText: 'Stop',
    type: 'error',
  })
  if (!ok) return

  try {
    await AdminApiService.stopProcess(p.pid)
    toast.success('Process stopped')
    await refresh()
    await refreshModels()
  } catch (e) {
    toast.error(`Could not stop ${processLabel(p)}: ${errorMessage(e)}`)
  }
}

const COLUMNS: DataTableColumn<ProcessInfo>[] = [
  { key: 'status', label: 'Status' },
  { key: 'pid', label: 'PID', numeric: true, value: (p) => p.pid },
  { key: 'model', label: 'Model', value: (p) => p.model || NO_VALUE },
  { key: 'port', label: 'Port', numeric: true, value: (p) => (p.port ? `:${p.port}` : NO_VALUE) },
  { key: 'uptime', label: 'Uptime', numeric: true, value: (p) => p.uptime },
  { key: 'actions', label: 'Actions' },
]
</script>

<template>
  <Panel title="Local model processes" flush>
    <p class="m-0 border-b border-hairline px-4 py-3 text-[length:var(--text-small)] text-muted">
      llama-server processes on this machine. Stopping an orphaned process frees GPU memory without affecting the proxy.
    </p>
    <DataTable
      :columns="COLUMNS"
      :rows="processes"
      :row-key="(p) => String(p.pid)"
      caption="Model processes"
      :loading="!loaded"
      :error="processes.length ? null : error || null"
      error-next="The list refreshes every 10 seconds; check the app log if it keeps failing."
      empty-title="No model processes running"
      empty-body="Start a local model from Models to see its llama-server here."
    >
      <template #cell-status="{ row }">
        <StatusTag :state="row.active ? 'success' : 'running'" :label="row.active ? 'Managed' : 'Orphan'" />
      </template>
      <template #cell-actions="{ row }">
        <BaseButton variant="ghost" size="sm" icon="stop" icon-only :label="`Stop ${processLabel(row)}`" @click="handleKill(row)" />
      </template>
    </DataTable>
  </Panel>
</template>
