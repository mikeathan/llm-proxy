<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import { useRouter } from "vue-router"
import BaseButton from "../common/buttons/BaseButton.vue"
import StatusTag from "../common/display/StatusTag.vue"
import { useDispatcher } from "../../composables/automation/useDispatcher"
import { AUDITOR_PROFILE } from "../../constants/netaudit"
import { toAutomation } from "../../router/routes"
import { automationStatus, nextRunLabel, triggerLabel } from "../../utils/automation/automationDisplay"
import { formatRelativeTime } from "../../utils/format/time"
import type { Automation } from "../../types/dispatcher"

// The network audits that already exist (automations that use the Security auditor), with when each runs, how the last
// run went and a way to open it, so nobody has to remember where they made one.
const router = useRouter()
const { automations, workspaces, fetchAutomations, fetchWorkspaces } = useDispatcher()

const audits = computed(() => automations.value.filter((a) => a.profile === AUDITOR_PROFILE))
const showWorkspace = computed(() => workspaces.value.length > 1)

const lastRun = (a: Automation): string => {
  const at = a.history?.[0]?.timestamp
  return at ? `Last run ${formatRelativeTime(at)}` : "Has not run yet"
}
const schedule = (a: Automation): string => [triggerLabel(a), nextRunLabel(a) ? `next ${nextRunLabel(a)}` : ""].filter(Boolean).join(" · ")

// `loaded` is false until the automations have been read, so the page never offers "set up" while an audit exists.
const loaded = ref(false)
defineExpose({ audits, loaded })
onMounted(async () => {
  void fetchWorkspaces()
  await fetchAutomations(true)
  loaded.value = true
})
</script>

<template>
  <ul v-if="audits.length" class="m-0 flex list-none flex-col gap-2 p-0" aria-label="Your network audits" data-test="audits">
    <li v-for="a in audits" :key="a.id" class="flex flex-col gap-2 border-l-2 border-accent-info bg-surface px-3 py-2 sm:flex-row sm:items-center sm:justify-between">
      <span class="min-w-0">
        <span class="flex flex-wrap items-center gap-2">
          <span class="break-all font-semibold text-primary">{{ a.name }}</span>
          <span v-if="showWorkspace" class="font-mono text-[length:var(--text-micro)] text-faint">{{ a.workspace }}</span>
          <StatusTag v-bind="automationStatus(a)" />
        </span>
        <span class="block text-[length:var(--text-small)] text-muted">{{ schedule(a) }} · {{ lastRun(a) }}</span>
      </span>
      <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" @click="router.push(toAutomation(a.id))">Open</BaseButton>
    </li>
  </ul>
</template>
