<script setup lang="ts">
import { ref } from "vue"
import CopyButton from "../common/display/CopyButton.vue"
import Disclosure from "../common/layout/Disclosure.vue"
import type { FixGuide } from "../../types/netaudit"

// The steps, commands, risk and way back of a fix, folded to one line. Fixed text from the server; nothing here runs a
// command or changes a device. Simple shows the steps; Advanced adds the commands to copy.
defineProps<{ guide: FixGuide; advanced: boolean }>()

const open = ref(false)
</script>

<template>
  <Disclosure data-test="fix-guide" title="How to fix it" summary="steps, risks and how to undo" :open="open" class="font-normal" @update:open="open = $event">
    <ol class="m-0 flex list-decimal flex-col gap-1 pl-5 text-[length:var(--text-small)] text-secondary">
      <li v-for="step in guide.steps" :key="step" class="break-words">{{ step }}</li>
    </ol>
    <div v-if="advanced && guide.commands?.length" class="flex flex-col gap-2">
      <div v-for="c in guide.commands" :key="c.platform" data-test="fix-command" class="flex min-w-0 flex-col gap-1">
        <span class="flex items-center justify-between gap-2 font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">
          {{ c.platform }}
          <CopyButton :text="c.command" :title="`Copy the ${c.platform} command`" />
        </span>
        <pre class="m-0 overflow-x-auto whitespace-pre-wrap break-all bg-canvas p-2 font-mono text-[length:var(--text-small)] text-primary">{{ c.command }}</pre>
      </div>
    </div>
    <p class="m-0 text-[length:var(--text-small)] text-secondary"><span class="font-semibold">What could go wrong:</span> {{ guide.risk }}</p>
    <p class="m-0 text-[length:var(--text-small)] text-secondary"><span class="font-semibold">How to undo it:</span> {{ guide.undo }}</p>
  </Disclosure>
</template>
