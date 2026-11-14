<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Callout from "../common/feedback/Callout.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import FormField from "../common/forms/FormField.vue"
import { useRadioScanPlan } from "../../composables/radio/useRadioScanPlan"
import {
  RADIO_SCAN_CHANGE, RADIO_SCAN_CHANNELS, RADIO_SCAN_CLOSE, RADIO_SCAN_CYCLE, RADIO_SCAN_INTRO, RADIO_SCAN_NONE, RADIO_SCAN_NO_FIT, RADIO_SCAN_RESET,
  RADIO_SCAN_RUNNING, RADIO_SCAN_SAVE, RADIO_SCAN_SAVED, RADIO_SCAN_TITLE, RADIO_SCAN_WINDOW, RADIO_SCAN_WINDOW_CHOICES, RADIO_SCAN_CYCLE_CHOICES,
  radioBand, radioScanSummary, radioScanTooMany, radioSecondsLabel,
} from "../../constants/radio"

// Radio · Scan plan: the channels a listening run visits, the time on each and how often it comes round. The server checks every change and says
// in a sentence what is wrong; the same checks run here so Save is only offered for a plan that will be accepted. A running run keeps the plan it
// started with, so the editor is closed to it: stopping is what makes a change possible.
const p = useRadioScanPlan()
const open = ref(false)
const channels = ref<number[]>([])
const windowSeconds = ref("20")
const cycleSeconds = ref("300")

const scan = computed(() => p.scan.value)
const limits = computed(() => scan.value?.limits)
const groups = computed(() => {
  const all = limits.value?.channels ?? []
  return [
    { band: radioBand(1), list: all.filter((c) => c <= 14) },
    { band: radioBand(36), list: all.filter((c) => c > 14) },
  ]
})
const choices = (base: number[], current: number): number[] => [...new Set([...base, current])].sort((a, b) => a - b)
const windowOptions = computed(() => choices(RADIO_SCAN_WINDOW_CHOICES, Number(windowSeconds.value)).map((v) => ({ value: String(v), label: `${v} seconds` })))
const cycleOptions = computed(() => choices(RADIO_SCAN_CYCLE_CHOICES, Number(cycleSeconds.value)).map((v) => ({ value: String(v), label: radioSecondsLabel(v) })))
const perHour = computed(() => Math.floor((channels.value.length * 3600) / Math.max(Number(cycleSeconds.value), 1)))
const problem = computed(() => {
  if (!limits.value) return ""
  if (!channels.value.length) return RADIO_SCAN_NONE
  if (channels.value.length > limits.value.max_channels || perHour.value > limits.value.max_windows_per_hour) return radioScanTooMany(limits.value.max_windows_per_hour)
  if (channels.value.length * Number(windowSeconds.value) > Number(cycleSeconds.value)) return RADIO_SCAN_NO_FIT
  return ""
})

function fill(): void {
  const plan = scan.value?.plan
  if (!plan) return
  channels.value = [...plan.channels]
  windowSeconds.value = String(plan.window_seconds)
  cycleSeconds.value = String(plan.cycle_seconds)
}
function openClose(): void { open.value = !open.value; if (open.value) void p.load() }
function toggle(c: number): void { channels.value = channels.value.includes(c) ? channels.value.filter((x) => x !== c) : [...channels.value, c].sort((a, b) => a - b) }
function save(): void { void p.save({ channels: channels.value, window_seconds: Number(windowSeconds.value), cycle_seconds: Number(cycleSeconds.value) }) }
function reset(): void { void p.reset().then(fill) }

watch(scan, fill)
onMounted(() => { void p.load() })
</script>

<template>
  <Panel :title="RADIO_SCAN_TITLE">
    <div class="flex flex-col gap-3" data-test="radio-scan">
      <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_SCAN_INTRO }}</p>
      <Callout v-if="p.error.value && !scan" tone="error" title="Could not read"><span data-test="radio-scan-error">{{ p.error.value }}</span></Callout>
      <template v-else-if="scan">
        <p class="m-0 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 text-primary">
          <span data-test="radio-scan-summary">{{ radioScanSummary(scan.plan.channels.length, scan.plan.window_seconds, scan.plan.cycle_seconds, scan.windows_per_hour, scan.custom) }}</span>
          <BaseButton variant="secondary" class-name="max-sm:!h-11" :aria-expanded="open" data-test="radio-scan-toggle" @click="openClose">{{ open ? RADIO_SCAN_CLOSE : RADIO_SCAN_CHANGE }}</BaseButton>
        </p>
        <div v-if="open" class="flex flex-col gap-3" data-test="radio-scan-editor">
          <p v-if="p.running.value" class="m-0 text-state-running" data-test="radio-scan-running">{{ RADIO_SCAN_RUNNING }}</p>
          <div v-for="g in groups" :key="g.band" class="flex flex-col gap-1">
            <span class="text-[length:var(--text-small)] text-secondary">{{ RADIO_SCAN_CHANNELS }}: {{ g.band }}</span>
            <div class="flex flex-wrap gap-2">
              <button
                v-for="c in g.list" :key="c" type="button" :aria-pressed="channels.includes(c)" :disabled="p.running.value" :data-channel="c" data-test="radio-scan-channel"
                class="min-h-11 min-w-11 rounded-[var(--radius-md)] border px-2 font-mono text-[length:var(--text-small)] focus-visible:outline-none focus-visible:ring-2 disabled:opacity-60"
                :class="channels.includes(c) ? 'border-accent-brand bg-surface-active text-primary' : 'border-hairline text-secondary hover:bg-surface-hover'"
                @click="toggle(c)"
              >{{ c }}</button>
            </div>
          </div>
          <div class="flex flex-wrap items-end gap-3">
            <FormField :label="RADIO_SCAN_WINDOW"><template #default><SelectInput v-model="windowSeconds" :options="windowOptions" :label="RADIO_SCAN_WINDOW" /></template></FormField>
            <FormField :label="RADIO_SCAN_CYCLE"><template #default><SelectInput v-model="cycleSeconds" :options="cycleOptions" :label="RADIO_SCAN_CYCLE" /></template></FormField>
          </div>
          <p class="m-0 text-[length:var(--text-small)]" :class="problem ? 'text-state-error' : 'text-secondary'" data-test="radio-scan-check">{{ problem || radioScanSummary(channels.length, Number(windowSeconds), Number(cycleSeconds), perHour, true) }}</p>
          <Callout v-if="p.error.value" tone="error" title="Not saved"><span data-test="radio-scan-save-error">{{ p.error.value }}</span></Callout>
          <p v-if="p.saved.value" class="m-0 text-state-success" data-test="radio-scan-saved">{{ RADIO_SCAN_SAVED }}</p>
          <div class="flex flex-wrap gap-2">
            <BaseButton variant="primary" class-name="max-sm:!h-11" :disabled="!!problem || p.busy.value || p.running.value" data-test="radio-scan-save" @click="save">{{ RADIO_SCAN_SAVE }}</BaseButton>
            <BaseButton v-if="scan.custom" variant="secondary" class-name="max-sm:!h-11" :disabled="p.busy.value || p.running.value" data-test="radio-scan-reset" @click="reset">{{ RADIO_SCAN_RESET }}</BaseButton>
          </div>
        </div>
      </template>
    </div>
  </Panel>
</template>
