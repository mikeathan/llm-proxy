<script setup lang="ts">
import { computed, onMounted } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import RadioMemberRow from "./RadioMemberRow.vue"
import { useRadioAsk } from "../../composables/radio/useRadioAsk"
import { usePolling } from "../../composables/ui/usePolling"
import { RADIO_ASK_INTRO, RADIO_ASK_MINE, RADIO_ASK_NOT_MINE, RADIO_ASK_REASONS, RADIO_ASK_TITLE, radioLearnedLine } from "../../constants/radioIdentity"
import { defaultLabel } from "../../utils/radio/identity"
import type { RadioMember } from "../../types/radio"

// Radio · "Help it learn": the few devices an answer would teach the sorting most about (likely yours first, then close ones it is unsure of),
// each with two buttons. "Mine" names it yours (under its make and model: never its name on your network or its network name, which are kept off
// the disk) and records ownership only; "Not mine" lets the server record a neighbour's or a passer-by, whichever fits. The card says what was learned so far,
// and hides itself when there is nothing to ask.
const emit = defineEmits<{ (e: "answered"): void }>()
const ask = useRadioAsk()
const REFRESH_MS = 60000
const devices = computed(() => ask.view.value?.devices ?? [])
const learned = computed(() => ask.view.value?.learned ?? {})

async function mine(m: RadioMember): Promise<void> {
  if (await ask.mine(m.id, defaultLabel(m))) emit("answered")
}
async function notMine(m: RadioMember): Promise<void> {
  if (await ask.setClass(m.id, "not_mine")) emit("answered")
}

onMounted(() => { void ask.load() })
usePolling(() => { void ask.load() }, REFRESH_MS)
</script>

<template>
  <section v-if="devices.length" class="flex flex-col gap-2 rounded-[var(--radius-md)] border border-accent-brand/50 px-3 py-3" data-test="radio-ask">
    <header class="flex flex-col gap-0.5">
      <h3 class="m-0 text-[length:var(--text-body)] text-primary">{{ RADIO_ASK_TITLE }}</h3>
      <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_ASK_INTRO }} {{ radioLearnedLine(learned.owner_labels ?? 0, learned.agree ?? 0, learned.checked ?? 0) }}</p>
    </header>
    <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
      <li v-for="m in devices" :key="m.id" class="flex flex-col gap-1 py-1 sm:flex-row sm:items-center sm:gap-3" data-test="radio-ask-row">
        <div class="min-w-0 flex-1">
          <RadioMemberRow :member="m" />
          <p class="m-0 pl-10 text-[length:var(--text-small)] text-muted">{{ RADIO_ASK_REASONS[m.reason ?? ''] ?? '' }}</p>
        </div>
        <div class="flex flex-none gap-2 pl-10 sm:pl-0">
          <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" :disabled="ask.busy.value === m.id" data-test="radio-ask-mine" @click="mine(m)">{{ RADIO_ASK_MINE }}</BaseButton>
          <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" :disabled="ask.busy.value === m.id" data-test="radio-ask-not-mine" @click="notMine(m)">{{ RADIO_ASK_NOT_MINE }}</BaseButton>
        </div>
      </li>
    </ul>
    <p v-if="ask.error.value" class="m-0 text-[length:var(--text-small)] text-state-error" data-test="radio-ask-error">{{ ask.error.value }}</p>
  </section>
</template>
