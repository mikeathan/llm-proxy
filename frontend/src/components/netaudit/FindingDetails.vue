<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import type { FindingDetail } from "../../types/netaudit"

// The facts behind a finding as a short list: a label, then its value(s) (a file's path is monospace and wraps anywhere, so a long one never
// pushes the page sideways on a phone). Rows that share a label are one group, so twelve changed files say "Changed file" once. A long list
// shows its first values and a button for the rest. Presentation only.
const props = defineProps<{ details: readonly FindingDetail[] }>()

// SHOWN_VALUES is how many values show before the rest fold; one hidden value saves nothing, so a list folds only when it hides two or more.
const SHOWN_VALUES = 5
const MIN_HIDDEN = 2
const all = ref(false)
const folds = computed(() => props.details.length >= SHOWN_VALUES + MIN_HIDDEN)
const shown = computed(() => (folds.value && !all.value ? props.details.slice(0, SHOWN_VALUES) : props.details))
const hidden = computed(() => props.details.length - SHOWN_VALUES)
// Consecutive rows with one label become one group; the server writes a label's rows together.
const groups = computed(() => {
  const out: { label: string; values: string[] }[] = []
  for (const d of shown.value) {
    const last = out[out.length - 1]
    if (last?.label === d.label) last.values.push(d.value)
    else out.push({ label: d.label, values: [d.value] })
  }
  return out
})
</script>

<template>
  <div class="flex min-w-0 flex-col gap-1.5" data-test="finding-details">
    <dl class="m-0 flex flex-col gap-2">
      <div v-for="(g, i) in groups" :key="`${g.label}|${i}`" class="grid grid-cols-1 gap-x-4 gap-y-0.5 sm:grid-cols-[8rem_minmax(0,1fr)]" data-test="finding-detail">
        <dt class="text-[length:var(--text-small)] text-muted">{{ g.label }}</dt>
        <dd class="m-0 flex min-w-0 flex-col gap-0.5">
          <span v-for="(v, k) in g.values" :key="`${v}|${k}`" class="min-w-0 break-all font-mono text-[length:var(--text-small)] text-secondary" data-test="finding-detail-value">{{ v }}</span>
        </dd>
      </div>
    </dl>
    <div v-if="folds" class="flex">
      <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" data-test="details-toggle" :aria-expanded="all" @click="all = !all">{{ all ? "Show fewer" : `Show ${hidden} more` }}</BaseButton>
    </div>
  </div>
</template>
