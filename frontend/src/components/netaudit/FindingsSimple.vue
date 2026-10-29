<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Disclosure from "../common/layout/Disclosure.vue"
import ProblemGroups from "./ProblemGroups.vue"
import { useNetauditHelp } from "../../composables/netaudit/useNetauditHelp"
import { checkCount, describePort, describeResolved, devicesIn, findingTitle, isCheck, isPlainText, lookSentence, needsLookSentence, portCount, seenSentence } from "../../utils/netaudit/friendly"
import { formatAbsoluteTime } from "../../utils/format/time"
import { groupByDevice, groupProblems } from "../../utils/netaudit/report"
import type { FindingsReport, ReportSummary, ScanFinding } from "../../types/netaudit"

// The Simple report: one plain line saying how many things need a look, the problems the checks found grouped by kind
// and any new or changed port, each with one decision ("This is fine"), and the rest as counts. The first scan lists no
// port as new: it says so, and keeps what it found one tap away.
const props = withDefaults(defineProps<{ report: FindingsReport; summary: ReportSummary; extraProblems?: readonly ScanFinding[] }>(), { extraProblems: () => [] })
const emit = defineEmits<{
  (e: "accept", finding: ScanFinding): void
  (e: "revoke", finding: ScanFinding): void
  (e: "mark", finding: ScanFinding): void
  (e: "unmark", finding: ScanFinding): void
  (e: "rescan", device: string): void
}>()
const help = useNetauditHelp()
const whatIs = findingTitle
// Problems are listed by kind in ProblemGroups; what is left to look at here is a port that is new or changed.
// Problems a check found, and any from a deep check the audit has not carried into the report yet.
const problemCount = computed(() => props.summary.problems.length + props.extraProblems.length)
const problemGroups = computed(() => groupProblems([...props.summary.problems, ...props.extraProblems]))
const portsToLook = computed(() => props.summary.needLook.filter((f) => !isCheck(f)))
const inventoryOpen = ref(false)
const inventory = computed(() => groupByDevice(props.report.findings.filter((f) => !isCheck(f))))
const inventorySummary = computed(() => seenSentence(devicesIn(props.report.findings), portCount(props.report.findings)))
</script>

<template>
  <div class="flex flex-col gap-4" data-test="findings-simple">
    <p class="m-0 text-[15px] font-semibold text-primary" data-test="summary">{{ needsLookSentence(summary.needLook.length + extraProblems.length) }}</p>
    <p v-if="problemCount" class="m-0 font-semibold text-state-running" data-test="problems">
      {{ problemCount }} {{ problemCount === 1 ? "problem" : "problems" }} found by checks
    </p>
    <p v-if="summary.baseline" class="m-0 text-[length:var(--text-small)] text-secondary" data-test="baseline-note">
      This is the first scan. What it found is saved as the baseline: from the next scan only what changed is listed here.
    </p>
    <ProblemGroups :groups="problemGroups" :advanced="false" @accept="emit('accept', $event)" @mark="emit('mark', $event)" @unmark="emit('unmark', $event)" @rescan="emit('rescan', $event)" />
    <ul v-if="portsToLook.length" class="m-0 flex list-none flex-col gap-2 p-0">
      <li v-for="f in portsToLook" :key="f.key" class="flex flex-wrap items-center justify-between gap-2 border-l-2 border-accent-info bg-surface px-3 py-2.5">
        <span class="flex min-w-0 flex-col gap-1">
          <span class="block break-words text-primary">{{ lookSentence(f) }}</span>
          <span v-if="isPlainText(f)" class="flex flex-wrap items-center gap-2 text-[length:var(--text-small)] text-state-error">
            It sends everything unencrypted.
            <BaseButton variant="ghost" size="sm" @click="help.show('words')">Learn more</BaseButton>
          </span>
        </span>
        <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" @click="emit('accept', f)">This is fine</BaseButton>
      </li>
    </ul>
    <Disclosure v-if="summary.baseline && inventory.length" data-test="inventory" title="What was found" :summary="inventorySummary" :open="inventoryOpen" @update:open="inventoryOpen = $event">
      <p v-for="g in inventory" :key="g.device" class="m-0 break-words text-[length:var(--text-small)] text-secondary"><span class="font-mono font-semibold text-primary">{{ g.device }}</span> {{ g.items.map(describePort).join(", ") }}</p>
    </Disclosure>
    <p class="m-0 text-[length:var(--text-small)] text-muted">
      Seen in total: {{ seenSentence(devicesIn(report.findings), portCount(report.findings), checkCount(report.findings)) }}<template v-if="summary.accepted.length">, {{ summary.accepted.length }} marked as fine</template>.
    </p>
    <section v-if="summary.accepted.length" aria-label="Marked as fine" class="flex flex-col gap-1">
      <p class="m-0 font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">Marked as fine</p>
      <div v-for="f in summary.accepted" :key="f.key" class="flex flex-wrap items-center justify-between gap-2">
        <span class="min-w-0 break-words text-secondary">{{ whatIs(f) }}<template v-if="f.accepted_note"> · {{ f.accepted_note }}</template></span>
        <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" @click="emit('revoke', f)">Take back</BaseButton>
      </div>
    </section>
    <section v-if="summary.resolved.length" aria-label="Gone since last time" class="flex flex-col gap-1">
      <p class="m-0 font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">Gone since last time</p>
      <p v-for="r in summary.resolved" :key="describeResolved(r)" class="m-0 break-words text-secondary">
        {{ describeResolved(r) }}<template v-if="r.fixed_at"> <span class="text-state-success" data-test="fixed-confirmed">· fixed: you marked it on {{ formatAbsoluteTime(r.fixed_at) }} and the scan confirms it</span></template>
      </p>
    </section>
  </div>
</template>
