<script setup lang="ts">
import { ref, useId } from "vue"
import { MORE_ABOUT_THIS } from "../../constants/labels"
import { MODULE_SWITCHED_OFF_NOTE, MODULE_SWITCHED_ON_NOTE, MODULES_LINK_LABEL } from "../../constants/stacks"
import { toNetwork } from "../../router/routes"

// What a module's detail card shows where its switch used to be: whether it is on (in words), a link to the one place it is switched (the
// stack's Modules tab), and the explanation the switch carried (one short hint, a longer text folded under an expander).
defineProps<{ enabled: boolean; hint?: string; details?: string }>()

const id = useId()
const detailsId = `${id}-details`
const open = ref(false)
</script>

<template>
  <div class="flex min-w-0 flex-col gap-0.5" data-test="module-switch-note">
    <p class="m-0 text-[length:var(--text-small)] text-secondary" data-test="module-switch-state">
      <span :class="enabled ? 'text-state-success' : 'text-muted'">{{ enabled ? MODULE_SWITCHED_ON_NOTE : MODULE_SWITCHED_OFF_NOTE }}</span>
      <RouterLink :to="toNetwork('modules')" class="text-accent-info-text underline-offset-2 hover:underline">{{ MODULES_LINK_LABEL }}</RouterLink>
    </p>
    <p v-if="hint" class="m-0 text-[length:var(--text-small)] text-faint">{{ hint }}</p>
    <template v-if="details">
      <button
        type="button"
        data-test="toggle-details"
        :aria-expanded="open"
        :aria-controls="detailsId"
        class="inline-flex min-h-11 items-center self-start rounded-[var(--radius-sm)] text-[length:var(--text-small)] text-accent-info-text underline-offset-2 hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring sm:min-h-8"
        @click="open = !open"
      >{{ MORE_ABOUT_THIS }}</button>
      <p v-show="open" :id="detailsId" data-test="toggle-details-text" class="m-0 text-[length:var(--text-small)] text-muted">{{ details }}</p>
    </template>
  </div>
</template>
