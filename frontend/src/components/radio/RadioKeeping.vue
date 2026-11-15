<script setup lang="ts">
import { computed, onMounted } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import { useRadioKeeping } from "../../composables/radio/useRadioKeeping"
import {
  RADIO_FORGET, RADIO_FORGET_BODY, RADIO_KEEP_CHOICES, RADIO_KEEP_DROP, RADIO_KEEP_INTRO, RADIO_KEEP_OFF_NOTE, RADIO_KEEP_ON_NOTE,
  RADIO_KEEP_TITLE, RADIO_KEEP_TRACK_LABEL, radioKeepStored,
} from "../../constants/radio"

// Radio · What is kept: the one place that decides what stays on disk about other people's devices. One choice (the track: off, 24 or 48 hours;
// off by default, ADR-0055), what is stored with one way to delete it, and the one way to forget every neighbour. Turning the track off deletes it.
const k = useRadioKeeping()
const hours = computed(() => k.keeping.value?.track_hours ?? 0)
const options = computed(() => RADIO_KEEP_CHOICES.filter((c) => !k.keeping.value || k.keeping.value.choices.includes(Number(c.value))))
onMounted(() => { void k.load() })
</script>

<template>
  <Panel :title="RADIO_KEEP_TITLE">
    <div class="flex flex-col gap-3" data-test="radio-keeping">
      <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_KEEP_INTRO }}</p>
      <p v-if="k.error.value" class="m-0 text-state-error" data-test="radio-keeping-error">{{ k.error.value }}</p>
      <template v-if="k.keeping.value">
        <div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
          <span class="text-primary">{{ RADIO_KEEP_TRACK_LABEL }}</span>
          <SegmentedControl :model-value="String(hours)" :options="options" :label="RADIO_KEEP_TRACK_LABEL" variant="compact" @update:model-value="k.setHours(Number($event))" />
        </div>
        <p class="m-0 text-[length:var(--text-small)] text-muted" data-test="radio-keeping-note">{{ hours ? RADIO_KEEP_ON_NOTE : RADIO_KEEP_OFF_NOTE }}</p>
        <p v-if="hours" class="m-0 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 text-[length:var(--text-small)]" data-test="radio-keeping-stored">
          <span class="text-secondary">{{ radioKeepStored(k.keeping.value.stored_points, k.keeping.value.oldest) }}</span>
          <BaseButton v-if="k.keeping.value.stored_points" variant="secondary" class-name="max-sm:!h-11" :disabled="k.busy.value" data-test="radio-keeping-drop" @click="k.dropTracks">{{ RADIO_KEEP_DROP }}</BaseButton>
        </p>
      </template>
      <p v-if="k.notice.value" class="m-0 text-[length:var(--text-small)] text-state-success" data-test="radio-keeping-done">{{ k.notice.value }}</p>
      <div class="flex flex-col gap-2 border-t border-hairline pt-3">
        <p class="m-0 text-[length:var(--text-small)] text-muted">{{ RADIO_FORGET_BODY }}</p>
        <div><BaseButton variant="danger" class-name="max-sm:!h-11" :disabled="k.busy.value" data-test="radio-forget" @click="k.forget">{{ RADIO_FORGET }}</BaseButton></div>
      </div>
    </div>
  </Panel>
</template>
