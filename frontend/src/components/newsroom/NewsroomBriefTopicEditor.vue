<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import ChipInput from "../common/forms/ChipInput.vue"
import NewsroomBriefSourceBrowser from "./NewsroomBriefSourceBrowser.vue"
import NewsroomBriefCustomFeeds from "./NewsroomBriefCustomFeeds.vue"
import { starterFeeds, topicEdit, topicFields, topicNameFrom, usableStarters } from "../../utils/newsroom/briefForm"
import {
  BRIEF_CATEGORIES, BRIEF_STARTERS, TOPIC_FINE_TUNE_HINT, TOPIC_FINE_TUNE_LABEL, TOPIC_SUGGESTIONS_LABEL, TOPIC_SUGGESTION_LIMIT, topicSummaryWords, TOPIC_CATEGORY_HINT, TOPIC_CATEGORY_LABEL, REGION_OPTIONS, SOURCE_SEARCH_NOTE, TOPIC_FEEDS_HINT, TOPIC_FEEDS_LABEL, TOPIC_FEEDS_PLACEHOLDER, TOPIC_FEEDS_REJECTED, isFeedAddress, TOPIC_IGNORE_HINT, TOPIC_IGNORE_LABEL, TOPIC_NAME_HINT, TOPIC_NAME_LABEL, TOPIC_NEEDS_SOMETHING, TOPIC_REGION_LABEL,
  TOPIC_SEARCHES_HINT, TOPIC_SEARCHES_LABEL, TOPIC_SEARCHES_PLACEHOLDER, WATCH_NAME_MAX,
} from "../../constants/newsroom"
import type { BriefSource, BriefStarter, BriefTopic, BriefTopicEdit } from "../../types/newsroom"

// Create or edit one topic. A new topic asks for one thing: what to follow (chips, or a suggestion tapped). Its name comes from the first search and its
// section from a suggestion, unless the owner sets them; the name, section, websites, region and words to skip are all under Fine-tune (open when a
// topic is edited). Websites are chosen from the well-known and the owner's own sources (`sources`) while typing or in the browser under the field, or
// pasted as a feed link. The server checks everything again and says why it refuses (`error`).
const props = withDefaults(defineProps<{ topic: BriefTopic | null; saving: boolean; error: string; sources?: readonly BriefSource[]; categories?: readonly string[]; addSource?: (label: string, target: string) => Promise<boolean> }>(), { sources: () => [], categories: () => [], addSource: undefined })
const emit = defineEmits<{ (e: "save", edit: BriefTopicEdit): void; (e: "cancel"): void }>()

const f = ref(topicFields(props.topic))
// A feed is shown by the name it was picked by: the list's, else the one the topic already had.
const labels = computed<Record<string, string>>(() => ({
  ...Object.fromEntries((props.topic?.sources ?? []).filter((s) => s.kind === "feed" && s.label).map((s) => [s.target, s.label as string])),
  ...Object.fromEntries(props.sources.filter((s) => s.kind === "feed").map((s) => [s.target, s.label])),
}))
// The feed links in neither list: the ones pasted, which the form checks and offers to keep.
const knownFeeds = computed(() => new Set(props.sources.filter((s) => s.kind === "feed").map((s) => s.target.toLowerCase())))
const customFeeds = computed(() => f.value.feeds.filter((x) => !knownFeeds.value.has(x.toLowerCase())))
const replaceFeed = (from: string, to: string) => (f.value.feeds = [...new Set(f.value.feeds.map((x) => (x === from ? to : x)))])
const feedChoices = computed(() => props.sources.filter((s) => s.kind === "feed").map((s) => ({ value: s.target, label: s.label, note: s.category })))
const searchChoices = computed(() => props.sources.filter((s) => s.kind === "newssearch").map((s) => ({ value: s.target, label: s.label, note: `${s.target} · ${SOURCE_SEARCH_NOTE}` })))
const order = computed(() => [...new Set(props.sources.map((s) => s.category))])
const list = (s: BriefSource): string[] => (s.kind === "feed" ? f.value.feeds : f.value.searches)
const chosen = (s: BriefSource): boolean => list(s).some((x) => x.toLowerCase() === s.target.toLowerCase())
function toggle(s: BriefSource) {
  const next = chosen(s) ? list(s).filter((x) => x.toLowerCase() !== s.target.toLowerCase()) : [...list(s), s.target]
  if (s.kind === "feed") f.value.feeds = next
  else f.value.searches = next
}
// The sections offered: the fixed list, then the others the owner's topics already use.
const sectionChoices = computed(() => [...new Set([...BRIEF_CATEGORIES, ...props.categories])])
// The name typed, else the first search's (or first site's) words; `topicEdit` trims what it is given.
const derivedName = computed(() => topicNameFrom(f.value.searches, f.value.feeds, labels.value))
const edit = computed(() => topicEdit({ ...f.value, name: f.value.name.trim() || derivedName.value }, props.topic, labels.value))
const hasSomething = computed(() => edit.value.queries.length + edit.value.sources.length > 0)
const canSave = computed(() => !props.saving && edit.value.name !== "" && hasSomething.value)
// Suggestions: the starters, as chips. Tapping one adds its searches, sites and section (and a name when none is set); tapping it again takes its searches
// and sites back out.
const suggestions = computed(() => usableStarters(BRIEF_STARTERS, props.sources).slice(0, TOPIC_SUGGESTION_LIMIT))
const starterTargets = (s: BriefStarter): string[] => starterFeeds(s, props.sources).map((x) => x.target)
const lowered = (v: string): string => v.toLowerCase()
const isOn = (s: BriefStarter): boolean => s.queries.every((q) => f.value.searches.some((x) => lowered(x) === lowered(q))) && starterTargets(s).every((t) => f.value.feeds.includes(t))
function tapStarter(s: BriefStarter) {
  if (isOn(s)) {
    f.value.searches = f.value.searches.filter((x) => !s.queries.some((q) => lowered(q) === lowered(x)))
    f.value.feeds = f.value.feeds.filter((x) => !starterTargets(s).includes(x))
    return
  }
  f.value.searches = [...f.value.searches, ...s.queries.filter((q) => !f.value.searches.some((x) => lowered(x) === lowered(q)))]
  f.value.feeds = [...new Set([...f.value.feeds, ...starterTargets(s)])]
  if (f.value.category.trim() === "") f.value.category = s.section
  if (f.value.name.trim() === "") f.value.name = s.name
}
// A stored region the list does not offer (the assistant made it) stays selectable instead of being replaced.
const regions = computed(() => (REGION_OPTIONS.some((o) => o.value === f.value.region) ? REGION_OPTIONS : [...REGION_OPTIONS, { value: f.value.region, label: f.value.region }]))
function submit() {
  if (canSave.value) emit("save", edit.value)
}
</script>

<template>
  <form class="flex flex-col gap-4" @submit.prevent="submit">
    <FormField :label="TOPIC_SEARCHES_LABEL" :hint="TOPIC_SEARCHES_HINT">
      <template #default="{ id, describedBy }">
        <ChipInput
          :id="id"
          v-model="f.searches"
          :label="TOPIC_SEARCHES_LABEL"
          :described-by="describedBy"
          :placeholder="TOPIC_SEARCHES_PLACEHOLDER"
          :suggestions="searchChoices.length ? searchChoices : undefined"
        />
      </template>
    </FormField>
    <ul v-if="!topic && suggestions.length" class="m-0 flex list-none flex-wrap gap-1.5 p-0" :aria-label="TOPIC_SUGGESTIONS_LABEL" data-test="topic-suggestions">
      <li v-for="s in suggestions" :key="s.name">
        <button
          type="button"
          :aria-pressed="isOn(s)"
          :class="[
            'inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-[length:var(--text-small)] transition-colors focus-visible:outline-none focus-visible:ring-2',
            isOn(s) ? 'border-accent-brand bg-surface-active text-primary' : 'border-control bg-canvas text-secondary hover:border-accent-brand/60 hover:text-primary',
          ]"
          @click="tapStarter(s)"
        >
          {{ s.name }}<span aria-hidden="true" class="text-faint">{{ isOn(s) ? "✓" : "+" }}</span>
        </button>
      </li>
    </ul>
    <p v-if="hasSomething && edit.name" role="status" class="m-0 text-[length:var(--text-small)] text-faint" data-test="topic-summary">
      {{ topicSummaryWords(edit.name, edit.category, edit.queries.length, edit.sources.filter((s) => s.kind === "feed").length) }}
    </p>
    <p v-else-if="f.name.trim()" role="status" class="m-0 text-[length:var(--text-small)] text-faint" data-test="topic-needs">{{ TOPIC_NEEDS_SOMETHING }}</p>
    <details :open="!!topic" class="rounded-[var(--radius-md)] border border-hairline" data-test="topic-fine-tune">
      <summary class="cursor-pointer select-none px-3 py-2 text-[length:var(--text-small)] font-medium text-secondary">{{ TOPIC_FINE_TUNE_LABEL }}<span class="ml-2 font-normal text-faint">{{ TOPIC_FINE_TUNE_HINT }}</span></summary>
      <div class="flex flex-col gap-4 border-t border-hairline p-3">
        <FormField :label="TOPIC_NAME_LABEL" :hint="TOPIC_NAME_HINT">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model="f.name" :aria-describedby="describedBy" type="text" :maxlength="WATCH_NAME_MAX" :placeholder="derivedName" class="form-control" data-test="topic-name" />
          </template>
        </FormField>
        <FormField :label="TOPIC_CATEGORY_LABEL" :hint="TOPIC_CATEGORY_HINT">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model="f.category" :aria-describedby="describedBy" :list="`${id}-sections`" type="text" :maxlength="WATCH_NAME_MAX" class="form-control" data-test="topic-section" />
            <datalist :id="`${id}-sections`"><option v-for="c in sectionChoices" :key="c" :value="c" /></datalist>
          </template>
        </FormField>
        <FormField :label="TOPIC_FEEDS_LABEL" :hint="TOPIC_FEEDS_HINT">
          <template #default="{ id, describedBy }">
            <ChipInput
              :id="id"
              v-model="f.feeds"
              :label="TOPIC_FEEDS_LABEL"
              :described-by="describedBy"
              :placeholder="TOPIC_FEEDS_PLACEHOLDER"
              :suggestions="feedChoices"
              :labels="labels"
              :accept="isFeedAddress"
              :rejected="TOPIC_FEEDS_REJECTED"
            />
          </template>
        </FormField>
        <NewsroomBriefCustomFeeds v-if="customFeeds.length" :feeds="customFeeds" :saving="saving" :save="addSource" @replace="replaceFeed" />
        <NewsroomBriefSourceBrowser :sources="sources" :order="order" :topic-name="edit.name" :chosen="chosen" @toggle="toggle" />
        <div class="grid gap-4 sm:grid-cols-2">
          <FormField :label="TOPIC_REGION_LABEL">
            <template #default="{ id }">
              <select :id="id" v-model="f.region" class="form-control"><option v-for="o in regions" :key="o.value" :value="o.value">{{ o.label }}</option></select>
            </template>
          </FormField>
          <FormField :label="TOPIC_IGNORE_LABEL" :hint="TOPIC_IGNORE_HINT">
            <template #default="{ id, describedBy }">
              <ChipInput :id="id" v-model="f.ignore" lower :label="TOPIC_IGNORE_LABEL" :described-by="describedBy" />
            </template>
          </FormField>
        </div>
      </div>
    </details>
    <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
    <div class="flex gap-2">
      <BaseButton type="submit" size="sm" :disabled="!canSave">Save</BaseButton>
      <BaseButton variant="ghost" size="sm" @click="emit('cancel')">Cancel</BaseButton>
    </div>
  </form>
</template>
