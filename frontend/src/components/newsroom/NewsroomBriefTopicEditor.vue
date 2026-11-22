<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import ChipInput from "../common/forms/ChipInput.vue"
import NewsroomBriefSourceBrowser from "./NewsroomBriefSourceBrowser.vue"
import NewsroomBriefCustomFeeds from "./NewsroomBriefCustomFeeds.vue"
import { topicEdit, topicFields } from "../../utils/newsroom/briefForm"
import {
  REGION_OPTIONS, SOURCE_SEARCH_NOTE, TOPIC_FEEDS_HINT, TOPIC_FEEDS_LABEL, TOPIC_FEEDS_PLACEHOLDER, TOPIC_FEEDS_REJECTED, isFeedAddress, TOPIC_IGNORE_HINT, TOPIC_IGNORE_LABEL, TOPIC_NAME_HINT, TOPIC_NAME_LABEL, TOPIC_NEEDS_SOMETHING, TOPIC_REGION_LABEL,
  TOPIC_SEARCHES_HINT, TOPIC_SEARCHES_LABEL, TOPIC_SEARCHES_PLACEHOLDER, WATCH_NAME_MAX,
} from "../../constants/newsroom"
import type { BriefSource, BriefTopic, BriefTopicEdit } from "../../types/newsroom"

// Create or edit one topic: a name, what it should find (chips), websites to follow, and the region and words to skip. There is nothing hidden behind
// "Advanced". Websites are chosen from the well-known and the owner's own sources (`sources`) while typing or in the browser under the field, or pasted as
// a feed link. The server checks everything again and says why it refuses (`error`).
const props = withDefaults(defineProps<{ topic: BriefTopic | null; saving: boolean; error: string; sources?: readonly BriefSource[]; addSource?: (label: string, target: string) => Promise<boolean> }>(), { sources: () => [], addSource: undefined })
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
const edit = computed(() => topicEdit(f.value, props.topic, labels.value))
const hasSomething = computed(() => edit.value.queries.length + edit.value.sources.length > 0)
const canSave = computed(() => !props.saving && edit.value.name !== "" && hasSomething.value)
// A stored region the list does not offer (the assistant made it) stays selectable instead of being replaced.
const regions = computed(() => (REGION_OPTIONS.some((o) => o.value === f.value.region) ? REGION_OPTIONS : [...REGION_OPTIONS, { value: f.value.region, label: f.value.region }]))
function submit() {
  if (canSave.value) emit("save", edit.value)
}
</script>

<template>
  <form class="flex flex-col gap-4" @submit.prevent="submit">
    <FormField :label="TOPIC_NAME_LABEL" :hint="TOPIC_NAME_HINT">
      <template #default="{ id, describedBy }">
        <input :id="id" v-model="f.name" :aria-describedby="describedBy" type="text" :maxlength="WATCH_NAME_MAX" required class="form-control" />
      </template>
    </FormField>
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
    <NewsroomBriefSourceBrowser :sources="sources" :order="order" :topic-name="f.name" :chosen="chosen" @toggle="toggle" />
    <p v-if="!hasSomething && f.name.trim()" role="status" class="m-0 text-[length:var(--text-small)] text-faint" data-test="topic-needs">{{ TOPIC_NEEDS_SOMETHING }}</p>
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
    <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
    <div class="flex gap-2">
      <BaseButton type="submit" size="sm" :disabled="!canSave">Save</BaseButton>
      <BaseButton variant="ghost" size="sm" @click="emit('cancel')">Cancel</BaseButton>
    </div>
  </form>
</template>
