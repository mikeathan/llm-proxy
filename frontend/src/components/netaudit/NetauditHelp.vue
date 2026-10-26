<script setup lang="ts">
import { ref, watch } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Disclosure from "../common/layout/Disclosure.vue"
import MicroLabel from "../common/display/MicroLabel.vue"
import { useDismissable } from "../../composables/ui/useDismissable"
import { useNetauditHelp } from "../../composables/netaudit/useNetauditHelp"
import { HELP_SECTIONS } from "../../constants/netaudit"
import type { NetauditHelpTopic } from "../../types/netaudit"

// The Help panel: every part of it folded to a line, the part that was asked about open. One per page.
const { open, topic, hide } = useNetauditHelp()
const TOPICS = Object.keys(HELP_SECTIONS) as NetauditHelpTopic[]
const opened = ref<Record<string, boolean>>({})
const panel = ref<HTMLElement | null>(null)
useDismissable(panel, open)

watch([open, topic], ([isOpen, on]) => {
  if (isOpen) opened.value = { [on]: true }
}, { immediate: true })
</script>

<template>
  <div v-if="open" class="fixed inset-0 z-40 flex justify-end bg-scrim/50">
    <aside ref="panel" role="dialog" aria-label="Help with network scans" class="flex h-full w-full max-w-md flex-col gap-3 overflow-y-auto border-l border-strong bg-surface p-4">
      <div class="flex items-center justify-between gap-2">
        <MicroLabel>Help</MicroLabel>
        <BaseButton variant="ghost" size="sm" icon="close" icon-only label="Close help" @click="hide" />
      </div>
      <Disclosure v-for="key in TOPICS" :key="key" :title="HELP_SECTIONS[key].title" :open="!!opened[key]" @update:open="opened[key] = $event">
        <p v-for="paragraph in HELP_SECTIONS[key].paragraphs" :key="paragraph" class="m-0 text-secondary">{{ paragraph }}</p>
      </Disclosure>
    </aside>
  </div>
</template>
