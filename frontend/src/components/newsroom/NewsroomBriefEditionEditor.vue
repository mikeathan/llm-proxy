<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import BaseToggle from "../common/buttons/BaseToggle.vue"
import FormField from "../common/forms/FormField.vue"
import { editionEdit, editionSectionKeys, hoursOf, sectionGroups, topicIdsOf } from "../../utils/newsroom/briefForm"
import {
  ALL_DAYS, DAY_ORDER, DAY_SHORT, DEFAULT_EVERY_HOURS, DEFAULT_FOR_HOURS, DEFAULT_READY_BY, EDITION_DAYS_LABEL, EDITION_HOME_LABEL, EDITION_INCLUDE_HINT, EDITION_NAME_HINT, EDITION_NAME_LABEL,
  EDITION_NEEDS_SOMETHING, EDITION_NEWS_HINT, EDITION_NEWS_LABEL, EDITION_ON_LABEL, EDITION_READY_LABEL, EDITION_SECTIONS_LABEL, EDITION_TOO_MANY, EDITION_TOPICS_LABEL, EDITION_WATCHES_HINT,
  EDITION_WATCHES_LABEL, EVERY_HOURS, FOR_HOURS, GATHER_AFTER, GATHER_BEFORE, GATHER_START, HOME_COUNTS_ONLY, MAX_EDITION_TOPICS, NEED_TOPIC_FIRST, WATCH_NAME_MAX, addSectionLabel, hoursWords, removeSectionLabel,
} from "../../constants/newsroom"
import type { BriefEdition, BriefEditionEdit, BriefTopic } from "../../types/newsroom"

// Create or edit one edition (one brief): its name, when it is ready, the days, and what goes in it: the news sections you pick, what your watches found,
// and your home. A brief may be only the last two. The sections are the Topics panel's (topics sharing a section name); the server keeps topics, so the
// picked sections are turned into their topics on save. The order is fixed: news sections, then watches, then home. In one sentence it also says how often it
// gathers the news and from how long before. Nothing is hidden behind "Advanced". The server checks everything again and says why it refuses (`error`).
const props = defineProps<{ edition: BriefEdition | null; topics: BriefTopic[]; saving: boolean; error: string }>()
const emit = defineEmits<{ (e: "save", edit: BriefEditionEdit): void; (e: "cancel"): void }>()

const groups = computed(() => sectionGroups(props.topics))
const name = ref(props.edition?.name ?? "")
const readyBy = ref(props.edition?.ready_by ?? DEFAULT_READY_BY)
const days = ref<string[]>((props.edition?.days ?? ALL_DAYS).map(String))
const picked = ref<string[]>(editionSectionKeys(props.edition, groups.value))
const enabled = ref(props.edition?.enabled ?? true)
const watches = ref(props.edition?.watches ?? false)
const home = ref(props.edition?.home ?? false)
const every = ref(hoursOf(props.edition?.collect_every ?? "") ?? DEFAULT_EVERY_HOURS)
const forHours = ref(hoursOf(props.edition?.collect_for ?? "") ?? DEFAULT_FOR_HOURS)

// A stored value the pickers do not list (the assistant or an older version made it) stays selectable instead of being replaced.
const withCurrent = (list: readonly number[], current: number): number[] => (list.includes(current) ? [...list] : [...list, current].sort((a, b) => a - b))
const everyOptions = computed(() => withCurrent(EVERY_HOURS, every.value))
const forOptions = computed(() => withCurrent(FOR_HOURS, forHours.value))

const news = computed(() => picked.value.length > 0)
const topicIds = computed(() => topicIdsOf(picked.value, groups.value))
const tooMany = computed(() => topicIds.value.length > MAX_EDITION_TOPICS)
const hasSomething = computed(() => news.value || watches.value || home.value)
const canSave = computed(() => !props.saving && name.value.trim() !== "" && days.value.length > 0 && hasSomething.value && !tooMany.value)

const setNews = (on: boolean) => (picked.value = on ? groups.value.map((g) => g.key) : [])
const toggleSection = (key: string) => (picked.value = picked.value.includes(key) ? picked.value.filter((k) => k !== key) : [...picked.value, key])
const toggleDay = (d: string) => (days.value = days.value.includes(d) ? days.value.filter((x) => x !== d) : [...days.value, d])

function submit() {
  if (!canSave.value) return
  emit("save", editionEdit({ name: name.value, readyBy: readyBy.value, days: days.value.map(Number), topics: topicIds.value, enabled: enabled.value, every: every.value, forHours: forHours.value, watches: watches.value, home: home.value }))
}
const BLOCK = "flex flex-col gap-2 rounded-[var(--radius-md)] border border-hairline bg-canvas p-3"
const TAG = "inline-flex min-h-8 items-center gap-1 rounded-full bg-surface-active pl-2.5 pr-1 text-[length:var(--text-small)] text-secondary"
</script>

<template>
  <form class="flex flex-col gap-4" @submit.prevent="submit">
    <div class="grid gap-4 sm:grid-cols-[1fr_auto]">
      <FormField :label="EDITION_NAME_LABEL" :hint="EDITION_NAME_HINT">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model="name" :aria-describedby="describedBy" type="text" :maxlength="WATCH_NAME_MAX" required class="form-control" data-test="edition-name" />
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
          :class="['inline-flex min-h-11 min-w-12 items-center justify-center rounded-[var(--radius-sm)] border px-3 font-mono text-[length:var(--text-small)] focus-visible:outline-none focus-visible:ring-2 sm:min-h-8', days.includes(d) ? 'border-control bg-surface-active text-primary' : 'border-hairline text-muted hover:bg-surface-hover']"
          @click="toggleDay(d)"
        >{{ DAY_SHORT[d] }}</button>
      </div>
    </fieldset>
    <fieldset class="m-0 flex min-w-0 flex-col gap-2 border-0 p-0" data-test="edition-blocks">
      <legend class="mb-1 p-0 text-[length:var(--text-small)] font-medium text-secondary">{{ EDITION_TOPICS_LABEL }}<span class="ml-2 font-normal text-faint">{{ EDITION_INCLUDE_HINT }}</span></legend>
      <div :class="BLOCK" data-test="block-news">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <div class="font-medium text-primary">{{ EDITION_NEWS_LABEL }}</div>
            <div class="text-[length:var(--text-small)] text-muted">{{ groups.length ? EDITION_NEWS_HINT : NEED_TOPIC_FIRST }}</div>
          </div>
          <BaseToggle :model-value="news" :label="EDITION_NEWS_LABEL" hide-label :disabled="!groups.length" @update:model-value="setNews" />
        </div>
        <div v-if="news" class="flex flex-wrap items-center gap-1.5" role="group" :aria-label="EDITION_SECTIONS_LABEL" data-test="edition-sections">
          <template v-for="g in groups" :key="g.key">
            <span v-if="picked.includes(g.key)" :class="TAG">
              {{ g.label }}<span class="text-faint">{{ g.topics.length }}</span>
              <button type="button" class="inline-flex h-6 w-6 items-center justify-center rounded-full text-muted hover:text-primary focus-visible:outline-none focus-visible:ring-2" :aria-label="removeSectionLabel(g.label)" @click="toggleSection(g.key)">×</button>
            </span>
            <button v-else type="button" class="inline-flex min-h-8 items-center rounded-full px-2.5 text-[length:var(--text-small)] text-muted hover:bg-surface-hover hover:text-primary focus-visible:outline-none focus-visible:ring-2" :aria-label="addSectionLabel(g.label)" @click="toggleSection(g.key)">+ {{ g.label }}</button>
          </template>
        </div>
      </div>
      <div :class="BLOCK">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0"><div class="font-medium text-primary">{{ EDITION_WATCHES_LABEL }}</div><div class="text-[length:var(--text-small)] text-muted">{{ EDITION_WATCHES_HINT }}</div></div>
          <BaseToggle v-model="watches" :label="EDITION_WATCHES_LABEL" hide-label />
        </div>
      </div>
      <div :class="BLOCK">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0"><div class="font-medium text-primary">{{ EDITION_HOME_LABEL }}</div><div class="text-[length:var(--text-small)] text-muted">{{ HOME_COUNTS_ONLY }}</div></div>
          <BaseToggle v-model="home" :label="EDITION_HOME_LABEL" hide-label />
        </div>
      </div>
      <p v-if="tooMany" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error" data-test="edition-too-many">{{ EDITION_TOO_MANY }}</p>
      <p v-else-if="!hasSomething" role="status" class="m-0 text-[length:var(--text-small)] text-faint" data-test="edition-needs">{{ EDITION_NEEDS_SOMETHING }}</p>
    </fieldset>
    <div class="flex flex-wrap items-center gap-x-2 gap-y-2 rounded-[var(--radius-md)] bg-canvas p-3 text-[length:var(--text-small)] text-secondary" data-test="gather-sentence">
      <span>{{ GATHER_BEFORE }}</span>
      <select v-model="every" aria-label="How often to gather" class="form-control w-fit"><option v-for="h in everyOptions" :key="h" :value="h">{{ hoursWords(h) }}</option></select>
      <span>{{ GATHER_START }}</span>
      <select v-model="forHours" aria-label="How long before it is ready" class="form-control w-fit"><option v-for="h in forOptions" :key="h" :value="h">{{ hoursWords(h) }}</option></select>
      <span>{{ GATHER_AFTER }}</span>
    </div>
    <BaseToggle v-model="enabled" :label="EDITION_ON_LABEL" />
    <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
    <div class="flex gap-2">
      <BaseButton type="submit" size="sm" :disabled="!canSave">Save</BaseButton>
      <BaseButton variant="ghost" size="sm" @click="emit('cancel')">Cancel</BaseButton>
    </div>
  </form>
</template>
