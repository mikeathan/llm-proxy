<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from "vue"
import { useRoute, useRouter } from "vue-router"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import ToggleField from "../common/forms/ToggleField.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import AuditList from "../netaudit/AuditList.vue"
import VulnDataCard from "../netaudit/VulnDataCard.vue"
import ReadinessBanner from "../netaudit/ReadinessBanner.vue"
import NetauditToolbar from "../netaudit/NetauditToolbar.vue"
import NetauditHelp from "../netaudit/NetauditHelp.vue"
import { useScanScope } from "../../composables/netaudit/useScanScope"
import { useNetauditReadiness } from "../../composables/netaudit/useNetauditReadiness"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { useActiveWorkspace } from "../../composables/ui/useActiveWorkspace"
import { AUDIT_PLAYBOOK, DEVICE_TIME_OPTIONS, SCAN_TIME_ANCHOR, SIMPLE_USE_NETWORK } from "../../constants/netaudit"
import { toAutomationNew, toNetwork } from "../../router/routes"
import type { ScanRange } from "../../types/netaudit"

// Settings · Network scan: which networks scans may look at. Every change saves at once, so there is no Save button.
// Simple is one decision ("Use my network"); Advanced lists the ranges, the networks this machine is on, and the
// confinement switch.
const scope = useScanScope()
const { advanced } = useNetauditViewMode()
const { readiness } = useNetauditReadiness()
const auditList = ref<InstanceType<typeof AuditList> | null>(null)
const hasAudit = computed(() => (auditList.value?.audits.length ?? 0) > 0)
const auditsLoaded = computed(() => auditList.value?.loaded === true)
const typed = ref("")
const router = useRouter()
const route = useRoute()
const { active: activeWorkspace } = useActiveWorkspace()

const suggestions = scope.unusedSuggestions
const canUseNetwork = computed(() => suggestions.value.length > 0)
const devicesLabel = (r: ScanRange) => `${r.devices} ${r.devices === 1 ? "device" : "devices"}`

async function addTyped() {
  if (!typed.value.trim()) return
  if (await scope.addRange(typed.value)) typed.value = ""
}

// An audit is an ordinary automation, started from its playbook in the ordinary form: the playbook sets the agent,
// network access, journal and first schedule, and everything stays editable there. It is offered once a scan could
// run, so the form never opens onto a setup that cannot work; what is missing is listed above.
const missing = computed(() => (readiness.value?.requirements.length ?? 1) > 0 || !scope.ranges.value.length)
const setUpAudit = () => router.push(toAutomationNew({ playbook: AUDIT_PLAYBOOK, workspace: activeWorkspace.value || undefined }))

// A warning can send you straight to the scan-time control (/settings/network-scan#scan-time).
async function scrollToAnchor() {
  if (route.hash !== `#${SCAN_TIME_ANCHOR}`) return
  await nextTick()
  document.getElementById(SCAN_TIME_ANCHOR)?.scrollIntoView({ block: "start" })
}

onMounted(async () => {
  await scope.load()
  await scrollToAnchor()
})
</script>

<template>
  <div class="flex flex-col gap-4">
    <NetauditToolbar />
    <ReadinessBanner here="network-scan" />

    <ErrorState v-if="scope.loadError.value" title="The allowed networks could not be read" :cause="scope.loadError.value" next="Check that the server is running, then retry.">
      <template #action><BaseButton variant="secondary" icon="refresh" @click="scope.load">Retry</BaseButton></template>
    </ErrorState>
    <LoadingState v-else-if="scope.loading.value && !scope.scope.value" label="Reading the allowed networks" />

    <template v-else-if="scope.scope.value">
      <Panel title="What may be scanned">
        <div class="flex flex-col gap-3">
          <p v-if="!scope.ranges.value.length" class="m-0 text-secondary">Scanning is off. Choose your network to turn it on; only devices inside it are ever looked at.</p>
          <ul v-else class="m-0 flex list-none flex-col gap-1 p-0" aria-label="Allowed networks">
            <li v-for="r in scope.ranges.value" :key="r.cidr" class="flex flex-wrap items-center justify-between gap-2">
              <span class="min-w-0 break-words text-primary">
                <span :class="advanced ? 'font-mono' : ''">{{ advanced ? r.cidr : "Your network" }}</span>
                <span class="text-muted"> · {{ devicesLabel(r) }}</span>
              </span>
              <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" @click="scope.removeRange(r.cidr)">{{ advanced ? "Remove" : "Stop allowing" }}</BaseButton>
            </li>
          </ul>

          <BaseButton v-if="!advanced && canUseNetwork" class-name="self-start max-sm:!h-11" :loading="scope.saving.value" @click="scope.useMyNetwork">{{ SIMPLE_USE_NETWORK }}</BaseButton>
          <p v-else-if="!advanced && !scope.ranges.value.length" class="m-0 text-[length:var(--text-small)] text-muted">
            No private network was found on this computer. Switch to Advanced to type one.
          </p>

          <template v-if="advanced">
            <section v-if="suggestions.length" aria-label="Networks this computer is on" class="flex flex-col gap-1">
              <p class="m-0 font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">This computer is on</p>
              <div v-for="s in suggestions" :key="s.cidr" class="flex flex-wrap items-center justify-between gap-2">
                <span class="font-mono text-[length:var(--text-small)] text-secondary">{{ s.cidr }} · {{ s.interface }} · {{ s.devices }} devices</span>
                <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" :loading="scope.saving.value" @click="scope.addRange(s.cidr)">Allow</BaseButton>
              </div>
            </section>
            <p v-if="scope.scope.value.suggestions_error" class="m-0 text-[length:var(--text-small)] text-muted">The networks this computer is on could not be listed: {{ scope.scope.value.suggestions_error }}</p>
            <form class="flex flex-wrap items-end gap-2" @submit.prevent="addTyped">
              <FormField label="Add an address or range" hint="For example 192.168.1.0/24, 192.168.1.10-192.168.1.40 or a device name on your network.">
                <template #default="{ id, describedBy }">
                  <input :id="id" v-model="typed" type="text" autocomplete="off" spellcheck="false" :aria-describedby="describedBy" class="h-8 min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 font-mono text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2" />
                </template>
              </FormField>
              <BaseButton type="submit" variant="secondary" class-name="max-sm:!h-11" :loading="scope.saving.value" :disabled="!typed.trim()">Add</BaseButton>
            </form>
          </template>

          <p v-if="scope.saveError.value" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ scope.saveError.value }}</p>
        </div>
      </Panel>

      <VulnDataCard />

      <p class="m-0 text-[length:var(--text-small)] text-muted">
        The devices on your network, the new-device check and the decoy services are on the
        <RouterLink :to="toNetwork('devices')" class="text-accent-info-text underline-offset-2 hover:underline focus-visible:outline-none focus-visible:ring-2">Network page</RouterLink>.
      </p>

      <Panel title="Network audit">
        <div class="flex flex-col gap-3">
          <AuditList ref="auditList" />
          <template v-if="hasAudit">
            <div v-if="advanced" class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
              <p class="m-0 min-w-0 flex-1 text-[length:var(--text-small)] text-muted">Another audit can use a different schedule, depth or ports. All audits use the networks allowed above.</p>
              <BaseButton variant="secondary" class-name="max-sm:!h-11" :disabled="missing" @click="setUpAudit">Add another audit</BaseButton>
            </div>
          </template>
          <div v-else-if="auditsLoaded" class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <p class="m-0 min-w-0 flex-1 text-secondary">Scan on a schedule and be told only what is new, changed or gone. This opens the new automation form with everything filled in; you can change any of it.</p>
            <BaseButton variant="primary" class-name="max-sm:!h-11" :disabled="missing" @click="setUpAudit">Set up a network audit</BaseButton>
          </div>
          <p v-if="auditsLoaded && missing && (!hasAudit || advanced)" class="m-0 text-[length:var(--text-small)] text-muted">{{ scope.ranges.value.length ? "Fix the items above first." : "Choose a network above first." }}</p>
        </div>
      </Panel>

      <Panel v-if="advanced" :id="SCAN_TIME_ANCHOR" title="Scan time">
        <FormField label="Time allowed per device" hint="Raise this if a slow device keeps showing as not scanned. A device that runs out of time is tried once more with twice as long. A whole scan may take up to 12 minutes.">
          <template #default="{ id, describedBy }">
            <SelectInput
              :id="id"
              :aria-describedby="describedBy"
              :model-value="String(scope.scope.value.device_timeout_minutes)"
              :options="DEVICE_TIME_OPTIONS"
              label="Time allowed per device"
              @update:model-value="scope.setDeviceTimeout(Number($event))"
            />
          </template>
        </FormField>
      </Panel>

      <Panel v-if="advanced" title="Confinement">
        <ToggleField
          :model-value="scope.scope.value.allow_unconfined"
          label="Allow scans without file confinement"
          hint="For development only. Normally nmap is kept inside its own folder by the operating system, and refuses to run where that is not possible."
          @update:model-value="scope.setAllowUnconfined"
        />
      </Panel>
    </template>

    <NetauditHelp />
  </div>
</template>
