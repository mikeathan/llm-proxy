<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { AdminApiService } from '../../services/admin/adminService'
import { useToast } from '../../composables/useToast'
import { useConfirm } from '../../composables/ui/useConfirm'
import { errorMessage } from '../../utils/errors'
import { formatAbsoluteTime, formatRelativeTime } from '../../utils/format/time'
import type { TerminalSessionView } from '../../types/admin'
import type { DataTableColumn } from '../../types/ui'
import Panel from '../common/layout/Panel.vue'
import DataTable from '../common/display/DataTable.vue'
import StatusTag from '../common/display/StatusTag.vue'
import BaseButton from '../common/buttons/BaseButton.vue'

// The persistent host terminals, polled while the Security section is open.
const POLL_MS = 5000

const toast = useToast()
const { confirm } = useConfirm()

const sessions = ref<TerminalSessionView[]>([])
const isLoading = ref(true)
const loadError = ref('')
let pollTimer: ReturnType<typeof setInterval> | null = null
// Request sequencing: discard a stale response that resolves after a newer
// poll started (prevents an out-of-order write from a slow request).
let sessionsReqId = 0

const fetchSessions = async () => {
  const mine = ++sessionsReqId
  try {
    const data = await AdminApiService.fetchTerminalSessions()
    if (mine !== sessionsReqId) return
    sessions.value = data
    loadError.value = ''
  } catch (e) {
    if (mine !== sessionsReqId) return
    loadError.value = errorMessage(e)
  } finally {
    if (mine === sessionsReqId) isLoading.value = false
  }
}

const resetTerminal = async (workspaceID: string) => {
  const ok = await confirm({
    title: `Reset terminals of ${workspaceID}?`,
    message: 'Every terminal session of this workspace is killed; a running command stops. The next command starts a fresh session.',
    type: 'warning',
    confirmText: 'Reset',
  })
  if (!ok) return
  try {
    await AdminApiService.resetTerminalSession(workspaceID)
    toast.success(`Terminal sessions reset for ${workspaceID}`)
    await fetchSessions()
  } catch (e) {
    toast.error(`Could not reset the terminal: ${errorMessage(e)}`)
  }
}

const sessionKey = (s: TerminalSessionView) => `${s.workspace_id}|${s.network_on}`
const COLUMNS: DataTableColumn<TerminalSessionView>[] = [
  { key: 'workspace', label: 'Workspace', value: (s) => s.workspace_id },
  { key: 'network', label: 'Network' },
  { key: 'used', label: 'Last activity' },
  { key: 'path', label: 'Host path' },
  { key: 'actions', label: 'Actions' },
]

onMounted(() => {
  void fetchSessions()
  pollTimer = setInterval(() => void fetchSessions(), POLL_MS)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
  pollTimer = null
})
</script>

<template>
  <Panel title="Active host terminals" flush>
    <template #actions>
      <span class="font-mono text-[length:var(--text-small)] tabular-nums text-muted">{{ sessions.length }} active</span>
    </template>
    <DataTable
      :columns="COLUMNS"
      :rows="sessions"
      :row-key="sessionKey"
      caption="Host terminal sessions"
      :loading="isLoading && !sessions.length"
      :error="sessions.length ? null : loadError || null"
      error-next="The monitor retries every few seconds; check the app log if it keeps failing."
      empty-title="No persistent sessions"
      empty-body="Terminals start on demand when an agent runs a command."
    >
      <template #cell-network="{ row }">
        <StatusTag :state="row.network_on ? 'success' : 'neutral'" :label="row.network_on ? 'Net on' : 'Net off'" />
      </template>
      <template #cell-used="{ row }">
        <time :datetime="row.last_used" :title="formatAbsoluteTime(row.last_used)">{{ formatRelativeTime(row.last_used) }}</time>
      </template>
      <template #cell-path="{ row }">
        <span class="block max-w-xs truncate font-mono text-[length:var(--text-small)] text-muted" :title="row.host_path">{{ row.host_path }}</span>
      </template>
      <template #cell-actions="{ row }">
        <BaseButton variant="ghost" size="sm" icon="refresh" icon-only :label="`Reset terminals of ${row.workspace_id}`" @click="resetTerminal(row.workspace_id)" />
      </template>
    </DataTable>
  </Panel>
</template>
