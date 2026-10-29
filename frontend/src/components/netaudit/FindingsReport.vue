<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import AcceptDialog from "./AcceptDialog.vue"
import DeepCheckDialog from "./DeepCheckDialog.vue"
import ExpectedPortsDialog from "./ExpectedPortsDialog.vue"
import FindingsAdvanced from "./FindingsAdvanced.vue"
import FindingsSimple from "./FindingsSimple.vue"
import UnscannedNotice from "./UnscannedNotice.vue"
import { useDeepCheck } from "../../composables/netaudit/useDeepCheck"
import { useFindings } from "../../composables/netaudit/useFindings"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { openPortsOf } from "../../utils/netaudit/expectations"
import { checkCount, findingTitle, portCount } from "../../utils/netaudit/friendly"
import { summarize } from "../../utils/netaudit/report"
import { formatAbsoluteTime } from "../../utils/format/time"
import type { ScanFinding } from "../../types/netaudit"

// The latest scan of one automation. Simple: what needs a look, in plain sentences, one decision each. Advanced:
// counts, filters, and every device with its ports and versions. Both read the same data, so switching loses nothing.
// `refreshKey` changes when a run starts or finishes; the report is read again then, so a finished scan shows up
// without leaving the page.
const props = defineProps<{ workspaceId: string; automation: string; refreshKey?: string }>()

const { advanced } = useNetauditViewMode()
const findings = useFindings(() => props.workspaceId, () => props.automation)
const { report, expectations, loading, error, actionError, notice } = findings
const deepCheck = useDeepCheck(() => props.workspaceId, () => props.automation)

const accepting = ref<ScanFinding | null>(null)
const saving = ref(false)

// The device whose expected ports are being edited, and what the dialog starts from: what it expects now, or what is open.
const editing = ref("")
const editingPorts = computed(() => expectations.value.find((e) => e.device === editing.value)?.ports ?? openPortsOf(report.value?.findings ?? [], editing.value))

// The device whose deep check is being asked for; the dialog says what will be sent before anything is.
const deepDevice = ref("")
const deepPorts = computed(() => openPortsOf(report.value?.findings ?? [], deepDevice.value))
async function runDeep() {
  if (await deepCheck.run(deepDevice.value)) deepDevice.value = ""
}

const summary = computed(() => (report.value ? summarize(report.value) : null))

// Devices the scan ran out of time on that still have findings from a scan before: those are kept.
const hasEarlier = computed(() => !!report.value?.unscanned.some((d) => report.value?.findings.some((f) => f.device === d)))

const meta = computed(() => {
  const r = report.value
  if (!r?.scanned) return ""
  const parts = [formatAbsoluteTime(r.scanned_at ?? ""), r.engine, `${r.devices_up} ${r.devices_up === 1 ? "device" : "devices"} up`, `${portCount(r.findings)} open ${portCount(r.findings) === 1 ? "port" : "ports"}`, checkCount(r.findings) ? `${checkCount(r.findings)} ${checkCount(r.findings) === 1 ? "problem" : "problems"} found by checks` : ""]
  return parts.filter(Boolean).join(" · ")
})

async function accept(days: number, note: string) {
  if (!accepting.value) return
  saving.value = true
  const ok = await findings.accept(accepting.value.key, note, days)
  saving.value = false
  if (ok) accepting.value = null
}

async function saveExpected(ports: number[]) {
  saving.value = true
  const ok = await findings.setExpected(editing.value, ports)
  saving.value = false
  if (ok) editing.value = ""
}

watch(() => [props.workspaceId, props.automation, props.refreshKey], () => { void findings.load(); void deepCheck.load() })
onMounted(() => { void findings.load(); void deepCheck.load() })
</script>

<template>
  <Panel title="Latest network scan" preserve-case>
    <LoadingState v-if="loading && !report" label="Reading the latest scan" />
    <ErrorState v-else-if="error" title="The latest scan could not be read" :cause="error" next="Check that the server is running, then try again.">
      <template #action><BaseButton variant="secondary" icon="refresh" @click="findings.load">Retry</BaseButton></template>
    </ErrorState>
    <EmptyState v-else-if="!report?.scanned || !summary" title="Not scanned yet" body="The first report appears here after the automation's first scan." />

    <div v-else class="flex flex-col gap-4">
      <p class="m-0 text-[length:var(--text-small)] text-muted" data-test="scan-meta">{{ meta }}</p>
      <p v-if="actionError" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ actionError }}</p>
      <p v-if="notice" role="status" class="m-0 text-[length:var(--text-small)] text-secondary" data-test="scan-notice">{{ notice }}</p>

      <FindingsAdvanced v-if="advanced" :report="report" :summary="summary" :expectations="expectations" :deep-checks="deepCheck.checks.value" :deep-running="deepCheck.running.value" @deep="deepDevice = $event" @expect="findings.setExpected" @edit-expected="editing = $event" @unexpect="findings.removeExpected($event)" @accept="accepting = $event" @revoke="findings.revoke($event.key)" @rescan="findings.rescan($event)" @mark="findings.markFixed($event.key)" @unmark="findings.unmarkFixed($event.key)">
        <template #notice><UnscannedNotice :devices="report.unscanned" :has-earlier="hasEarlier" @rescan="findings.rescan($event)" /></template>
      </FindingsAdvanced>
      <template v-else>
        <UnscannedNotice :devices="report.unscanned" :has-earlier="hasEarlier" @rescan="findings.rescan($event)" />
        <FindingsSimple :report="report" :summary="summary" @accept="accepting = $event" @revoke="findings.revoke($event.key)" @mark="findings.markFixed($event.key)" @unmark="findings.unmarkFixed($event.key)" @rescan="findings.rescan($event)" />
      </template>
    </div>

    <DeepCheckDialog :open="deepDevice !== ''" :device="deepDevice" :ports="deepPorts" :running="deepCheck.running.value !== ''" :error="deepCheck.error.value" @run="runDeep" @cancel="deepDevice = ''" />
    <ExpectedPortsDialog :open="editing !== ''" :device="editing" :ports="editingPorts" :saving="saving" :error="actionError" @save="saveExpected" @cancel="editing = ''" />
    <AcceptDialog :open="accepting !== null" :what="accepting ? findingTitle(accepting) : ''" :saving="saving" :error="actionError" @accept="accept" @cancel="accepting = null" />
  </Panel>
</template>
