<script setup lang="ts">
import { ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import CopyButton from "../common/display/CopyButton.vue"
import Disclosure from "../common/layout/Disclosure.vue"
import { formatAbsoluteTime } from "../../utils/format/time"
import type { ScanFinding } from "../../types/netaudit"

// What to do about a problem a check found: the steps, what could go wrong and how to undo it, folded to one line; and the
// operator's own word that they did it. Nothing here changes a device. The guide is fixed text from the server; the app
// only records "I've done this" and the next scan says whether it worked. Simple shows the steps; Advanced adds commands.
const props = defineProps<{ finding: ScanFinding; advanced: boolean }>()
const emit = defineEmits<{ (e: "mark", finding: ScanFinding): void; (e: "unmark", finding: ScanFinding): void; (e: "rescan", device: string): void }>()

const open = ref(false)
const markedOn = () => formatAbsoluteTime(props.finding.fix_marked_at ?? "")
</script>

<template>
  <div v-if="finding.guide" class="flex w-full min-w-0 flex-col gap-2 font-normal" data-test="fix-guide-block">
    <Disclosure data-test="fix-guide" title="How to fix it" summary="steps, risks and how to undo" :open="open" @update:open="open = $event">
      <ol class="m-0 flex list-decimal flex-col gap-1 pl-5 text-[length:var(--text-small)] text-secondary">
        <li v-for="step in finding.guide.steps" :key="step" class="break-words">{{ step }}</li>
      </ol>
      <div v-if="advanced && finding.guide.commands?.length" class="flex flex-col gap-2">
        <div v-for="c in finding.guide.commands" :key="c.platform" data-test="fix-command" class="flex min-w-0 flex-col gap-1">
          <span class="flex items-center justify-between gap-2 font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">
            {{ c.platform }}
            <CopyButton :text="c.command" :title="`Copy the ${c.platform} command`" />
          </span>
          <pre class="m-0 overflow-x-auto whitespace-pre-wrap break-all bg-canvas p-2 font-mono text-[length:var(--text-small)] text-primary">{{ c.command }}</pre>
        </div>
      </div>
      <p class="m-0 text-[length:var(--text-small)] text-secondary"><span class="font-semibold">What could go wrong:</span> {{ finding.guide.risk }}</p>
      <p class="m-0 text-[length:var(--text-small)] text-secondary"><span class="font-semibold">How to undo it:</span> {{ finding.guide.undo }}</p>
    </Disclosure>

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
