<script setup lang="ts">
import { computed, onMounted } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Meter from "../common/display/Meter.vue"
import VulnDataImport from "./VulnDataImport.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import { useConfirm } from "../../composables/ui/useConfirm"
import { useVulnData } from "../../composables/netaudit/useVulnData"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { VULN_ATTRIBUTION, VULN_DOWNLOAD_SIZE, VULN_NOT_PROOF } from "../../constants/netaudit"
import { vulnProgress, vulnSources, vulnSummary } from "../../utils/netaudit/vulnData"
import type { VulnDataAction } from "../../types/netaudit"

// Settings · Network scan · Vulnerability data: whether the list of known flaws is on this computer, and the one button
// that fetches it. Simple is a status line and a button; Advanced adds a row per source. The download runs on the
// server, so closing this page does not stop it and a reload shows where it is.
const { status, error, actionError, busy, refresh, start, cancel, clear } = useVulnData()
const { confirm } = useConfirm()
const { advanced } = useNetauditViewMode()

const ACTION_LABEL: Record<VulnDataAction, string> = { download: "Download", resume: "Resume", cancel: "Cancel", update: "Update" }

const summary = computed(() => (status.value ? vulnSummary(status.value) : null))
const progress = computed(() => (status.value ? vulnProgress(status.value) : null))
const sources = computed(() => (status.value ? vulnSources(status.value) : []))
const running = computed(() => status.value?.state === "running")
// Advanced only: fetch every file again, for when what is held is doubted. Resume and Update already cover the rest.
const canStartAgain = computed(() => advanced.value && !running.value && (status.value?.files_done ?? 0) > 0)
// A stopped download says why; so does saved data that could not be read.
// Deleting is offered once something is held and nothing is running (a running download is cancelled first).
const canDelete = computed(() => !running.value && (status.value?.files_done ?? 0) > 0)

async function deleteData() {
  const ok = await confirm({
    title: "Delete the vulnerability data?",
    message: "Audits will skip the known-vulnerability check until you download it again (more than 100 MB).",
    type: "warning",
    confirmText: "Delete data",
  })
  if (ok) await clear()
}

// The import is for a computer that cannot download: always in Advanced, and in Simple once a download has stopped.
const showImport = computed(() => !running.value && (advanced.value || status.value?.state === "stopped"))

const reason = computed(() => (running.value ? "" : (status.value?.reason ?? "")))

onMounted(refresh)
</script>

<template>
  <Panel title="Vulnerability data">
    <div class="flex flex-col gap-3" data-test="vuln-data">
      <ErrorState v-if="error && !status" title="The vulnerability data could not be checked" :cause="error" next="Check that the server is running, then retry.">
        <template #action><BaseButton variant="secondary" icon="refresh" class-name="max-sm:!h-11" @click="refresh">Retry</BaseButton></template>
      </ErrorState>
      <LoadingState v-else-if="!status || !summary" label="Checking the vulnerability data" />

      <template v-else>
        <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <p class="m-0 min-w-0 text-primary" data-test="vuln-line">
            {{ summary.line }}
            <span v-if="advanced && running && status.file" class="text-muted"> · {{ status.file }}</span>
          </p>
          <div class="flex flex-wrap gap-2">
            <BaseButton
              v-if="summary.action"
              :variant="summary.action === 'cancel' ? 'secondary' : 'primary'"
              class-name="max-sm:!h-11"
              :loading="busy"
              @click="summary.action === 'cancel' ? cancel() : start()"
            >{{ ACTION_LABEL[summary.action] }}</BaseButton>
            <BaseButton v-if="canStartAgain" variant="secondary" class-name="max-sm:!h-11" :disabled="busy" @click="start(true)">Start again</BaseButton>
            <BaseButton v-if="canDelete" variant="ghost" class-name="max-sm:!h-11" :disabled="busy" @click="deleteData">Delete data</BaseButton>
          </div>
        </div>

        <Meter v-if="running && progress" label="Download progress" :value="progress.fraction * 100" :text="progress.text" />
        <p v-if="reason" class="m-0 text-[length:var(--text-small)] text-state-running" data-test="vuln-reason">{{ reason }}</p>
        <p v-if="summary.old" class="m-0 text-[length:var(--text-small)] text-muted">This data is more than 60 days old, so newer flaws are missing. Update it.</p>
        <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }} The status shown may be out of date.</p>
        <p v-if="actionError" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ actionError }}</p>

        <ul v-if="advanced && sources.length" class="m-0 flex list-none flex-col gap-1 p-0" aria-label="Data held">
          <li v-for="row in sources" :key="row.name" class="flex flex-col gap-0.5">
            <span class="flex flex-wrap justify-between gap-x-3 text-[length:var(--text-small)]">
              <span class="text-secondary">{{ row.name }}</span>
              <span class="font-mono text-muted">{{ row.detail }}</span>
            </span>
            <span v-if="row.skipped" class="text-[length:var(--text-small)] text-faint">{{ row.skipped }}</span>
          </li>
        </ul>

        <VulnDataImport v-if="showImport" />

        <div class="flex flex-col gap-1 text-[length:var(--text-small)] text-muted">
          <p class="m-0">{{ VULN_NOT_PROOF }}</p>
          <p v-if="summary.action && summary.action !== 'cancel'" class="m-0">{{ VULN_DOWNLOAD_SIZE }}</p>
          <p class="m-0">{{ VULN_ATTRIBUTION }}</p>
        </div>
      </template>
    </div>
  </Panel>
</template>
