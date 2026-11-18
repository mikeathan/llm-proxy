<script setup lang="ts">
import { computed, ref } from "vue"
import RadioAroundRadar from "./RadioAroundRadar.vue"
import {
  RADIO_AROUND_CHANGES_TITLE, RADIO_CHANGES_NONE, RADIO_CHANGES_PICK, RADIO_CHANGE_FILTERS, RADIO_CHANGE_WORDS, radioAroundChangeLine, radioRangeLine,
} from "../../constants/radio"
import { changesOf } from "../../utils/radio/around"
import { deviceFacts, deviceTitle, heardAgoWord } from "../../utils/radio/describe"
import type { RadioAroundState, RadioChange } from "../../types/radio"

// Radio · view "What changed": new devices, devices unlike their usual and devices that are usually here and not heard now, most important first. A
// tap opens that device's own page. The stored history is by the day, so "new" means first heard today.
const props = defineProps<{ around: RadioAroundState; today: string; now: number }>()
const kind = ref<"all" | RadioChange["kind"]>("all")
const changes = computed(() => changesOf(props.around.everyone.value, props.today).filter((c) => kind.value === "all" || c.kind === kind.value))
const TONE: Record<RadioChange["kind"], string> = { unusual: "border-state-running text-state-running", arrival: "border-state-success text-state-success", departure: "border-hairline text-muted" }
</script>

<template>
  <div class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,26rem)]" data-test="radio-view-changes">
    <div class="flex min-w-0 flex-col gap-2">
      <h3 class="m-0 text-[length:var(--text-small)] font-medium text-secondary">{{ RADIO_AROUND_CHANGES_TITLE }}</h3>
      <div role="group" :aria-label="RADIO_AROUND_CHANGES_TITLE" class="flex flex-wrap gap-1.5">
        <button v-for="f in RADIO_CHANGE_FILTERS" :key="f.id" type="button" :aria-pressed="kind === f.id" class="inline-flex min-h-11 items-center rounded-[var(--radius-md)] border px-3 text-[length:var(--text-small)] focus-visible:outline-none focus-visible:ring-2 sm:min-h-8" :class="kind === f.id ? 'border-accent-brand bg-surface-active text-primary' : 'border-hairline text-secondary hover:bg-surface-hover'" data-test="radio-change-filter" @click="kind = f.id">{{ f.label }}</button>
      </div>
      <p v-if="!changes.length" class="m-0 text-secondary" data-test="radio-changes-none">{{ RADIO_CHANGES_NONE }}</p>
      <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0" data-test="radio-changes">
        <li v-for="c in changes" :key="c.device.id + c.kind">
          <button type="button" class="flex min-h-11 w-full items-start justify-between gap-3 py-2 text-left hover:text-primary focus-visible:outline-none focus-visible:ring-2" data-test="radio-change" @click="around.open(c.device.id)">
            <span class="flex min-w-0 flex-col gap-0.5">
              <span class="truncate text-secondary">{{ deviceTitle(c.device) }} <span class="font-mono text-[length:var(--text-small)] text-faint">{{ c.device.id }}</span></span>
              <span class="text-[length:var(--text-small)] text-muted">{{ radioAroundChangeLine(c.kind) }} · {{ deviceFacts(c.device).join(" · ") }}</span>
            </span>
            <span class="flex flex-none flex-col items-end gap-0.5 text-[length:var(--text-small)]">
              <span class="rounded-full border px-2" :class="TONE[c.kind]">{{ RADIO_CHANGE_WORDS[c.kind] }}</span>
              <span class="font-mono text-primary">{{ radioRangeLine(c.device.near_m, c.device.far_m) }}</span>
              <span v-if="heardAgoWord(c.device, now)" class="text-faint">{{ heardAgoWord(c.device, now) }}</span>
            </span>
          </button>
        </li>
      </ul>
      <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_CHANGES_PICK }}</p>
    </div>
    <RadioAroundRadar :around="around" />
  </div>
</template>
