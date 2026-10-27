<script setup lang="ts">
import BaseButton from "../common/buttons/BaseButton.vue"
import StatusTag from "../common/display/StatusTag.vue"
import { useNetauditHelp } from "../../composables/netaudit/useNetauditHelp"
import { checkCount, describeResolved, devicesIn, findingTitle, isCheck, isPlainText, lookSentence, needsLookSentence, portCount, seenSentence, severityState } from "../../utils/netaudit/friendly"
import { PLAIN_CARD_CLASS, SEVERITY_CARD_CLASS } from "../../constants/netaudit"
import type { FindingsReport, ReportSummary, ScanFinding } from "../../types/netaudit"

// The Simple report: one plain line saying how many things need a look, each of them with one decision ("This is
// fine"), and the rest as counts.
defineProps<{ report: FindingsReport; summary: ReportSummary }>()
const emit = defineEmits<{ (e: "accept", finding: ScanFinding): void; (e: "revoke", finding: ScanFinding): void }>()
const help = useNetauditHelp()
const whatIs = findingTitle
// A problem a check found has a thick coloured edge and tint, so it cannot be missed in a long list; a plain open port does not.
const cardClass = (f: ScanFinding) => (isCheck(f) ? `border-l-4 ${SEVERITY_CARD_CLASS[f.severity] ?? PLAIN_CARD_CLASS}` : "border-l-2 border-accent-info bg-surface")
</script>

<template>
  <div class="flex flex-col gap-4" data-test="findings-simple">
    <p class="m-0 text-[15px] font-semibold text-primary" data-test="summary">{{ needsLookSentence(summary.needLook.length) }}</p>
    <p v-if="summary.problems.length" class="m-0 font-semibold text-state-running" data-test="problems">
      {{ summary.problems.length }} {{ summary.problems.length === 1 ? "problem" : "problems" }} found by checks
    </p>
    <ul v-if="summary.needLook.length" class="m-0 flex list-none flex-col gap-2 p-0">
      <li v-for="f in summary.needLook" :key="f.key" :data-severity="isCheck(f) ? f.severity : undefined" :class="['flex flex-wrap items-center justify-between gap-2 px-3 py-2.5', cardClass(f)]">
        <span class="flex min-w-0 flex-col gap-1">
          <StatusTag v-if="isCheck(f)" :state="severityState(f.severity)" :label="`Problem · ${f.severity}`" class="self-start" />
          <span :class="['block break-words text-primary', isCheck(f) ? 'font-semibold' : '']">{{ lookSentence(f) }}</span>
          <span v-if="f.fix" class="block break-words text-[length:var(--text-small)] text-secondary" data-test="fix"><span class="font-semibold">How to fix it:</span> {{ f.fix }}</span>
          <span v-if="isPlainText(f)" class="flex flex-wrap items-center gap-2 text-[length:var(--text-small)] text-state-error">
            It sends everything unencrypted.
            <BaseButton variant="ghost" size="sm" @click="help.show('words')">Learn more</BaseButton>
          </span>
        </span>
        <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" @click="emit('accept', f)">This is fine</BaseButton>
      </li>
    </ul>
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
      <p v-for="r in summary.resolved" :key="describeResolved(r)" class="m-0 break-words text-secondary">{{ describeResolved(r) }}</p>
    </section>
  </div>
</template>
