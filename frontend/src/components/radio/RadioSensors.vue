<script setup lang="ts">
import { onMounted } from "vue"
import { useRouter } from "vue-router"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import StatusTag from "../common/display/StatusTag.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import { useRadioSensors } from "../../composables/radio/useRadioSensors"
import { toRadio } from "../../router/routes"
import {
  RADIO_CAPABILITIES_TITLE, RADIO_ELIGIBLE_WORD, RADIO_EMPTY_BODY, RADIO_EMPTY_TITLE, RADIO_KIND_WORDS, RADIO_LOAD_FAILED_TITLE, RADIO_LOAD_NEXT,
  RADIO_NOT_ELIGIBLE_WORD, RADIO_NOT_READ_YET, RADIO_OFF_ACTION, RADIO_OFF_BODY, RADIO_OFF_TITLE, RADIO_RELOAD, RADIO_SENSORS_INTRO,
  RADIO_SENSORS_TITLE, RADIO_USE_WORDS, radioUnusedLine,
} from "../../constants/radio"
import type { CapabilityUse } from "../../types/radio"
import type { StatusState } from "../../types/ui"

// Radio · Sensors: each radio on this computer, whether it may listen, and what it can do with a plain word for what the
// stack does about each capability. The "not used" ones are the list of what could be built next.
const r = useRadioSensors()
const router = useRouter()

const USE_STATE: Record<CapabilityUse, StatusState> = {
  used: "success",
  unused_by_design: "neutral",
  unused_not_built: "queued",
  found_no_driver: "queued",
}

function detail(parts: (string | undefined)[]): string {
  return parts.filter(Boolean).join(" · ")
}

onMounted(() => { void r.load() })
</script>

<template>
  <div class="flex flex-col gap-4" data-test="radio-sensors">
    <Panel v-if="r.view.value && !r.view.value.enabled" :title="RADIO_OFF_TITLE">
      <div class="flex flex-col gap-3" data-test="radio-off">
        <p class="m-0 text-secondary">{{ RADIO_OFF_BODY }}</p>
        <div><BaseButton variant="secondary" class-name="max-sm:!h-11" data-test="radio-open-modules" @click="router.push(toRadio('modules'))">{{ RADIO_OFF_ACTION }}</BaseButton></div>
      </div>
    </Panel>

    <Panel :title="RADIO_SENSORS_TITLE">
      <div class="flex flex-col gap-3">
        <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_SENSORS_INTRO }}</p>
        <ErrorState v-if="r.error.value && !r.view.value" :title="RADIO_LOAD_FAILED_TITLE" :cause="r.error.value" :next="RADIO_LOAD_NEXT" />
        <LoadingState v-else-if="!r.view.value" label="Reading the radios" />
        <EmptyState v-else-if="!r.view.value.sensors.length" :title="RADIO_EMPTY_TITLE" :body="r.view.value.reason || RADIO_EMPTY_BODY" />
        <template v-else>
          <p class="m-0 text-[length:var(--text-small)] text-secondary" data-test="radio-unused">{{ radioUnusedLine(r.view.value.unused) }}</p>
          <ul class="m-0 flex list-none flex-col gap-4 p-0">
            <li v-for="s in r.view.value.sensors" :key="`${s.kind}-${s.id}`" class="flex flex-col gap-2" data-test="radio-sensor" :data-sensor="s.id">
              <div class="flex flex-wrap items-center gap-2">
                <span class="font-medium text-primary">{{ s.id }}</span>
                <span class="text-muted">{{ RADIO_KIND_WORDS[s.kind] }}</span>
                <StatusTag :state="s.eligible ? 'success' : 'neutral'" :label="s.eligible ? RADIO_ELIGIBLE_WORD : RADIO_NOT_ELIGIBLE_WORD" />
              </div>
              <p class="m-0 break-words text-[length:var(--text-small)] text-muted">{{ s.why }}</p>
              <p v-if="detail([s.bus, s.driver, s.usb_id, s.mode])" class="m-0 break-words font-mono text-[length:var(--text-small)] text-muted">{{ detail([s.bus, s.driver, s.usb_id, s.mode]) }}</p>
              <h3 class="m-0 mt-1 text-[length:var(--text-small)] font-medium text-secondary">{{ RADIO_CAPABILITIES_TITLE }}</h3>
              <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
                <li v-for="c in s.capabilities" :key="c.id" class="flex flex-col gap-0.5 py-2 first:pt-0" data-test="radio-capability" :data-use="c.use">
                  <span class="flex flex-wrap items-center gap-2">
                    <span class="text-primary">{{ c.title }}</span>
                    <StatusTag :state="USE_STATE[c.use]" :label="RADIO_USE_WORDS[c.use]" />
                    <span v-if="!c.known" class="text-[length:var(--text-small)] text-muted">{{ RADIO_NOT_READ_YET }}</span>
                  </span>
                  <span class="break-words text-[length:var(--text-small)] text-muted">{{ c.note }}</span>
                </li>
              </ul>
            </li>
          </ul>
        </template>
        <div><BaseButton variant="secondary" icon="refresh" class-name="max-sm:!h-11" :disabled="r.loading.value" data-test="radio-reload" @click="r.load">{{ RADIO_RELOAD }}</BaseButton></div>
      </div>
    </Panel>
  </div>
</template>
