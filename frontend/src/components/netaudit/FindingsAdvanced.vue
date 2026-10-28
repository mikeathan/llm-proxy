<script setup lang="ts">
import { computed, ref } from "vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import DeviceFindings from "./DeviceFindings.vue"
import ProblemGroups from "./ProblemGroups.vue"
import { describePort, isCheck } from "../../utils/netaudit/friendly"
import { openPortsOf } from "../../utils/netaudit/expectations"
import { groupByDevice, groupProblems } from "../../utils/netaudit/report"
import { formatAbsoluteTime } from "../../utils/format/time"
import type { DeviceExpectation, FindingFilter, FindingsReport, ReportSummary, ScanFinding } from "../../types/netaudit"

// The Advanced report: a tile for each part (they filter the list), the problems the checks found grouped by kind, then
// every device with its ports, services and versions. Both views read the same data.
const props = withDefaults(defineProps<{ report: FindingsReport; summary: ReportSummary; expectations?: readonly DeviceExpectation[] }>(), { expectations: () => [] })
const emit = defineEmits<{
  (e: "accept", finding: ScanFinding): void
  (e: "revoke", finding: ScanFinding): void
  (e: "rescan", device: string): void
  (e: "mark", finding: ScanFinding): void
  (e: "unmark", finding: ScanFinding): void
  (e: "expect", device: string, ports: number[]): void
  (e: "edit-expected", device: string): void
  (e: "unexpect", device: string): void
}>()

const filter = ref<FindingFilter>("all")
// A device the reader opened or folded by hand; every other device is open only when something there needs a look.
const toggled = ref<Record<string, boolean>>({})

const TILES: { value: Exclude<FindingFilter, "all">; label: string; count: () => number; dashed?: boolean }[] = [
  { value: "problems", label: "problems found by checks", count: () => props.summary.problems.length },
  { value: "new", label: "new since the last scan", count: () => props.summary.added.length },
  { value: "changed", label: "changed", count: () => props.summary.changed.length },
  { value: "resolved", label: "resolved", count: () => props.summary.resolved.length },
  { value: "accepted", label: "accepted (hidden from the assistant)", count: () => props.summary.accepted.length, dashed: true },
]
const CHIPS: { value: FindingFilter; label: string }[] = [
  { value: "all", label: "All" }, { value: "problems", label: "Problems" }, { value: "new", label: "New" }, { value: "changed", label: "Changed" },
  { value: "accepted", label: "Accepted" }, { value: "resolved", label: "Resolved" },
]
const countOf = (value: FindingFilter): number => value === "all" ? props.report.findings.length : TILES.find((t) => t.value === value)!.count()
const chipOptions = computed(() => CHIPS.map((c) => ({ value: c.value, label: `${c.label} (${countOf(c.value)})` })))

// The problems a filter lists, grouped; every unaccepted problem is in the Problems block, so a device list never repeats one.
const problemGroups = computed(() => {
  switch (filter.value) {
    case "all":
    case "problems": return props.summary.problemGroups
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
const pick = (value: FindingFilter) => { filter.value = filter.value === value ? "all" : value }
// Something needs a look on a device when a port is new or changed (never on the first scan, where all are the baseline), or it has an unaccepted problem.
const needsLook = (device: string) =>
  props.summary.problems.some((f) => f.device === device) || (!props.summary.baseline && [...props.summary.added, ...props.summary.changed].some((f) => f.device === device))
const expectedOf = (device: string) => props.expectations.find((e) => e.device === device)
const problemsOn = (device: string) => props.summary.problems.filter((f) => f.device === device).length
const isOpen = (device: string) => toggled.value[device] ?? (filter.value !== "all" || needsLook(device))
const toggle = (device: string) => { toggled.value = { ...toggled.value, [device]: !isOpen(device) } }
</script>

<template>
  <div class="flex flex-col gap-4" data-test="findings-advanced">
    <div class="grid grid-cols-2 gap-2 lg:grid-cols-5">
      <button
        v-for="tile in TILES"
        :key="tile.value"
        type="button"
        :aria-pressed="filter === tile.value"
        :class="['flex min-h-11 flex-col items-start gap-0.5 rounded-[var(--radius-md)] border px-3 py-2 text-left focus-visible:outline-none focus-visible:ring-2', tile.dashed ? 'border-dashed' : '', filter === tile.value ? 'border-accent-info bg-surface-active' : 'border-control hover:bg-surface-hover']"
        @click="pick(tile.value)"
      >
        <span class="text-[22px] font-semibold leading-none text-primary">{{ tile.count() }}</span>
        <span class="text-[length:var(--text-small)] text-muted">{{ tile.label }}</span>
      </button>
    </div>

    <slot name="notice" />

    <p v-if="summary.baseline" class="m-0 text-[length:var(--text-small)] text-secondary" data-test="baseline-note">
      This is the first scan. What it found is saved as the baseline; from the next scan only changes are marked as new.
    </p>

    <SegmentedControl :model-value="filter" :options="chipOptions" label="Show findings" @update:model-value="setFilter" />

    <ProblemGroups :groups="problemGroups" advanced @accept="emit('accept', $event)" @mark="emit('mark', $event)" @unmark="emit('unmark', $event)" @rescan="emit('rescan', $event)" />

    <template v-if="showDevices">
      <p v-if="!groups.length && !problemGroups.length" class="m-0 text-muted">Nothing to show here.</p>
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
