<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import {
  RADIO_FIND_BUTTON, RADIO_FIND_CANDIDATE, RADIO_FIND_CLOSE, RADIO_FIND_NONE, RADIO_FIND_OPEN_HINT, RADIO_FIND_STEPS, RADIO_FIND_TITLE, RADIO_MARK_LABEL,
  RADIO_MARK_NO_NAME, RADIO_MARK_PLACEHOLDER, RADIO_MARK_SAVE, RADIO_MOVING_NOW, RADIO_TREND_MARK, RADIO_TREND_WORDS, radioDbmLine, radioFindLead, radioRangeLine,
  radioRouterShort,
} from "../../constants/radio"
import { findLead, strongestNow } from "../../utils/radio/around"
import { deviceFacts, deviceTitle, heardAgoWord } from "../../utils/radio/describe"
import { conclusionOf } from "../../utils/radio/identity"
import { toRadioDevice } from "../../router/routes"
import type { RadioLiveDevice } from "../../types/radio"

// Radio · Find my device: the owner holds a device next to the computer and makes it send; the strongest devices heard now are the likely ones, and
// the owner names the right one. Each name opens the device's own page; each row says whose it probably is, the router box it is joined to and
// whether its signal is rising (the device coming closer), and one line says whether the strongest is clearly ahead of the next. Nothing is sent,
// and it uses only the signal already measured.
const props = defineProps<{ devices: RadioLiveDevice[]; busy: boolean; now: number }>()
const emit = defineEmits<{ (e: "mark", id: string, label: string): void }>()
const open = ref(false)
const naming = ref<string | null>(null)
const label = ref("")
const missing = ref(false)
const candidates = computed(() => strongestNow(props.devices))
const lead = computed(() => findLead(candidates.value))
function reading(d: RadioLiveDevice): string {
  const out = [...deviceFacts(d), radioDbmLine(d.dbm), radioRangeLine(d.near_m, d.far_m)]
  if (d.trend !== "unknown") out.push(`${RADIO_TREND_MARK[d.trend]} ${RADIO_TREND_WORDS[d.trend]}`)
  if (d.moving) out.push(RADIO_MOVING_NOW)
  if (d.router) out.push(radioRouterShort(d.router))
  out.push(heardAgoWord(d, props.now))
  return out.join(" · ")
}

function save(id: string): void {
  const name = label.value.trim()
  missing.value = !name
  if (!name) return
  emit("mark", id, name)
  naming.value = null
  label.value = ""
}
</script>

<template>
  <div class="flex flex-col gap-2" data-test="radio-find">
    <BaseButton v-if="!open" variant="secondary" class-name="self-start max-sm:!h-11" data-test="radio-find-open" @click="open = true">{{ RADIO_FIND_BUTTON }}</BaseButton>
    <div v-else class="flex flex-col gap-2 rounded-[var(--radius-md)] bg-surface-raised px-3 py-3">
      <div class="flex items-baseline justify-between gap-3"><h3 class="m-0 text-[length:var(--text-small)] font-medium text-primary">{{ RADIO_FIND_TITLE }}</h3>
        <button type="button" class="min-h-11 px-2 text-accent-brand hover:underline focus-visible:outline-none focus-visible:ring-2" @click="open = false">{{ RADIO_FIND_CLOSE }}</button></div>
      <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_FIND_STEPS }}</p>
      <p v-if="!candidates.length" class="m-0 text-secondary" data-test="radio-find-none">{{ RADIO_FIND_NONE }}</p>
      <template v-else>
        <p v-if="lead" class="m-0 text-[length:var(--text-small)]" :class="lead.clear ? 'text-primary' : 'text-state-running'" data-test="radio-find-lead">{{ radioFindLead(lead.db, lead.clear) }}</p>
        <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_FIND_OPEN_HINT }}</p>
      </template>
      <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
        <li v-for="(d, i) in candidates" :key="d.id" class="flex flex-col gap-2 py-2" data-test="radio-find-candidate">
          <div class="flex flex-wrap items-baseline justify-between gap-2">
            <span class="flex min-w-0 flex-[1_1_12rem] flex-col gap-0.5">
              <span class="flex flex-wrap items-baseline gap-x-2">
                <RouterLink :to="toRadioDevice(d.id)" class="inline-flex min-h-11 items-center break-words text-primary underline decoration-hairline underline-offset-2 hover:text-accent-brand focus-visible:outline-none focus-visible:ring-2 sm:min-h-0" data-test="radio-find-open-device">{{ deviceTitle(d) }}</RouterLink>
                <span class="font-mono text-[length:var(--text-small)] text-faint">{{ d.id }}</span>
                <span v-if="i === 0" class="text-[length:var(--text-small)] text-accent-brand">{{ RADIO_FIND_CANDIDATE }}</span>
              </span>
              <span class="text-[length:var(--text-small)] text-secondary" data-test="radio-find-whose">{{ conclusionOf(d) }}</span>
              <span class="text-[length:var(--text-small)] text-muted" data-test="radio-find-reading">{{ reading(d) }}</span>
            </span>
            <BaseButton v-if="naming !== d.id" variant="secondary" class-name="max-sm:!h-11" @click="naming = d.id; label = ''; missing = false">{{ RADIO_MARK_SAVE }}</BaseButton>
          </div>
          <form v-if="naming === d.id" class="flex flex-wrap items-end gap-2" @submit.prevent="save(d.id)">
            <label class="flex min-w-0 flex-col gap-1 text-[length:var(--text-small)] text-secondary">{{ RADIO_MARK_LABEL }}
              <input v-model="label" type="text" maxlength="60" :placeholder="RADIO_MARK_PLACEHOLDER" class="h-8 min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2.5 text-primary placeholder:text-faint focus-visible:outline-none focus-visible:ring-2 max-sm:h-11" data-test="radio-find-input" />
            </label>
            <BaseButton type="submit" variant="primary" class-name="max-sm:!h-11" :disabled="busy" data-test="radio-find-save">{{ RADIO_MARK_SAVE }}</BaseButton>
            <span v-if="missing" class="text-[length:var(--text-small)] text-state-error">{{ RADIO_MARK_NO_NAME }}</span>
          </form>
        </li>
      </ul>
    </div>
  </div>
</template>
