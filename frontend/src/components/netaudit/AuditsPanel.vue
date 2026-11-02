<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import { useRouter } from "vue-router"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import AuditList from "./AuditList.vue"
import { useScanScope } from "../../composables/netaudit/useScanScope"
import { useNetauditReadiness } from "../../composables/netaudit/useNetauditReadiness"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { useActiveWorkspace } from "../../composables/ui/useActiveWorkspace"
import { AUDIT_PLAYBOOK, AUDITS_ADD_ANOTHER, AUDITS_ADD_ANOTHER_HINT, AUDITS_FIX_FIRST, AUDITS_NEED_NETWORK, AUDITS_NONE, AUDITS_OPEN_SETTINGS, AUDITS_SET_UP, NETWORK_AUDITS_TITLE } from "../../constants/netaudit"
import { toAutomationNew, toNetwork } from "../../router/routes"

// Network · Audits: the scans that run on a schedule, and the one way to make another. An audit is an ordinary automation,
// started from its playbook in the ordinary form: the playbook sets the agent, network access, journal and first schedule,
// and everything stays editable there. It is offered once a scan could run, so the form never opens onto a setup that
// cannot work; what is missing is listed at the top of the page. Advanced adds a second audit.
const scope = useScanScope()
const { readiness } = useNetauditReadiness()
const { advanced } = useNetauditViewMode()
const router = useRouter()
const { active: activeWorkspace } = useActiveWorkspace()

const auditList = ref<InstanceType<typeof AuditList> | null>(null)
const hasAudit = computed(() => (auditList.value?.audits.length ?? 0) > 0)
const auditsLoaded = computed(() => auditList.value?.loaded === true)
const missing = computed(() => (readiness.value?.requirements.length ?? 1) > 0 || !scope.ranges.value.length)
const setUpAudit = () => router.push(toAutomationNew({ playbook: AUDIT_PLAYBOOK, workspace: activeWorkspace.value || undefined }))

onMounted(() => void scope.load())
</script>

<template>
  <Panel :title="NETWORK_AUDITS_TITLE">
    <div class="flex flex-col gap-3" data-test="audits-panel">
      <AuditList ref="auditList" />
      <div v-if="hasAudit && advanced" class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <p class="m-0 min-w-0 flex-1 text-[length:var(--text-small)] text-muted">{{ AUDITS_ADD_ANOTHER_HINT }}</p>
        <BaseButton variant="secondary" class-name="max-sm:!h-11" :disabled="missing" @click="setUpAudit">{{ AUDITS_ADD_ANOTHER }}</BaseButton>
      </div>
      <div v-else-if="auditsLoaded && !hasAudit" class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between" data-test="no-audit">
        <p class="m-0 min-w-0 flex-1 text-secondary">{{ AUDITS_NONE }}</p>
        <BaseButton variant="primary" class-name="max-sm:!h-11" :disabled="missing" @click="setUpAudit">{{ AUDITS_SET_UP }}</BaseButton>
      </div>
      <p v-if="auditsLoaded && missing && (!hasAudit || advanced)" class="m-0 text-[length:var(--text-small)] text-muted" data-test="audits-missing">
        <template v-if="scope.ranges.value.length">{{ AUDITS_FIX_FIRST }}</template>
        <template v-else>
          {{ AUDITS_NEED_NETWORK }}
          <RouterLink :to="toNetwork('settings')" class="inline-flex min-h-11 items-center text-accent-info-text underline-offset-2 hover:underline focus-visible:outline-none focus-visible:ring-2 sm:min-h-8">{{ AUDITS_OPEN_SETTINGS }}</RouterLink>
        </template>
      </p>
    </div>
  </Panel>
</template>
