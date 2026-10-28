<script setup lang="ts">
import BaseButton from "../common/buttons/BaseButton.vue"
import FixGuideSteps from "./FixGuideSteps.vue"
import { formatAbsoluteTime } from "../../utils/format/time"
import type { ScanFinding } from "../../types/netaudit"

// What to do about a problem a check found: the steps, what could go wrong and how to undo it, folded to one line; and the
// operator's own word that they did it. Nothing here changes a device. The guide is fixed text from the server; the app
// only records "I've done this" and the next scan says whether it worked. A group of the same problem shows the steps
// once above its places and passes `steps: false`, so each place keeps only its own record.
const props = withDefaults(defineProps<{ finding: ScanFinding; advanced: boolean; steps?: boolean }>(), { steps: true })
const emit = defineEmits<{ (e: "mark", finding: ScanFinding): void; (e: "unmark", finding: ScanFinding): void; (e: "rescan", device: string): void }>()

const markedOn = () => formatAbsoluteTime(props.finding.fix_marked_at ?? "")
</script>

<template>
  <div v-if="finding.guide" class="flex w-full min-w-0 flex-col gap-2 font-normal" data-test="fix-guide-block">
    <FixGuideSteps v-if="steps" :guide="finding.guide" :advanced="advanced" />

    <div v-if="!finding.fix_state" class="flex flex-wrap items-center gap-2">
      <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" @click="emit('mark', finding)">I've done this</BaseButton>
    </div>
    <div v-else data-test="fix-state" class="flex flex-wrap items-center gap-2 text-[length:var(--text-small)]">
      <template v-if="finding.fix_state === 'marked'">
        <span class="text-secondary">You marked this as fixed on {{ markedOn() }}. The next scan will confirm it.</span>
      </template>
      <template v-else>
        <span class="font-semibold text-state-running">Still showing after you marked it fixed on {{ markedOn() }}. Check the steps again, or mark it as fine.</span>
        <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" @click="emit('rescan', finding.device)">Scan this device again</BaseButton>
      </template>
      <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" @click="emit('unmark', finding)">Take back</BaseButton>
    </div>
  </div>
</template>
