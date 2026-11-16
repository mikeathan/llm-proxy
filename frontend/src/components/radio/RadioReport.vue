<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Callout from "../common/feedback/Callout.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import FormField from "../common/forms/FormField.vue"
import StatusTag from "../common/display/StatusTag.vue"
import { useRadioReport } from "../../composables/radio/useRadioReport"
import { useRadioSensors } from "../../composables/radio/useRadioSensors"
import { toAutomation, toRadio } from "../../router/routes"
import {
  RADIO_REPORT_ANALYSE, RADIO_REPORT_DAILY, RADIO_REPORT_INTRO, RADIO_REPORT_LAST, RADIO_REPORT_MANUAL, RADIO_REPORT_NOT_LISTENING, RADIO_REPORT_OFF,
  RADIO_REPORT_OPEN, RADIO_REPORT_QUEUED, RADIO_REPORT_RUNNING, RADIO_REPORT_RUNS_MANUAL, RADIO_REPORT_SCHEDULE_LABEL, RADIO_REPORT_SETUP,
  RADIO_REPORT_START, RADIO_REPORT_TITLE, RADIO_REPORT_WORKSPACE, radioReportRuns,
} from "../../constants/radio"

// Radio · Report: one button. The first press asks when the report should run (only on a press, or every morning), makes the automation and runs it;
// later presses run it. It never starts listening (that takes the passphrase on the Listening tab); it says so when nothing is listening.
const r = useRadioReport()
const s = useRadioSensors()
const schedule = ref("manual")
const workspace = ref("")

const off = computed(() => !!s.view.value && !s.view.value.enabled)
const found = computed(() => r.automation.value)
const queued = computed(() => !!found.value?.queued)
const running = computed(() => !!found.value?.is_running || (r.waiting.value && !queued.value))
const notListening = computed(() => r.listening.value === false)
const scheduleOptions = [{ value: "manual", label: RADIO_REPORT_MANUAL }, { value: "daily", label: RADIO_REPORT_DAILY }]
const workspaceOptions = computed(() => r.workspaces.value.map((w) => ({ value: w, label: w })))
const runsLine = computed(() => (!found.value ? "" : found.value.trigger_type === "manual" || !found.value.trigger_type ? RADIO_REPORT_RUNS_MANUAL : radioReportRuns(found.value.trigger_value, found.value.next_run_at)))
const canGo = computed(() => !off.value && !r.busy.value && !running.value && !queued.value)

function go(): void { void (found.value ? r.analyse() : r.setUp(workspace.value || r.workspaces.value[0] || "", schedule.value === "daily" ? "daily" : "manual")) }

onMounted(async () => {
  await Promise.all([r.load(), s.load()])
  workspace.value = r.workspaces.value[0] ?? ""
})
</script>

<template>
  <Panel :title="RADIO_REPORT_TITLE">
    <div class="flex flex-col gap-3" data-test="radio-report">
      <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_REPORT_INTRO }}</p>

      <Callout v-if="off" tone="warning" title="The Radio stack is off"><span data-test="radio-report-off">{{ RADIO_REPORT_OFF }}</span></Callout>
      <p v-else-if="notListening" class="m-0 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 text-[length:var(--text-small)] text-secondary" data-test="radio-report-notlistening">
        <span>{{ RADIO_REPORT_NOT_LISTENING }}</span>
        <RouterLink :to="toRadio()" class="inline-flex min-h-11 items-center text-accent-brand hover:underline" data-test="radio-report-start">{{ RADIO_REPORT_START }}</RouterLink>
      </p>

      <div v-if="!found" class="flex flex-wrap items-end gap-3" data-test="radio-report-setup">
        <FormField :label="RADIO_REPORT_SCHEDULE_LABEL"><template #default><SegmentedControl v-model="schedule" :options="scheduleOptions" :label="RADIO_REPORT_SCHEDULE_LABEL" variant="compact" touch /></template></FormField>
        <FormField v-if="workspaceOptions.length > 1" :label="RADIO_REPORT_WORKSPACE"><template #default><SelectInput v-model="workspace" :options="workspaceOptions" :label="RADIO_REPORT_WORKSPACE" /></template></FormField>
      </div>
      <p v-else class="m-0 text-[length:var(--text-small)] text-secondary" data-test="radio-report-runs">{{ runsLine }}</p>

      <div class="flex flex-wrap items-center gap-3">
        <BaseButton variant="primary" class-name="max-sm:!h-11" :disabled="!canGo" data-test="radio-report-go" @click="go">{{ found ? RADIO_REPORT_ANALYSE : RADIO_REPORT_SETUP }}</BaseButton>
        <StatusTag v-if="running" state="running" :label="RADIO_REPORT_RUNNING" data-test="radio-report-running" />
        <StatusTag v-else-if="queued" state="queued" :label="RADIO_REPORT_QUEUED" data-test="radio-report-queued" />
        <RouterLink v-if="found" :to="toAutomation(found.id)" class="inline-flex min-h-11 items-center text-accent-brand hover:underline" data-test="radio-report-open">{{ RADIO_REPORT_OPEN }}</RouterLink>
      </div>

      <Callout v-if="r.error.value" tone="error" title="The report did not run"><span data-test="radio-report-error">{{ r.error.value }}</span></Callout>
      <Callout v-else-if="found?.last_error && !running" tone="error" title="The last run failed"><span data-test="radio-report-last-error">{{ found.last_error }}</span></Callout>
      <div v-else-if="found?.last_output && !running" class="flex flex-col gap-1">
        <span class="text-[length:var(--text-small)] text-secondary">{{ RADIO_REPORT_LAST }}</span>
        <p class="m-0 whitespace-pre-wrap text-primary" data-test="radio-report-output">{{ found.last_output }}</p>
      </div>
    </div>
  </Panel>
</template>
