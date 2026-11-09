<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Disclosure from "../common/layout/Disclosure.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import ProblemGroups from "./ProblemGroups.vue"
import ProblemsByDevice from "./ProblemsByDevice.vue"
import { useNetauditHelp } from "../../composables/netaudit/useNetauditHelp"
import { useProblemLayout } from "../../composables/netaudit/useProblemLayout"
import { describePort, describeResolved, devicesIn, isCheck, isPlainText, lookSentence, needsLookSentence, portCount, seenSentence, shortTitle } from "../../utils/netaudit/friendly"
import { formatAbsoluteTime } from "../../utils/format/time"
import { groupByDevice, groupProblems, groupProblemsByDevice, splitProblems } from "../../utils/netaudit/report"
import type { FindingsReport, ProblemLayout, ReportSummary, ScanFinding } from "../../types/netaudit"
import type { ChoiceOption } from "../../types/ui"

// The Simple report: one plain line saying how many things need a look, the problems the checks found grouped by kind
// and any new or changed port, each with one decision ("This is fine"), and the rest as counts. The first scan lists no
// port as new: it says so, and keeps what it found one tap away. Everything below the problems is one quiet list per kind
// (ports to look at, marked as fine, gone), each folded to its count when it is long, so a phone shows the problems first.
// The problems themselves are split in two: "Needs you" (a serious kind, or anything new) in full, and "Notes" (quiet and known) folded
// to one line each, so a long list of the same small thing never buries the one that matters. "By device" is the same findings with one
// card per device. `names` maps an address to what the person calls the device.
const props = withDefaults(defineProps<{ report: FindingsReport; summary: ReportSummary; extraProblems?: readonly ScanFinding[]; names?: Readonly<Record<string, string>> }>(), { extraProblems: () => [], names: () => ({}) })
const emit = defineEmits<{
  (e: "accept", finding: ScanFinding): void
  (e: "accept-many", findings: ScanFinding[]): void
  (e: "revoke", finding: ScanFinding): void
  (e: "mark", finding: ScanFinding): void
  (e: "unmark", finding: ScanFinding): void
  (e: "rescan", device: string): void
}>()
const help = useNetauditHelp()
const { layout, set: setLayout } = useProblemLayout()
const LAYOUT_LABEL = "Group the problems"
const LAYOUT_OPTIONS: ChoiceOption[] = [{ value: "check", label: "By kind" }, { value: "device", label: "By device" }]
const onLayout = (value: string) => setLayout(value as ProblemLayout)
// LONG_LIST is the most rows a list shows before it starts folded to its count.
const LONG_LIST = 3
const portsOpen = ref<boolean | null>(null)
const acceptedOpen = ref(false)
const resolvedOpen = ref(false)
// Problems are listed by kind in ProblemGroups; what is left to look at here is a port that is new or changed.
// Problems a check found, and any from a deep check the audit has not carried into the report yet.
const problemCount = computed(() => props.summary.problems.length + props.extraProblems.length)
const allProblems = computed(() => [...props.summary.problems, ...props.extraProblems])
const problemGroups = computed(() => groupProblems(allProblems.value))
const split = computed(() => splitProblems(problemGroups.value))
const noteCount = computed(() => split.value.notes.reduce((n, g) => n + g.items.length, 0))
const deviceProblems = computed(() => groupProblemsByDevice(allProblems.value))
const portsToLook = computed(() => props.summary.needLook.filter((f) => !isCheck(f)))
// What needs the reader: the places of the problems that are not notes, and the ports that are new or changed. A note is counted on its own line.
const needCount = computed(() => split.value.needsYou.reduce((n, g) => n + g.items.length, 0) + portsToLook.value.length)
const portsShown = computed(() => portsOpen.value ?? portsToLook.value.length <= LONG_LIST)
const portsSummary = computed(() => `${portsToLook.value.length} new or changed`)
const inventoryOpen = ref(false)
const inventory = computed(() => groupByDevice(props.report.findings.filter((f) => !isCheck(f))))
const inventorySummary = computed(() => seenSentence(devicesIn(props.report.findings), portCount(props.report.findings)))
</script>

<template>
  <div class="flex flex-col gap-4" data-test="findings-simple">
    <p class="m-0 text-[15px] font-semibold text-primary" data-test="summary">{{ needsLookSentence(needCount) }}</p>
    <p v-if="problemCount" class="m-0 font-semibold text-state-running" data-test="problems">
      {{ problemCount }} {{ problemCount === 1 ? "problem" : "problems" }} found by checks
    </p>
    <p v-if="summary.baseline" class="m-0 text-[length:var(--text-small)] text-secondary" data-test="baseline-note">
      This is the first scan. What it found is saved as the baseline: from the next scan only what changed is listed here.
    </p>
    <div v-if="problemCount" class="flex justify-end" data-test="layout-switch">
      <SegmentedControl :model-value="layout" :options="LAYOUT_OPTIONS" :label="LAYOUT_LABEL" variant="compact" @update:model-value="onLayout" />
    </div>
    <ProblemsByDevice v-if="layout === 'device'" :devices="deviceProblems" :advanced="false" :names="names" @accept="emit('accept', $event)" @accept-many="emit('accept-many', $event)" />
    <template v-else>
      <section v-if="split.needsYou.length" aria-labelledby="needs-you-heading" class="flex flex-col gap-2" data-test="needs-you">
        <h3 id="needs-you-heading" class="m-0 font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">Needs you</h3>
        <ProblemGroups :groups="split.needsYou" :advanced="false" :names="names" @accept="emit('accept', $event)" @accept-many="emit('accept-many', $event)" @mark="emit('mark', $event)" @unmark="emit('unmark', $event)" @rescan="emit('rescan', $event)" />
      </section>
      <section v-if="split.notes.length" aria-labelledby="notes-heading" class="flex flex-col gap-2" data-test="notes">
        <h3 id="notes-heading" class="m-0 font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted" data-test="notes-heading">Notes · {{ noteCount }}</h3>
        <p class="m-0 text-[length:var(--text-small)] text-muted">Checked, low risk and nothing new. A note that turns into something worse moves up.</p>
        <ProblemGroups :groups="split.notes" :advanced="false" :names="names" notes @accept="emit('accept', $event)" @accept-many="emit('accept-many', $event)" @mark="emit('mark', $event)" @unmark="emit('unmark', $event)" @rescan="emit('rescan', $event)" />
      </section>
    </template>
    <Disclosure v-if="portsToLook.length" data-test="ports-to-look" title="Ports to look at" :summary="portsSummary" :open="portsShown" @update:open="portsOpen = $event">
      <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
        <li v-for="f in portsToLook" :key="f.key" class="flex items-center justify-between gap-3 py-1.5 first:pt-0 last:pb-0">
          <span class="flex min-w-0 flex-col gap-0.5">
            <span class="block break-words text-primary">{{ lookSentence(f) }}</span>
            <span v-if="isPlainText(f)" class="flex flex-wrap items-center gap-2 text-[length:var(--text-small)] text-state-error">
              It sends everything unencrypted.
              <BaseButton variant="ghost" size="sm" @click="help.show('words')">Learn more</BaseButton>
            </span>
          </span>
          <BaseButton variant="ghost" size="sm" class-name="flex-none max-sm:!h-11" @click="emit('accept', f)">This is fine</BaseButton>
        </li>
      </ul>
    </Disclosure>
    <Disclosure v-if="summary.baseline && inventory.length" data-test="inventory" title="What was found" :summary="inventorySummary" :open="inventoryOpen" @update:open="inventoryOpen = $event">
      <p v-for="g in inventory" :key="g.device" class="m-0 break-words text-[length:var(--text-small)] text-secondary"><span class="font-mono font-semibold text-primary">{{ g.device }}</span> {{ g.items.map(describePort).join(", ") }}</p>
    </Disclosure>
    <Disclosure v-if="summary.accepted.length" aria-label="Marked as fine" data-test="accepted" title="Marked as fine" :summary="String(summary.accepted.length)" :open="acceptedOpen" @update:open="acceptedOpen = $event">
      <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
        <li v-for="f in summary.accepted" :key="f.key" class="flex items-center justify-between gap-3 py-1 first:pt-0 last:pb-0">
          <span class="min-w-0 break-words text-[length:var(--text-small)] text-secondary">{{ shortTitle(f) }}<template v-if="f.accepted_note"> · {{ f.accepted_note }}</template></span>
          <BaseButton variant="ghost" size="sm" class-name="flex-none max-sm:!h-11" @click="emit('revoke', f)">Take back</BaseButton>
        </li>
      </ul>
    </Disclosure>
    <Disclosure v-if="summary.resolved.length" aria-label="Gone since last time" data-test="resolved" title="Gone since last time" :summary="String(summary.resolved.length)" :open="resolvedOpen" @update:open="resolvedOpen = $event">
      <p v-for="r in summary.resolved" :key="describeResolved(r)" class="m-0 break-words text-[length:var(--text-small)] text-secondary">
        {{ describeResolved(r) }}<template v-if="r.fixed_at"> <span class="text-state-success" data-test="fixed-confirmed">· fixed: you marked it on {{ formatAbsoluteTime(r.fixed_at) }} and the scan confirms it</span></template>
      </p>
    </Disclosure>
  </div>
</template>
