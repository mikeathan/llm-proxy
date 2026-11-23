<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import BaseToggle from "../common/buttons/BaseToggle.vue"
import FormField from "../common/forms/FormField.vue"
import CheckboxGroup from "../common/forms/CheckboxGroup.vue"
import { editionEdit, editionTopicIds, hoursOf } from "../../utils/newsroom/briefForm"
import {
  ALL_DAYS, DAY_ORDER, DAY_SHORT, DEFAULT_EVERY_HOURS, DEFAULT_FOR_HOURS, DEFAULT_READY_BY, EDITION_DAYS_LABEL, EDITION_NAME_HINT, EDITION_NAME_LABEL, EDITION_ON_LABEL,
  EDITION_READY_LABEL, EDITION_TOPICS_LABEL, EDITION_WATCHES_LABEL, EDITION_HOME_LABEL, EVERY_HOURS, FOR_HOURS, GATHER_AFTER, GATHER_BEFORE, GATHER_START, NEED_TOPIC_FIRST, WATCH_NAME_MAX, hoursWords,
} from "../../constants/newsroom"
import type { BriefEdition, BriefEditionEdit, BriefTopic } from "../../types/newsroom"

// Create or edit one edition: its name, when it is ready, the days and the topics it lists, and in one sentence how often it gathers the news and from how long
// before. Nothing is hidden behind "Advanced".
// The sections keep the order the topics are listed in. The server checks everything again and says why it refuses (`error`).
const props = defineProps<{ edition: BriefEdition | null; topics: BriefTopic[]; saving: boolean; error: string }>()
const emit = defineEmits<{ (e: "save", edit: BriefEditionEdit): void; (e: "cancel"): void }>()

const name = ref(props.edition?.name ?? "")
const readyBy = ref(props.edition?.ready_by ?? DEFAULT_READY_BY)
const days = ref<string[]>((props.edition?.days ?? ALL_DAYS).map(String))
const chosen = ref<string[]>(editionTopicIds(props.edition).map(String))
const enabled = ref(props.edition?.enabled ?? true)
const watches = ref(props.edition?.watches ?? false)
const home = ref(props.edition?.home ?? false)
const every = ref(hoursOf(props.edition?.collect_every ?? "") ?? DEFAULT_EVERY_HOURS)
const forHours = ref(hoursOf(props.edition?.collect_for ?? "") ?? DEFAULT_FOR_HOURS)

// A stored value the pickers do not list (the assistant or an older version made it) stays selectable instead of being replaced.
const withCurrent = (list: readonly number[], current: number): number[] => (list.includes(current) ? [...list] : [...list, current].sort((a, b) => a - b))
const everyOptions = computed(() => withCurrent(EVERY_HOURS, every.value))
const forOptions = computed(() => withCurrent(FOR_HOURS, forHours.value))
const topicOptions = computed(() => props.topics.map((t) => ({ value: String(t.id), label: t.name })))
const canSave = computed(() => !props.saving && name.value.trim() !== "" && chosen.value.length > 0 && days.value.length > 0)

const toggleDay = (d: string) => (days.value = days.value.includes(d) ? days.value.filter((x) => x !== d) : [...days.value, d])

function submit() {
  if (!canSave.value) return
  // The sections follow the order the topics are listed in on screen.
  emit("save", editionEdit({ name: name.value, readyBy: readyBy.value, days: days.value.map(Number), topics: chosen.value.map(Number), enabled: enabled.value, every: every.value, forHours: forHours.value, watches: watches.value, home: home.value }))
}
</script>

<template>
  <form class="flex flex-col gap-4" @submit.prevent="submit">
    <div class="grid gap-4 sm:grid-cols-[1fr_auto]">
      <FormField :label="EDITION_NAME_LABEL" :hint="EDITION_NAME_HINT">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model="name" :aria-describedby="describedBy" type="text" :maxlength="WATCH_NAME_MAX" required class="form-control" />
        </template>
      </FormField>
      <FormField :label="EDITION_READY_LABEL">
        <template #default="{ id }"><input :id="id" v-model="readyBy" type="time" required class="form-control w-fit" /></template>
      </FormField>
    </div>
    <fieldset class="m-0 flex min-w-0 flex-col gap-1 border-0 p-0">
      <legend class="mb-1 p-0 text-[length:var(--text-small)] font-medium text-secondary">{{ EDITION_DAYS_LABEL }}</legend>
      <div role="group" :aria-label="EDITION_DAYS_LABEL" class="flex flex-wrap gap-1.5" data-test="day-pills">
        <button
          v-for="d in DAY_ORDER"
          :key="d"
          type="button"
          :aria-pressed="days.includes(d)"
          :class="['inline-flex min-h-11 min-w-12 items-center justify-center rounded-full border px-3 text-[length:var(--text-small)] focus-visible:outline-none focus-visible:ring-2 sm:min-h-8', days.includes(d) ? 'border-accent-brand bg-surface-active text-primary' : 'border-hairline text-secondary hover:bg-surface-hover']"
          @click="toggleDay(d)"
        >{{ DAY_SHORT[d] }}</button>
      </div>
    </fieldset>
    <fieldset class="m-0 flex min-w-0 flex-col gap-1 border-0 p-0">
      <legend class="mb-1 p-0 text-[length:var(--text-small)] font-medium text-secondary">{{ EDITION_TOPICS_LABEL }}</legend>
      <CheckboxGroup v-model="chosen" :options="topicOptions" :label="EDITION_TOPICS_LABEL" :empty="NEED_TOPIC_FIRST" />
    </fieldset>
    <div class="flex flex-wrap items-center gap-x-2 gap-y-2 rounded-[var(--radius-md)] bg-canvas p-3 text-[length:var(--text-small)] text-secondary" data-test="gather-sentence">
      <span>{{ GATHER_BEFORE }}</span>
      <select v-model="every" aria-label="How often to gather" class="form-control w-fit"><option v-for="h in everyOptions" :key="h" :value="h">{{ hoursWords(h) }}</option></select>
      <span>{{ GATHER_START }}</span>
      <select v-model="forHours" aria-label="How long before it is ready" class="form-control w-fit"><option v-for="h in forOptions" :key="h" :value="h">{{ hoursWords(h) }}</option></select>
      <span>{{ GATHER_AFTER }}</span>
    </div>
    <BaseToggle v-model="watches" :label="EDITION_WATCHES_LABEL" />
    <BaseToggle v-model="home" :label="EDITION_HOME_LABEL" />
    <BaseToggle v-model="enabled" :label="EDITION_ON_LABEL" />
    <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
    <div class="flex gap-2">
      <BaseButton type="submit" size="sm" :disabled="!canSave">Save</BaseButton>
      <BaseButton variant="ghost" size="sm" @click="emit('cancel')">Cancel</BaseButton>
    </div>
  </form>
</template>
