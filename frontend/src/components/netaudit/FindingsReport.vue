<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import AcceptDialog from "./AcceptDialog.vue"
import FindingsAdvanced from "./FindingsAdvanced.vue"
import FindingsSimple from "./FindingsSimple.vue"
import UnscannedNotice from "./UnscannedNotice.vue"
import { useFindings } from "../../composables/netaudit/useFindings"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
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
const { report, loading, error, actionError, notice } = findings

const accepting = ref<ScanFinding | null>(null)
const saving = ref(false)

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

watch(() => [props.workspaceId, props.automation, props.refreshKey], findings.load)
onMounted(findings.load)
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

      <FindingsAdvanced v-if="advanced" :report="report" :summary="summary" @accept="accepting = $event" @revoke="findings.revoke($event.key)" @rescan="findings.rescan($event)">
        <template #notice><UnscannedNotice :devices="report.unscanned" :has-earlier="hasEarlier" @rescan="findings.rescan($event)" /></template>
      </FindingsAdvanced>
      <template v-else>
        <UnscannedNotice :devices="report.unscanned" :has-earlier="hasEarlier" @rescan="findings.rescan($event)" />
        <FindingsSimple :report="report" :summary="summary" @accept="accepting = $event" @revoke="findings.revoke($event.key)" />
      </template>
    </div>

    <AcceptDialog :open="accepting !== null" :what="accepting ? findingTitle(accepting) : ''" :saving="saving" :error="actionError" @accept="accept" @cancel="accepting = null" />
  </Panel>
</template>
