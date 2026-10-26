<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import StatusTag from "../common/display/StatusTag.vue"
import { checkCount, describePort, isCheck, isGuessedService, portCount, serviceName, severityState } from "../../utils/netaudit/friendly"
import { groupByDevice } from "../../utils/netaudit/report"
import { formatAbsoluteTime } from "../../utils/format/time"
import { PLAIN_CARD_CLASS, SEVERITY_CARD_CLASS } from "../../constants/netaudit"
import type { FindingFilter, FindingsReport, ReportSummary, ScanFinding } from "../../types/netaudit"

// The Advanced report: a tile for each part (they filter the list), then every device with its ports, services and
// versions, and Accept… or Undo on each. Both views read the same data.
const props = defineProps<{ report: FindingsReport; summary: ReportSummary }>()
const emit = defineEmits<{ (e: "accept", finding: ScanFinding): void; (e: "revoke", finding: ScanFinding): void }>()

const filter = ref<FindingFilter>("all")

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

const visible = computed<ScanFinding[]>(() => {
  switch (filter.value) {
    case "problems": return props.summary.problems
    case "new": return props.summary.added
    case "changed": return props.summary.changed
    case "accepted": return props.summary.accepted
    case "resolved": return []
    default: return props.report.findings
  }
})
const groups = computed(() => groupByDevice(visible.value))
const resolvedGroups = computed(() => groupByDevice(props.summary.resolved))
const portsLabel = (n: number) => `${n} open ${n === 1 ? "port" : "ports"}`
const setFilter = (value: string) => { filter.value = value as FindingFilter }
const pick = (value: FindingFilter) => { filter.value = filter.value === value ? "all" : value }
const stateOf = (f: ScanFinding) => (f.accepted ? "accepted" : f.new ? "new" : f.changed ? "changed" : "unchanged")
const STATE_CLASS: Record<string, string> = {
  new: "bg-accent-info/15 text-accent-info-text",
  changed: "border border-control text-primary",
  accepted: "border border-dashed border-control text-secondary",
  unchanged: "text-muted",
}
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

    <SegmentedControl :model-value="filter" :options="chipOptions" label="Show findings" @update:model-value="setFilter" />

    <template v-if="filter !== 'resolved'">
      <p v-if="!groups.length" class="m-0 text-muted">Nothing to show here.</p>
      <section v-for="g in groups" :key="g.device" :aria-label="g.device" class="min-w-0 rounded-[var(--radius-md)] border border-hairline">
        <header class="flex flex-wrap items-center justify-between gap-2 bg-surface-raised px-3 py-2">
          <span class="min-w-0 break-all"><span class="font-mono font-semibold text-primary">{{ g.device }}</span> <span class="text-muted">· {{ portsLabel(portCount(g.items)) }}<template v-if="checkCount(g.items)">, {{ checkCount(g.items) }} {{ checkCount(g.items) === 1 ? "problem" : "problems" }}</template></span></span>
          <span class="flex gap-1.5 text-[length:var(--text-micro)] text-muted">
            <span v-if="g.items.some((f) => f.new && !f.accepted)">{{ g.items.filter((f) => f.new && !f.accepted).length }} new</span>
            <span v-if="g.items.some((f) => f.changed && !f.accepted)">{{ g.items.filter((f) => f.changed && !f.accepted).length }} changed</span>
          </span>
        </header>
        <ul class="m-0 list-none p-0">
          <li v-for="f in g.items" :key="f.key" :data-key="f.key" :class="['flex flex-wrap items-center gap-x-4 gap-y-1 border-t border-hairline px-3 py-2', isCheck(f) ? `border-l-4 ${SEVERITY_CARD_CLASS[f.severity] ?? PLAIN_CARD_CLASS}` : '']">
            <span class="w-20 flex-none font-mono text-[length:var(--text-small)] text-primary">{{ f.port }}/{{ f.proto || "tcp" }}</span>
            <span v-if="isCheck(f)" class="flex min-w-0 flex-1 flex-wrap items-center gap-2 break-words text-[length:var(--text-small)] font-semibold text-primary" data-test="check">
              <StatusTag :state="severityState(f.severity)" :label="`Problem · ${f.severity}`" />
              {{ f.evidence || "A check found a problem" }}
            </span>
            <span v-else class="min-w-0 flex-1 break-words font-mono text-[length:var(--text-small)] text-secondary">
              {{ serviceName(f.service) }}<template v-if="isGuessedService(f.service)"> (guess)</template> ·
              <template v-if="f.changed && f.previous_version"><s class="text-muted">{{ f.previous_version }}</s> </template>{{ f.version || "version unknown" }}
            </span>
            <span :class="['inline-flex min-h-5 items-center px-2 font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)]', STATE_CLASS[stateOf(f)]]">
              <template v-if="f.accepted">Accepted until {{ formatAbsoluteTime(f.accepted_until ?? "") }}<template v-if="f.accepted_note"> · “{{ f.accepted_note }}”</template></template>
              <template v-else>{{ stateOf(f) }}</template>
            </span>
            <BaseButton v-if="f.accepted" variant="secondary" size="sm" class-name="max-sm:!h-11" @click="emit('revoke', f)">Undo</BaseButton>
            <BaseButton v-else variant="secondary" size="sm" class-name="max-sm:!h-11" @click="emit('accept', f)">Accept…</BaseButton>
          </li>
        </ul>
      </section>
    </template>

    <template v-else>
      <p v-if="!resolvedGroups.length" class="m-0 text-muted">Nothing has gone away since the scan before.</p>
      <section v-for="g in resolvedGroups" :key="g.device" :aria-label="g.device" class="rounded-[var(--radius-md)] border border-hairline">
        <header class="bg-surface-raised px-3 py-2 font-mono font-semibold text-primary">{{ g.device }}</header>
        <p v-for="r in g.items" :key="`${describePort(r)}|${r.check ?? ''}`" class="m-0 border-t border-hairline px-3 py-2 font-mono text-[length:var(--text-small)] text-secondary">{{ isCheck(r) ? (r.evidence || "A check finding") : describePort(r) }} <span class="text-muted">is no longer there</span></p>
      </section>
    </template>
  </div>
</template>
