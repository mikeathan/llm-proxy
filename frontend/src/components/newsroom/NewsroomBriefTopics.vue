<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Panel from "../common/layout/Panel.vue"
import NewsroomBriefTopicEditor from "./NewsroomBriefTopicEditor.vue"
import NewsroomBriefStarterGallery from "./NewsroomBriefStarterGallery.vue"
import { formatRelativeTime } from "../../utils/format/time"
import { isThinTopic, quickTopic, sectionGroups, topicReads } from "../../utils/newsroom/briefForm"
import {
  ADD_SECTION_LABEL, ADD_TOPIC_LABEL, NEW_SECTION_LABEL, NO_SECTION_HINT, NO_SECTION_LABEL, READ_NOW_LABEL, SECTION_NAME_LABEL, SECTION_NAME_PLACEHOLDER, THIN_TOPIC_HINT, TOPICS_INTRO, TOPICS_TITLE,
  TOPIC_NEVER_READ, TOPIC_READS_ROW, TOPIC_SEARCHES_ROW, WATCH_NAME_MAX, addTopicLabel, addTopicPlaceholder, topicCountWords, topicLastWords,
} from "../../constants/newsroom"
import type { BriefSource, BriefTopic, BriefTopicEdit } from "../../types/newsroom"

// The Topics panel, laid out like the brief it makes: each section is a heading with its topic cards under it, and a field at the end of the section adds a
// topic to it (the word typed is the topic's name and its search; ‘Edit’ on the card opens the rest). A new section is only a name until it gets its first
// topic, because the server keeps a section as the name its topics share. A topic with no section is read under its own name and is listed last.
// The page does the saving, deleting and reading; each save answers whether it worked so a form closes only on success.
const props = defineProps<{
  topics: BriefTopic[]
  sources: BriefSource[]
  saving: boolean
  error: string
  saveTopic: (id: number | null, edit: BriefTopicEdit) => Promise<boolean>
  addSource: (label: string, target: string) => Promise<boolean>
}>()
const emit = defineEmits<{ (e: "delete-topic", topic: BriefTopic): void; (e: "read-topic", topic: BriefTopic): void }>()

const CARD = "flex flex-col gap-2 rounded-[var(--radius-md)] border border-hairline bg-canvas p-3 transition-colors hover:border-accent-brand/50"
const HEADING = "m-0 font-mono text-[length:var(--text-small)] font-medium uppercase tracking-[0.04em] text-primary"

const editing = ref<number | null>(null)
const addingSection = ref(false)
const sectionName = ref("")
// Sections named here that have no topic yet.
const empty = ref<string[]>([])
const drafts = ref<Record<string, string>>({})

const groups = computed(() => sectionGroups(props.topics))
const named = computed(() => groups.value.filter((g) => !g.own))
const own = computed(() => groups.value.filter((g) => g.own).flatMap((g) => g.topics))
const lower = (v: string): string => v.trim().toLowerCase()
// The sections to draw: those with topics, then the new ones still empty, then the topics that have none (read under their own name, no field to add to).
const sections = computed(() => [
  ...named.value.map((g) => ({ label: g.label, topics: g.topics, loose: false })),
  ...empty.value.filter((n) => !named.value.some((g) => lower(g.label) === lower(n))).map((n) => ({ label: n, topics: [] as BriefTopic[], loose: false })),
  ...(own.value.length ? [{ label: NO_SECTION_LABEL, topics: own.value, loose: true }] : []),
])
const categories = computed(() => named.value.map((g) => g.label))

function addSection() {
  const n = sectionName.value.trim().replace(/\s+/g, " ")
  if (n === "") return
  if (!sections.value.some((s) => !s.loose && lower(s.label) === lower(n))) empty.value = [...empty.value, n]
  sectionName.value = ""
  addingSection.value = false
}
async function addTopic(section: string) {
  const word = (drafts.value[section] ?? "").trim()
  if (word === "" || props.saving) return
  if (await props.saveTopic(null, quickTopic(word, section))) {
    drafts.value = { ...drafts.value, [section]: "" }
    empty.value = empty.value.filter((n) => lower(n) !== lower(section))
  }
}
const searchesOf = (t: BriefTopic): string => (t.queries ?? []).join(", ")
const readsOf = (t: BriefTopic): string => topicReads(t).join(", ")
</script>

<template>
  <Panel :title="TOPICS_TITLE">
    <template #actions>
      <BaseButton size="sm" icon="plus" :disabled="addingSection" @click="addingSection = true">{{ NEW_SECTION_LABEL }}</BaseButton>
    </template>
    <p class="m-0 text-[length:var(--text-small)] text-muted">{{ TOPICS_INTRO }}</p>
    <form v-if="addingSection" class="mt-3 flex flex-wrap gap-2" data-test="new-section-form" @submit.prevent="addSection">
      <input v-model="sectionName" type="text" :maxlength="WATCH_NAME_MAX" :aria-label="SECTION_NAME_LABEL" :placeholder="SECTION_NAME_PLACEHOLDER" class="form-control min-w-0 flex-1" data-test="new-section-name" />
      <BaseButton type="submit" size="sm" :disabled="sectionName.trim() === ''">{{ ADD_SECTION_LABEL }}</BaseButton>
      <BaseButton variant="ghost" size="sm" @click="addingSection = false">Cancel</BaseButton>
    </form>
    <NewsroomBriefStarterGallery v-if="!topics.length && !empty.length && !addingSection" class="mt-4" :sources="sources" :saving="saving" @add="async (edits) => { for (const e of edits) if (!(await saveTopic(null, e))) return }" @own="addingSection = true" />
    <div v-for="s in sections" :key="(s.loose ? 'loose:' : 'section:') + s.label" class="mt-4 flex flex-col gap-2" data-test="topic-section-group">
      <div class="flex items-baseline gap-2">
        <h3 :class="HEADING">{{ s.label }}</h3>
        <span class="text-[length:var(--text-small)] text-faint">{{ s.loose ? NO_SECTION_HINT : topicCountWords(s.topics.length) }}</span>
      </div>
      <ul v-if="s.topics.length" class="m-0 grid list-none gap-2 p-0 md:grid-cols-2" :aria-label="s.loose ? 'Topics with no section' : `Topics in ${s.label}`">
        <li v-for="t in s.topics" :key="t.id" :class="editing === t.id ? 'md:col-span-2' : ''">
          <div v-if="editing === t.id" class="rounded-[var(--radius-md)] border border-hairline bg-canvas p-4">
            <NewsroomBriefTopicEditor
              :topic="t"
              :sources="sources"
              :categories="categories"
              :add-source="addSource"
              :saving="saving"
              :error="error"
              @save="async (edit) => { if (await saveTopic(t.id, edit)) editing = null }"
              @cancel="editing = null"
            />
          </div>
          <div v-else :class="CARD" data-test="topic-card">
            <div class="flex items-start gap-2">
              <span class="min-w-0 flex-1 break-words text-[length:var(--text-body)] font-medium text-primary">{{ t.name }}</span>
              <BaseButton variant="ghost" size="sm" icon="edit" icon-only :label="`Edit ${t.name}`" @click="editing = t.id" />
              <BaseButton variant="ghost" size="sm" icon="trash" icon-only :label="`Delete ${t.name}`" @click="emit('delete-topic', t)" />
            </div>
            <dl class="m-0 flex flex-col gap-0.5 text-[length:var(--text-small)]">
              <div v-if="searchesOf(t)" class="flex gap-2"><dt class="w-[58px] flex-none text-faint">{{ TOPIC_SEARCHES_ROW }}</dt><dd class="m-0 min-w-0 break-words text-secondary">{{ searchesOf(t) }}</dd></div>
              <div v-if="readsOf(t)" class="flex gap-2"><dt class="w-[58px] flex-none text-faint">{{ TOPIC_READS_ROW }}</dt><dd class="m-0 min-w-0 break-words text-secondary">{{ readsOf(t) }}</dd></div>
            </dl>
            <p v-if="isThinTopic(t)" class="m-0 text-[length:var(--text-small)] text-muted" data-test="thin-topic">{{ THIN_TOPIC_HINT }}</p>
            <ul v-if="t.last_problems?.length" class="m-0 list-none p-0 text-[length:var(--text-small)] text-state-error" role="status">
              <li v-for="p in t.last_problems" :key="p" class="break-words">{{ p }}</li>
            </ul>
            <div class="mt-1 flex flex-wrap items-center gap-2">
              <span class="min-w-0 flex-1 text-[length:var(--text-small)] text-faint">{{ t.last_collected ? topicLastWords(formatRelativeTime(t.last_collected), t.last_new) : TOPIC_NEVER_READ }}</span>
              <BaseButton variant="secondary" size="sm" :disabled="saving" @click="emit('read-topic', t)">{{ READ_NOW_LABEL }}</BaseButton>
            </div>
          </div>
        </li>
      </ul>
      <form v-if="!s.loose" class="flex gap-2" @submit.prevent="addTopic(s.label)">
        <input v-model="drafts[s.label]" type="text" :aria-label="addTopicLabel(s.label)" :placeholder="addTopicPlaceholder(s.label)" class="form-control min-w-0 flex-1" data-test="add-topic-input" />
        <BaseButton type="submit" variant="secondary" size="sm" :disabled="saving || !(drafts[s.label] ?? '').trim()">{{ ADD_TOPIC_LABEL }}</BaseButton>
      </form>
    </div>
  </Panel>
</template>
