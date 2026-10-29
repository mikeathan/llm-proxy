<script setup lang="ts">
import { computed, ref } from "vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import DeviceFindings from "./DeviceFindings.vue"
import ProblemGroups from "./ProblemGroups.vue"
import { describePort, isCheck } from "../../utils/netaudit/friendly"
import { openPortsOf } from "../../utils/netaudit/expectations"
import { groupByDevice, groupProblems } from "../../utils/netaudit/report"
import { formatAbsoluteTime } from "../../utils/format/time"
import type { DeepCheck, DeviceExpectation, FindingFilter, FindingsReport, ReportSummary, ScanFinding } from "../../types/netaudit"

// The Advanced report: a tile for each part (they filter the list), the problems the checks found grouped by kind, then
// every device with its ports, services and versions. Both views read the same data.
const props = withDefaults(defineProps<{ report: FindingsReport; summary: ReportSummary; expectations?: readonly DeviceExpectation[]; deepChecks?: readonly DeepCheck[]; deepRunning?: string; extraProblems?: readonly ScanFinding[] }>(), { expectations: () => [], deepChecks: () => [], deepRunning: "", extraProblems: () => [] })
const emit = defineEmits<{
  (e: "accept", finding: ScanFinding): void
  (e: "revoke", finding: ScanFinding): void
  (e: "rescan", device: string): void
  (e: "mark", finding: ScanFinding): void
  (e: "unmark", finding: ScanFinding): void
  (e: "expect", device: string, ports: number[]): void
  (e: "edit-expected", device: string): void
  (e: "unexpect", device: string): void
  (e: "deep", device: string): void
}>()

// Opens on the problems when the checks found any, otherwise on the devices.
const allProblems = computed(() => [...props.summary.problems, ...props.extraProblems])
const filter = ref<FindingFilter>(allProblems.value.length ? "problems" : "all")
// A device the reader opened or folded by hand; every other device is open only when something there needs a look.
const toggled = ref<Record<string, boolean>>({})

// One row of pills, each a part of the report with its count, so a long report is read one part at a time. Problems come
// first and are what opens when there are any; "Devices" is every device with its ports.
const countOf = (value: FindingFilter): number => {
  switch (value) {
    case "problems": return allProblems.value.length
    case "all": return groupByDevice(props.report.findings.filter((f) => !isCheck(f))).length
    case "new": return props.summary.added.length
    case "changed": return props.summary.changed.length
    case "resolved": return props.summary.resolved.length
    default: return props.summary.accepted.length
  }
}
const PILLS: { value: FindingFilter; label: string }[] = [
  { value: "problems", label: "Problems" }, { value: "all", label: "Devices" }, { value: "new", label: "New" },
  { value: "changed", label: "Changed" }, { value: "resolved", label: "Resolved" }, { value: "accepted", label: "Accepted" },
]
const chipOptions = computed(() => PILLS.map((c) => ({ value: c.value, label: `${c.label} (${countOf(c.value)})` })))
const EMPTY_TEXT: Record<string, string> = {
  problems: "The checks found no problems.", new: "Nothing is new since the scan before.", changed: "Nothing has changed since the scan before.",
  accepted: "Nothing is marked as fine.", all: "No devices were found.",
}

// The problems a filter lists, grouped; every unaccepted problem is in the Problems block, so a device list never repeats one.
const problemGroups = computed(() => {
  switch (filter.value) {
    case "problems": return groupProblems(allProblems.value)
    case "new": return groupProblems(props.summary.problems.filter((f) => f.new))
    case "changed": return groupProblems(props.summary.problems.filter((f) => f.changed))
    default: return []
  }
})
const visible = computed<ScanFinding[]>(() => {
  const ports = (list: ScanFinding[]) => list.filter((f) => !isCheck(f) || f.accepted)
  switch (filter.value) {
    case "problems":
    case "resolved": return []
    case "new": return ports(props.summary.added)
    case "changed": return ports(props.summary.changed)
    case "accepted": return props.summary.accepted
    default: return ports(props.report.findings)
  }
})
const groups = computed(() => groupByDevice(visible.value))
const resolvedGroups = computed(() => groupByDevice(props.summary.resolved))
const showDevices = computed(() => filter.value !== "resolved" && filter.value !== "problems")

const setFilter = (value: string) => { filter.value = value as FindingFilter }
// Something needs a look on a device when a port is new or changed (never on the first scan, where all are the baseline), or it has an unaccepted problem.
const needsLook = (device: string) =>
  props.summary.problems.some((f) => f.device === device) || (!props.summary.baseline && [...props.summary.added, ...props.summary.changed].some((f) => f.device === device))
const expectedOf = (device: string) => props.expectations.find((e) => e.device === device)
const deepOf = (device: string) => props.deepChecks.find((c) => c.device === device)
const problemsOn = (device: string) => props.summary.problems.filter((f) => f.device === device).length
const isOpen = (device: string) => toggled.value[device] ?? (filter.value !== "all" || needsLook(device))
const toggle = (device: string) => { toggled.value = { ...toggled.value, [device]: !isOpen(device) } }
</script>

<template>
  <div class="flex flex-col gap-4" data-test="findings-advanced">
    <slot name="notice" />

    <p v-if="summary.baseline" class="m-0 text-[length:var(--text-small)] text-secondary" data-test="baseline-note">
      This is the first scan. What it found is saved as the baseline; from the next scan only changes are marked as new.
    </p>

    <SegmentedControl :model-value="filter" :options="chipOptions" label="Show part of the report" touch @update:model-value="setFilter" />

    <p v-if="filter === 'problems' && !problemGroups.length" class="m-0 text-muted" data-test="empty-part">{{ EMPTY_TEXT.problems }}</p>
    <ProblemGroups :groups="problemGroups" advanced @accept="emit('accept', $event)" @mark="emit('mark', $event)" @unmark="emit('unmark', $event)" @rescan="emit('rescan', $event)" />

    <template v-if="showDevices">
      <p v-if="!groups.length && !problemGroups.length" class="m-0 text-muted" data-test="empty-part">{{ EMPTY_TEXT[filter] ?? "Nothing to show here." }}</p>
      <DeviceFindings
        v-for="g in groups"
        :key="g.device"
        :group="g"
        :open="isOpen(g.device)"
        :baseline="summary.baseline"
        :problems="problemsOn(g.device)"
        @toggle="toggle(g.device)"
        @accept="emit('accept', $event)"
        @revoke="emit('revoke', $event)"
        :expected="expectedOf(g.device)"
        :deep="deepOf(g.device)"
        :deep-running="deepRunning === g.device"
        :deep-busy="deepRunning !== ''"
        @deep="emit('deep', $event)"
        @expect="emit('expect', $event, openPortsOf(report.findings, $event))"
        @edit-expected="emit('edit-expected', $event)"
        @unexpect="emit('unexpect', $event)"
        @rescan="emit('rescan', $event)"
      />
    </template>

    <template v-else-if="filter === 'resolved'">
      <p v-if="!resolvedGroups.length" class="m-0 text-muted">Nothing has gone away since the scan before.</p>
      <section v-for="g in resolvedGroups" :key="g.device" :aria-label="g.device" class="rounded-[var(--radius-md)] border border-hairline">
        <header class="bg-surface-raised px-3 py-2 font-mono font-semibold text-primary">{{ g.device }}</header>
        <p v-for="r in g.items" :key="`${describePort(r)}|${r.check ?? ''}`" class="m-0 border-t border-hairline px-3 py-2 font-mono text-[length:var(--text-small)] text-secondary">{{ isCheck(r) ? (r.evidence || "A check finding") : describePort(r) }} <span class="text-muted">is no longer there</span><template v-if="r.fixed_at"> <span class="text-state-success" data-test="fixed-confirmed">· fixed: you marked it on {{ formatAbsoluteTime(r.fixed_at) }} and the scan confirms it</span></template></p>
      </section>
    </template>
  </div>
</template>
