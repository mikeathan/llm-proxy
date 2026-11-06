<script setup lang="ts">
import { ref, watch } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Disclosure from "../common/layout/Disclosure.vue"
import MicroLabel from "../common/display/MicroLabel.vue"
import { useDismissable } from "../../composables/ui/useDismissable"
import { useNetauditHelp } from "../../composables/netaudit/useNetauditHelp"
import { useModuleSources } from "../../composables/netaudit/useModuleSources"
import { HELP_SECTIONS } from "../../constants/netaudit"
import { HELP_MODULES_TITLE, MODULE_ABOUT_LABELS } from "../../constants/stacks"
import type { NetauditHelpTopic } from "../../types/netaudit"

// The Help panel: every part of it folded to a line, the part that was asked about open. One per page.
const { open, topic, hide } = useNetauditHelp()
// What each switch does is read from the modules themselves, so it is never written twice.
const { modules, load: loadModules } = useModuleSources()
const modulesOpen = ref(false)
const TOPICS = Object.keys(HELP_SECTIONS) as NetauditHelpTopic[]
const opened = ref<Record<string, boolean>>({})
const panel = ref<HTMLElement | null>(null)
useDismissable(panel, open)

watch([open, topic], ([isOpen, on]) => {
  if (isOpen) { opened.value = { [on]: true }; void loadModules() }
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
      <Disclosure v-if="modules.length" :title="HELP_MODULES_TITLE" :open="modulesOpen" data-test="help-modules" @update:open="modulesOpen = $event">
        <div v-for="m in modules" :key="m.id" class="flex flex-col gap-0.5 pb-2 text-secondary">
          <span class="font-medium text-primary">{{ m.title }}</span>
          <span v-for="(label, key) in MODULE_ABOUT_LABELS" :key="key"><span class="text-muted">{{ label }}:</span> {{ m.about[key] }}</span>
        </div>
      </Disclosure>
    </aside>
  </div>
</template>
