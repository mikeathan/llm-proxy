<script setup lang="ts">
import BaseButton from "../common/buttons/BaseButton.vue"
import FixGuideSteps from "./FixGuideSteps.vue"
import { inject } from "vue"
import { RECHECK_INJECTION } from "../../composables/netaudit/useRecheck"
import { FINDING_CAME_BACK_MANY, FINDING_CAME_BACK_ONCE, FINDING_THERE_SINCE, RECHECK_BUSY, RECHECK_BUTTON, RECHECK_COULD_NOT, RECHECK_FIXED, RECHECK_STILL_THERE, RECHECK_VERIFIED, RECHECK_VERIFIED_TAIL } from "../../constants/netaudit"
import { formatAbsoluteTime } from "../../utils/format/time"
import { isScannable } from "../../utils/netaudit/friendly"
import type { ScanFinding } from "../../types/netaudit"

// What to do about a problem a check found: the steps, what could go wrong and how to undo it, folded to one line; and the
// operator's own word that they did it. Nothing here changes a device. The guide is fixed text from the server; the app
// only records "I've done this" and the next scan says whether it worked. A group of the same problem shows the steps
// once above its places and passes `steps: false`, so each place keeps only its own record.
// `showMark` is whether the "I've done this" button is offered while nothing is recorded; a list that keeps it behind a "⋯" turns it on with the "⋯".
const props = withDefaults(defineProps<{ finding: ScanFinding; advanced: boolean; steps?: boolean; showMark?: boolean }>(), { steps: true, showMark: true })
const emit = defineEmits<{ (e: "mark", finding: ScanFinding): void; (e: "unmark", finding: ScanFinding): void; (e: "rescan", device: string): void }>()

const markedOn = () => formatAbsoluteTime(props.finding.fix_marked_at ?? "")

// "Check again" comes from the Findings screen that shows this card; a card shown elsewhere has none and offers no button.
const recheck = inject(RECHECK_INJECTION, null)
const canRecheck = () => !!recheck && !!props.finding.recheckable && !props.finding.verified_fixed_at
const checking = () => recheck?.busyKey.value === props.finding.key
const answer = () => recheck?.results[props.finding.key ?? ""]
const answerText = () => {
  const a = answer()
  if (!a) return ""
  if (a.outcome === "fixed") return RECHECK_FIXED
  return a.outcome === "still_there" ? RECHECK_STILL_THERE : RECHECK_COULD_NOT + a.reason
}
const cameBackText = () => {
  const n = props.finding.came_back ?? 0
  if (n === 0) return ""
  return n === 1 ? FINDING_CAME_BACK_ONCE : FINDING_CAME_BACK_MANY.replace("{n}", String(n))
}
</script>

<template>
  <div v-if="finding.guide" class="flex w-full min-w-0 flex-col gap-2 font-normal" data-test="fix-guide-block">
    <FixGuideSteps v-if="steps" :guide="finding.guide" :advanced="advanced" />

    <p v-if="finding.first_seen || cameBackText()" data-test="finding-history" class="text-[length:var(--text-small)] text-secondary">
      <span v-if="finding.first_seen">{{ FINDING_THERE_SINCE }} {{ formatAbsoluteTime(finding.first_seen) }}. </span>
      <span v-if="cameBackText()" class="font-semibold text-state-running">{{ cameBackText() }}</span>
    </p>
    <p v-if="finding.verified_fixed_at" data-test="verified-fixed" class="text-[length:var(--text-small)] font-semibold text-state-success">
      {{ RECHECK_VERIFIED }} {{ formatAbsoluteTime(finding.verified_fixed_at) }} {{ RECHECK_VERIFIED_TAIL }}
    </p>
    <div v-if="canRecheck()" class="flex flex-wrap items-center gap-2 text-[length:var(--text-small)]" data-test="recheck-block">
      <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" :disabled="!!recheck?.busyKey.value" data-test="recheck" @click="recheck?.run(finding.key ?? '')">{{ checking() ? RECHECK_BUSY : RECHECK_BUTTON }}</BaseButton>
      <span v-if="answer()" data-test="recheck-answer" class="text-secondary">{{ answerText() }}</span>
      <span v-else-if="recheck?.errors[finding.key ?? '']" role="alert" class="text-state-error">{{ recheck?.errors[finding.key ?? ""] }}</span>
    </div>

    <div v-if="!finding.fix_state && showMark" class="flex flex-wrap items-center gap-2">
      <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" @click="emit('mark', finding)">I've done this</BaseButton>
    </div>
    <div v-else-if="finding.fix_state" data-test="fix-state" class="flex flex-wrap items-center gap-2 text-[length:var(--text-small)]">
      <template v-if="finding.fix_state === 'marked'">
        <span class="text-secondary">You marked this as fixed on {{ markedOn() }}. The next scan will confirm it.</span>
      </template>
      <template v-else>
        <span class="font-semibold text-state-running">Still showing after you marked it fixed on {{ markedOn() }}. Check the steps again, or mark it as fine.</span>
        <BaseButton v-if="isScannable(finding.device)" variant="secondary" size="sm" class-name="max-sm:!h-11" @click="emit('rescan', finding.device)">Scan this device again</BaseButton>
      </template>
      <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" @click="emit('unmark', finding)">Take back</BaseButton>
    </div>
  </div>
</template>
