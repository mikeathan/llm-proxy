<script setup lang="ts">
import { computed, onMounted, reactive } from "vue"
import { useRouter } from "vue-router"
import Panel from "../common/layout/Panel.vue"
import Disclosure from "../common/layout/Disclosure.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import StatusTag from "../common/display/StatusTag.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import { useRadioSensors } from "../../composables/radio/useRadioSensors"
import { toRadio } from "../../router/routes"
import {
  RADIO_CAPABILITIES_TITLE, RADIO_EMPTY_BODY, RADIO_EMPTY_TITLE, RADIO_KIND_WORDS, RADIO_LOAD_FAILED_TITLE, RADIO_LOAD_NEXT, RADIO_NOT_READ_YET,
  RADIO_OFF_ACTION, RADIO_OFF_BODY, RADIO_OFF_TITLE, RADIO_PROBE_PENDING, RADIO_RELOAD, RADIO_ROLE_STATE, RADIO_ROLE_WORDS, RADIO_SENSORS_INTRO,
  RADIO_SENSORS_TITLE, RADIO_USE_STATE, RADIO_USE_WORDS, radioDetailsSummary, radioFoundLine, radioProbeLine, radioUnusedLine,
} from "../../constants/radio"
import type { RadioSensor } from "../../types/radio"

// Radio · Sensors: each radio on this computer with one word for what it is to the stack. What it can do is under Details, so the page reads
// as a short list first. Looking sends nothing and listens to nothing.
const r = useRadioSensors()
const router = useRouter()
const open = reactive<Record<string, boolean>>({})

const sensors = computed(() => r.view.value?.sensors ?? [])
const canListen = computed(() => sensors.value.filter((s) => s.role === "can_listen" || s.role === "listening").length)
const key = (s: RadioSensor): string => `${s.kind}-${s.id}`
function technical(s: RadioSensor): string {
  return [s.bus, s.driver, s.usb_id, s.mode].filter(Boolean).join(" · ")
}
function summary(s: RadioSensor): string {
  if (s.probe) return radioProbeLine(s.probe.bands, s.probe.standards, s.probe.monitor)
  return s.role === "can_listen" || s.role === "listening" ? RADIO_PROBE_PENDING : ""
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
        <EmptyState v-else-if="!sensors.length" :title="RADIO_EMPTY_TITLE" :body="r.view.value.reason || RADIO_EMPTY_BODY" />
        <template v-else>
          <p class="m-0 text-[length:var(--text-small)] text-secondary" data-test="radio-found">{{ radioFoundLine(canListen, sensors.length) }}</p>
          <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
            <li v-for="s in sensors" :key="key(s)" class="flex flex-col gap-1.5 py-3 first:pt-0 last:pb-0" data-test="radio-sensor" :data-sensor="s.id" :data-role="s.role">
              <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
                <span class="font-medium text-primary">{{ s.id }}</span>
                <span class="text-muted">{{ RADIO_KIND_WORDS[s.kind] }}</span>
                <StatusTag :state="RADIO_ROLE_STATE[s.role]" :label="RADIO_ROLE_WORDS[s.role]" data-test="radio-role" />
              </div>
              <p class="m-0 break-words text-[length:var(--text-small)] text-secondary" data-test="radio-why">{{ s.why }}</p>
              <p v-if="summary(s)" class="m-0 break-words text-[length:var(--text-small)] text-muted" data-test="radio-probe">{{ summary(s) }}</p>
              <Disclosure v-if="s.capabilities.length > 1" :open="!!open[key(s)]" @update:open="open[key(s)] = $event" :title="RADIO_CAPABILITIES_TITLE" :summary="radioDetailsSummary(s.capabilities.length)">
                <p v-if="technical(s)" class="m-0 break-words font-mono text-[length:var(--text-small)] text-muted">{{ technical(s) }}</p>
                <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
                  <li v-for="c in s.capabilities" :key="c.id" class="flex flex-col gap-0.5 py-2 first:pt-0" data-test="radio-capability" :data-use="c.use">
                    <span class="flex flex-wrap items-center gap-2">
                      <span class="text-primary">{{ c.title }}</span>
                      <StatusTag :state="RADIO_USE_STATE[c.use]" :label="RADIO_USE_WORDS[c.use]" />
                      <span v-if="!c.known && c.use !== 'unused_by_design'" class="text-[length:var(--text-small)] text-muted">{{ RADIO_NOT_READ_YET }}</span>
                    </span>
                    <span class="break-words text-[length:var(--text-small)] text-muted">{{ c.note }}</span>
                  </li>
                </ul>
              </Disclosure>
              <p v-else-if="technical(s)" class="m-0 break-words font-mono text-[length:var(--text-small)] text-muted">{{ technical(s) }}</p>
            </li>
          </ul>
          <p v-if="r.view.value.unused" class="m-0 text-[length:var(--text-small)] text-muted" data-test="radio-unused">{{ radioUnusedLine(r.view.value.unused) }}</p>
        </template>
        <div><BaseButton variant="secondary" icon="refresh" class-name="max-sm:!h-11" :disabled="r.loading.value" data-test="radio-reload" @click="r.load">{{ RADIO_RELOAD }}</BaseButton></div>
      </div>
    </Panel>
  </div>
</template>
