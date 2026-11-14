<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import Callout from "../common/feedback/Callout.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import { useRadioAir } from "../../composables/radio/useRadioAir"
import { useRadioAirDevices } from "../../composables/radio/useRadioAirDevices"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { usePolling } from "../../composables/ui/usePolling"
import {
  RADIO_AROUND_EMPTY, RADIO_CHANGES_TITLE, RADIO_CLASS_HELP, RADIO_CLASS_ORDER, RADIO_CLASS_WORDS, RADIO_DEVICE_COLUMNS, RADIO_JOINED_OWN,
  RADIO_NEIGH_CLOSE, RADIO_NEIGH_INTRO, RADIO_NEIGH_NONE, RADIO_NEIGH_OPEN, RADIO_NEIGH_TITLE, RADIO_RANDOM_ADDRESS, RADIO_SIGNAL_HELP,
  RADIO_SIGNAL_WORDS, RADIO_SORT_LABEL, RADIO_SORT_OPTIONS, RADIO_REASONS,
  radioAroundTotal, radioChangeLine, radioChannelsLine, radioHeardLine, radioListenedLine, radioProvisionalLine, radioRetryWord, radioWaitingLine, radioWaitingPercent,
} from "../../constants/radio"
import type { RadioAirDevice, RadioAirSort, RadioClass } from "../../types/radio"

// Radio · Neighbourhood: other people's devices by kind, and each kind opens onto its devices. A device is a short id (four bytes of a key that
// changes when the neighbours are forgotten) with a signal band and counts: never an address and never a name. It refreshes while open, because a
// run adds to it every cycle. Simple answers "who is here and what changed"; Advanced adds the evidence and a sort.
const a = useRadioAir()
const d = useRadioAirDevices()
const { advanced } = useNetauditViewMode()
const REFRESH_MS = 30000

const open = ref<RadioClass | null>(null)
const sort = ref<RadioAirSort | "">("")
const air = computed(() => a.air.value)
const thin = computed(() => !!air.value && air.value.listened_cycles < air.value.rules.min_cycles_listened)

function ask(): Promise<void> { return d.load(open.value ?? undefined, sort.value || undefined) }

async function toggle(c: RadioClass): Promise<void> {
  open.value = open.value === c ? null : c
  if (open.value) await ask()
}

async function resort(value: string): Promise<void> {
  sort.value = value as RadioAirSort
  await ask()
}

function waiting(x: RadioAirDevice): boolean { return x.why === "needs_more_listening" }

onMounted(() => { void a.load() })
usePolling(() => { void a.load(); if (open.value) void ask() }, REFRESH_MS)
</script>

<template>
  <Panel :title="RADIO_NEIGH_TITLE">
    <div class="flex flex-col gap-3" data-test="radio-around">
      <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_NEIGH_INTRO }}</p>
      <Callout v-if="a.error.value && !air" tone="error" title="Could not read"><span>{{ a.error.value }}</span></Callout>
      <template v-else-if="air">
        <p v-if="!air.total && !air.listened_cycles" class="m-0 text-secondary" data-test="radio-around-empty">{{ RADIO_AROUND_EMPTY }}</p>
        <template v-else>
          <p class="m-0 text-primary" data-test="radio-around-total">{{ radioAroundTotal(air.total) }}</p>
          <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
            <li v-for="c in RADIO_CLASS_ORDER" :key="c" data-test="radio-class" :data-class="c">
              <button
                type="button" class="flex min-h-11 w-full items-baseline justify-between gap-3 py-2 text-left hover:text-primary focus-visible:outline-none focus-visible:ring-2"
                :aria-expanded="open === c" :aria-label="`${open === c ? RADIO_NEIGH_CLOSE : RADIO_NEIGH_OPEN}: ${RADIO_CLASS_WORDS[c]}`" data-test="radio-class-open" @click="toggle(c)"
              >
                <span class="flex min-w-0 flex-col"><span class="text-primary">{{ RADIO_CLASS_WORDS[c] }}</span><span class="text-[length:var(--text-small)] text-muted">{{ RADIO_CLASS_HELP[c] }}</span></span>
                <span class="flex flex-none items-baseline gap-2"><span class="font-mono text-primary">{{ air.counts[c] ?? 0 }}</span><span aria-hidden="true" class="text-muted">{{ open === c ? "−" : "+" }}</span></span>
              </button>
              <div v-if="open === c" class="flex flex-col gap-2 pb-3" data-test="radio-device-list">
                <div v-if="advanced" class="flex items-center gap-2" data-test="radio-air-sort"><span class="text-[length:var(--text-small)] text-muted">{{ RADIO_SORT_LABEL }}</span><SelectInput :model-value="sort || 'last_heard'" :options="RADIO_SORT_OPTIONS" :label="RADIO_SORT_LABEL" @update:model-value="resort" /></div>
                <Callout v-if="d.error.value" tone="error" title="Could not read"><span data-test="radio-neigh-error">{{ d.error.value }}</span></Callout>
                <p v-else-if="d.list.value && !d.list.value.devices.length" class="m-0 text-secondary" data-test="radio-device-none">{{ RADIO_NEIGH_NONE }}</p>
                <ul v-else-if="d.list.value" class="m-0 flex list-none flex-col gap-2 p-0">
                  <li v-for="x in d.list.value.devices" :key="x.id" class="flex flex-col gap-1 rounded-[var(--radius-sm)] bg-canvas px-3 py-2 text-[length:var(--text-small)]" data-test="radio-air-device">
                    <div class="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5">
                      <span class="font-mono text-primary" :title="RADIO_DEVICE_COLUMNS.id">{{ x.id }}</span>
                      <span class="text-secondary"><span data-test="radio-air-signal" :title="RADIO_SIGNAL_HELP[x.signal]">{{ RADIO_SIGNAL_WORDS[x.signal] }}</span> · {{ RADIO_DEVICE_COLUMNS.last.toLowerCase() }} {{ x.last_seen }}</span>
                    </div>
                    <template v-if="waiting(x)">
                      <div class="h-1.5 w-full overflow-hidden rounded-full bg-hairline" role="progressbar" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="radioWaitingPercent(x.cycles_heard, x.need_heard, d.list.value.listened_cycles, d.list.value.min_listened_cycles)" aria-label="How much evidence there is" data-test="radio-air-progress">
                        <div class="h-full bg-accent-brand" :style="{ width: radioWaitingPercent(x.cycles_heard, x.need_heard, d.list.value.listened_cycles, d.list.value.min_listened_cycles) + '%' }" />
                      </div>
                      <span class="text-muted" data-test="radio-air-waiting">{{ radioWaitingLine(x.cycles_heard, x.need_heard, d.list.value.listened_cycles, d.list.value.min_listened_cycles) }}</span>
                    </template>
                    <span v-else class="text-muted" data-test="radio-air-why">{{ RADIO_REASONS[x.why] ?? x.why }}</span>
                    <dl v-if="advanced" class="m-0 grid grid-cols-2 gap-x-4 gap-y-0.5 text-muted sm:grid-cols-4">
                      <div><dt class="inline">{{ RADIO_DEVICE_COLUMNS.heard }}</dt> <dd class="m-0 inline font-mono text-secondary" data-test="radio-air-heard">{{ radioHeardLine(x.cycles_heard, d.list.value.listened_cycles) }}</dd></div>
                      <div><dt class="inline">{{ RADIO_DEVICE_COLUMNS.retries }}</dt> <dd class="m-0 inline font-mono text-secondary" data-test="radio-air-retries">{{ radioRetryWord(x.retry_percent) }}</dd></div>
                      <div><dt class="inline">{{ RADIO_DEVICE_COLUMNS.channels }}</dt> <dd class="m-0 inline font-mono text-secondary" data-test="radio-air-channels">{{ radioChannelsLine(x.channels) }}</dd></div>
                      <div><dt class="inline">First heard</dt> <dd class="m-0 inline font-mono text-secondary">{{ x.first_seen }}</dd></div>
                    </dl>
                    <span v-if="advanced && x.random_address" class="text-muted">{{ RADIO_RANDOM_ADDRESS }}</span>
                    <span v-if="x.joined_own" class="text-state-success">{{ RADIO_JOINED_OWN }}</span>
                  </li>
                </ul>
              </div>
            </li>
          </ul>
        </template>
        <p class="m-0 text-[length:var(--text-small)]" :class="thin ? 'text-state-running' : 'text-muted'" data-test="radio-around-listened">{{ radioListenedLine(air.listened_cycles, air.rules.min_cycles_listened, air.rules.window_days) }}</p>
        <p v-if="air.rules.provisional" class="m-0 text-[length:var(--text-small)] text-muted" data-test="radio-around-provisional">{{ radioProvisionalLine(advanced, air.rules.version) }}</p>
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
