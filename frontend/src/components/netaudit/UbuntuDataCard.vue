<script setup lang="ts">
import { computed, onMounted } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import { useConfirm } from "../../composables/ui/useConfirm"
import { useUbuntuData } from "../../composables/netaudit/useUbuntuData"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { formatRelativeTime, isRecorded } from "../../utils/format/time"
import { UBUNTU_ATTRIBUTION, UBUNTU_EMPTY, UBUNTU_NOTE, UBUNTU_OLD_DAYS, UBUNTU_OLD_NOTE, UBUNTU_SIZE, UBUNTU_TITLE, UBUNTU_UNSUPPORTED } from "../../constants/netaudit"

// Network · Settings · Ubuntu security data: whether Canonical's notices for this computer's release are held, and the one
// button that fetches them. With them the audit says which installed packages are behind a security fix. Simple is one line
// and a button; Advanced adds the counts and when the data was built.
const { status, error, actionError, busy, refresh, update, clear } = useUbuntuData()
const { confirm } = useConfirm()
const { advanced } = useNetauditViewMode()

const MS_PER_DAY = 86_400_000
const held = computed(() => (status.value?.notices ?? 0) > 0)
const old = computed(() => held.value && isRecorded(status.value?.imported_at ?? "") && Date.now() - new Date(status.value!.imported_at!).getTime() > UBUNTU_OLD_DAYS * MS_PER_DAY)
const line = computed(() => {
  const s = status.value
  if (!s || !held.value) return UBUNTU_EMPTY
  return `${s.notices.toLocaleString("en-US")} security notices for Ubuntu ${s.release}, downloaded ${isRecorded(s.imported_at ?? "") ? formatRelativeTime(s.imported_at!) : "earlier"}.`
})

async function deleteData() {
  const ok = await confirm({
    title: "Delete the Ubuntu security data?",
    message: "Audits will not say which installed packages are behind a fix until you update it again (about 1.3 MB).",
    type: "warning",
    confirmText: "Delete data",
  })
  if (ok) await clear()
}

onMounted(refresh)
</script>

<template>
  <Panel :title="UBUNTU_TITLE">
    <div class="flex flex-col gap-3" data-test="ubuntu-data">
      <ErrorState v-if="error && !status" title="The Ubuntu security data could not be checked" :cause="error" next="Check that the server is running, then retry.">
        <template #action><BaseButton variant="secondary" icon="refresh" class-name="max-sm:!h-11" @click="refresh">Retry</BaseButton></template>
      </ErrorState>
      <LoadingState v-else-if="!status" label="Checking the Ubuntu security data" />
      <p v-else-if="!status.supported" class="m-0 text-muted" data-test="ubuntu-unsupported">{{ UBUNTU_UNSUPPORTED }}</p>

      <template v-else>
        <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <p class="m-0 min-w-0 text-primary" data-test="ubuntu-line">{{ line }}</p>
          <div class="flex flex-wrap gap-2">
            <BaseButton variant="primary" class-name="max-sm:!h-11" :loading="busy" data-test="ubuntu-update" @click="update">{{ held ? "Update notices" : "Download notices" }}</BaseButton>
            <BaseButton v-if="held" variant="ghost" class-name="max-sm:!h-11" :disabled="busy" data-test="ubuntu-delete" @click="deleteData">Delete data</BaseButton>
          </div>
        </div>
        <p v-if="status.reason" class="m-0 text-[length:var(--text-small)] text-state-running" data-test="ubuntu-reason">{{ status.reason }}</p>
        <p v-if="old" class="m-0 text-[length:var(--text-small)] text-muted" data-test="ubuntu-old">{{ UBUNTU_OLD_NOTE }}</p>
        <p v-if="actionError" role="alert" class="m-0 break-words text-[length:var(--text-small)] text-state-error" data-test="ubuntu-error">{{ actionError }}</p>
        <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }} The status shown may be out of date.</p>

        <dl v-if="advanced && held" class="m-0 flex flex-col gap-1 text-[length:var(--text-small)]" aria-label="Data held" data-test="ubuntu-details">
          <div class="flex flex-wrap justify-between gap-x-3"><dt class="text-secondary">Packages with a fix listed</dt><dd class="m-0 font-mono text-muted">{{ status.packages.toLocaleString("en-US") }}</dd></div>
          <div v-if="isRecorded(status.generated_at ?? '')" class="flex flex-wrap justify-between gap-x-3"><dt class="text-secondary">Built by Canonical</dt><dd class="m-0 font-mono text-muted">{{ status.generated_at!.slice(0, 10) }}</dd></div>
          <div class="flex flex-wrap justify-between gap-x-3"><dt class="text-secondary">This computer</dt><dd class="m-0 font-mono text-muted">Ubuntu {{ status.host_release }}</dd></div>
        </dl>

        <div class="flex flex-col gap-1 text-[length:var(--text-small)] text-muted">
          <p class="m-0">{{ UBUNTU_NOTE }}</p>
          <p v-if="!held" class="m-0">{{ UBUNTU_SIZE }}</p>
          <p class="m-0">{{ UBUNTU_ATTRIBUTION }}</p>
        </div>
      </template>
    </div>
  </Panel>
</template>
