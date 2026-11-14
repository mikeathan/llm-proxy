<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted } from "vue"
import Panel from "../common/layout/Panel.vue"
import Callout from "../common/feedback/Callout.vue"
import { useRadioAir } from "../../composables/radio/useRadioAir"
import {
  RADIO_AROUND_EMPTY, RADIO_AROUND_TITLE, RADIO_CHANGES_TITLE, RADIO_CLASS_HELP, RADIO_CLASS_ORDER, RADIO_CLASS_WORDS, RADIO_RULES_PROVISIONAL,
  radioAroundTotal, radioChangeLine, radioListenedLine, radioRulesLine,
} from "../../constants/radio"

// Radio · Around you: other people's devices counted by kind. A count is all there is: no device of someone else is ever named or addressed.
// It refreshes while the page is open, because a run adds to it every cycle.
const a = useRadioAir()
const REFRESH_MS = 30000
let timer: ReturnType<typeof setInterval> | null = null

const air = computed(() => a.air.value)
const thin = computed(() => !!air.value && air.value.listened_cycles < air.value.rules.min_cycles_listened)

onMounted(() => {
  void a.load()
  timer = setInterval(() => { void a.load() }, REFRESH_MS)
})
onBeforeUnmount(() => { if (timer) clearInterval(timer) })
</script>

<template>
  <Panel :title="RADIO_AROUND_TITLE">
    <div class="flex flex-col gap-3" data-test="radio-around">
      <Callout v-if="a.error.value && !air" tone="error" title="Could not read"><span>{{ a.error.value }}</span></Callout>
      <template v-else-if="air">
        <p v-if="!air.total && !air.listened_cycles" class="m-0 text-secondary" data-test="radio-around-empty">{{ RADIO_AROUND_EMPTY }}</p>
        <template v-else>
          <p class="m-0 text-primary" data-test="radio-around-total">{{ radioAroundTotal(air.total) }}</p>
          <ul class="m-0 grid list-none grid-cols-1 gap-x-6 gap-y-2 p-0 sm:grid-cols-2">
            <li v-for="c in RADIO_CLASS_ORDER" :key="c" class="flex items-baseline justify-between gap-3" data-test="radio-class" :data-class="c">
              <span class="flex min-w-0 flex-col"><span class="text-primary">{{ RADIO_CLASS_WORDS[c] }}</span><span class="text-[length:var(--text-small)] text-muted">{{ RADIO_CLASS_HELP[c] }}</span></span>
              <span class="flex-none font-mono text-primary">{{ air.counts[c] ?? 0 }}</span>
            </li>
          </ul>
        </template>
        <p class="m-0 text-[length:var(--text-small)]" :class="thin ? 'text-state-running' : 'text-muted'" data-test="radio-around-listened">{{ radioListenedLine(air.listened_cycles, air.rules.min_cycles_listened, air.rules.window_days) }}</p>
        <p v-if="air.rules.provisional" class="m-0 text-[length:var(--text-small)] text-muted" data-test="radio-around-provisional">{{ RADIO_RULES_PROVISIONAL }} {{ radioRulesLine(air.rules.version) }}</p>
        <div v-if="air.changes.length" class="flex flex-col gap-1">
          <h3 class="m-0 text-[length:var(--text-small)] font-medium text-secondary">{{ RADIO_CHANGES_TITLE }}</h3>
          <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
            <li v-for="c in air.changes.slice(0, 6)" :key="c.day + c.from + c.to + c.reason" class="py-1.5 text-[length:var(--text-small)] text-secondary" data-test="radio-change">
              <span class="text-muted">{{ c.day }}</span> · {{ radioChangeLine(c.n, c.from, c.to, c.reason) }}
            </li>
          </ul>
        </div>
      </template>
    </div>
  </Panel>
</template>
