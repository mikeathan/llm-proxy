<script setup lang="ts">
import { computed, onMounted } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import StatusTag from "../common/display/StatusTag.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import { useStorageReport } from "../../composables/settings/useStorageReport"
import { formatBytes } from "../../utils/format/formatters"
import {
  STORAGE_INTRO, STORAGE_LEVEL_WORDS, STORAGE_LOADING, STORAGE_NOT_DESCRIBED, STORAGE_STACK, STORAGE_TITLE, storageCapLine, storageRecordsLine,
  storageTotalLine,
} from "../../constants/storage"
import type { StatusState } from "../../types/ui"
import type { StorageRow } from "../../types/storage"

// Settings · Storage & logging · What is stored: one row per kind of store in the data folder, largest first, each with its size and where it
// can be removed; a stack's file says what the stack keeps it to; anything the service does not recognise is listed under its own name. The
// workspace folders are people's own work, so only their size is shown. It reads; removing stays where it already is.
const { report, error, busy, load } = useStorageReport()
const LEVEL_STATE: Record<string, StatusState> = { ok: "success", low: "warning", critical: "error", unknown: "neutral" }
const rows = computed<StorageRow[]>(() => [...(report.value?.rows ?? []), ...(report.value?.workspaces ? [report.value.workspaces] : [])])
function detail(r: StorageRow): string {
  const parts: string[] = []
  if (r.owner !== "core" && r.owner !== "people" && r.described) parts.push(STORAGE_STACK)
  if (r.cap_bytes) parts.push(storageCapLine(formatBytes(r.cap_bytes)))
  if (r.records) parts.push(storageRecordsLine(r.records))
  return parts.join(" · ")
}
onMounted(load)
</script>

<template>
  <Panel :title="STORAGE_TITLE">
    <div class="flex flex-col gap-3" data-test="storage-report">
      <p class="m-0 text-[length:var(--text-small)] text-muted">{{ STORAGE_INTRO }}</p>
      <ErrorState v-if="error && !report" title="The data folder could not be measured" :cause="error" next="Check that the server is running, then retry.">
        <template #action><BaseButton variant="secondary" icon="refresh" class-name="max-sm:!h-11" @click="load">Retry</BaseButton></template>
      </ErrorState>
      <LoadingState v-else-if="!report" :label="STORAGE_LOADING" />
      <template v-else>
        <div class="flex flex-wrap items-center gap-3">
          <StatusTag :state="LEVEL_STATE[report.level] ?? 'neutral'" :label="STORAGE_LEVEL_WORDS[report.level] ?? report.level" data-test="storage-level" />
          <span class="text-[length:var(--text-small)] text-secondary" data-test="storage-total">{{ storageTotalLine(formatBytes(report.total), formatBytes(report.free), formatBytes(report.size)) }}</span>
          <BaseButton variant="ghost" size="sm" icon="refresh" class-name="max-sm:!h-11" :disabled="busy" data-test="storage-refresh" @click="load">Refresh</BaseButton>
        </div>
        <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
          <li v-for="r in rows" :key="r.id" class="flex flex-col gap-0.5 py-2" :data-test="`storage-row-${r.id}`">
            <span class="flex flex-wrap items-baseline justify-between gap-2">
              <span class="min-w-0 break-words text-primary">{{ r.title }}</span>
              <span class="font-mono tabular-nums text-[length:var(--text-small)] text-secondary">{{ formatBytes(r.bytes) }}</span>
            </span>
            <span v-if="!r.described" class="text-[length:var(--text-small)] text-state-running" data-test="storage-unknown">{{ STORAGE_NOT_DESCRIBED }}</span>
            <span v-if="detail(r)" class="text-[length:var(--text-small)] text-muted">{{ detail(r) }}</span>
            <span v-if="r.clean" class="text-[length:var(--text-small)] text-muted" data-test="storage-clean">{{ r.clean }}</span>
          </li>
        </ul>
        <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
      </template>
    </div>
  </Panel>
</template>
