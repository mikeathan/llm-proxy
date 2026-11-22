<script setup lang="ts">
import { ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Panel from "../common/layout/Panel.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import NewsroomBriefTopicEditor from "./NewsroomBriefTopicEditor.vue"
import NewsroomBriefEditionEditor from "./NewsroomBriefEditionEditor.vue"
import NewsroomBriefLimit from "./NewsroomBriefLimit.vue"
import NewsroomBriefMySources from "./NewsroomBriefMySources.vue"
import { formatRelativeTime } from "../../utils/format/time"
import {
  EDITIONS_TITLE, EDITION_OFF_LABEL, EDITION_ON_BADGE, MODEL_LIMIT_TITLE, MY_SOURCES_TITLE, NEED_TOPIC_FIRST, NEW_EDITION_LABEL, NEW_TOPIC_LABEL, NO_EDITIONS_SETUP_BODY, NO_EDITIONS_TITLE, NO_TOPICS_BODY,
  READ_NOW_LABEL, TOPICS_TITLE, TOPIC_NEVER_READ, editionCardWords, gatherWords, topicFollowsWords, topicLastWords,
} from "../../constants/newsroom"
import { hoursOf } from "../../utils/newsroom/briefForm"
import type { BriefEdition, BriefEditionEdit, BriefSettings, BriefSource, BriefTopic, BriefTopicEdit } from "../../types/newsroom"

// Topics and editions: what is read, and what is written from it. The editing state is here; the page owns the data and does the saving, deleting and
// reading (it asks before anything is deleted), and each action answers whether it worked so the form closes only on success.
const NEW = "new" as const
// A new form sits above the list, so it is set apart from the first row.
const NEW_FORM_SPACING = "mb-4 rounded-[var(--radius-md)] border border-hairline bg-canvas p-4"
const CARD = "flex flex-col gap-2 rounded-[var(--radius-md)] border border-hairline bg-canvas p-4 transition-colors hover:border-accent-brand/50"
const CHIP = "inline-flex max-w-full items-center rounded-full bg-surface-active px-2.5 py-0.5 text-[length:var(--text-small)] text-secondary"
type Target = number | typeof NEW | null
const props = defineProps<{
  topics: BriefTopic[]
  editions: BriefEdition[]
  settings: BriefSettings | null
  sources: BriefSource[]
  saving: boolean
  error: string
  saveTopic: (id: number | null, edit: BriefTopicEdit) => Promise<boolean>
  saveEdition: (id: number | null, edit: BriefEditionEdit) => Promise<boolean>
  saveLimit: (calls: number) => Promise<boolean>
  addSource: (label: string, target: string) => Promise<boolean>
  removeSource: (id: number) => Promise<boolean>
}>()
const emit = defineEmits<{
  (e: "delete-topic", topic: BriefTopic): void
  (e: "delete-edition", edition: BriefEdition): void
  (e: "read-topic", topic: BriefTopic): void
}>()

const editingTopic = ref<Target>(null)
const editingEdition = ref<Target>(null)
const topicName = (id: number): string => props.topics.find((t) => t.id === id)?.name ?? "Removed topic"
const feedsOf = (t: BriefTopic): number => (t.sources ?? []).length
const gather = (e: BriefEdition): string => gatherWords(hoursOf(e.collect_every) ?? 0, hoursOf(e.collect_for) ?? 0)
const idOf = (t: Target): number | null => (typeof t === "number" ? t : null)
</script>

<template>
  <div class="flex flex-col gap-4">
    <Panel :title="TOPICS_TITLE">
      <template #actions>
        <BaseButton size="sm" icon="plus" :disabled="editingTopic === NEW" @click="editingTopic = NEW">{{ NEW_TOPIC_LABEL }}</BaseButton>
      </template>
      <NewsroomBriefTopicEditor
        v-if="editingTopic === NEW"
        :topic="null"
        :sources="sources"
        :add-source="addSource"
        :saving="saving"
        :error="error"
        :class="topics.length ? NEW_FORM_SPACING : ''"
        @save="async (edit) => { if (await saveTopic(null, edit)) editingTopic = null }"
        @cancel="editingTopic = null"
      />
      <p v-else-if="!topics.length" class="m-0 text-muted">{{ NO_TOPICS_BODY }}</p>
      <ul v-if="topics.length" class="m-0 grid list-none gap-3 p-0 md:grid-cols-2" aria-label="Topics">
        <li v-for="t in topics" :key="t.id" :class="editingTopic === t.id ? 'md:col-span-2' : ''">
          <NewsroomBriefTopicEditor
            v-if="editingTopic === t.id"
            :topic="t"
            :sources="sources"
            :add-source="addSource"
            :saving="saving"
            :error="error"
            :class="NEW_FORM_SPACING"
            @save="async (edit) => { if (await saveTopic(t.id, edit)) editingTopic = null }"
            @cancel="editingTopic = null"
          />
          <div v-else :class="CARD" data-test="topic-card">
            <div class="flex items-start gap-2">
              <span class="min-w-0 flex-1 break-words text-[length:var(--text-body)] font-medium text-primary">{{ t.name }}</span>
              <BaseButton variant="ghost" size="sm" icon="edit" icon-only :label="`Edit ${t.name}`" @click="editingTopic = t.id" />
              <BaseButton variant="ghost" size="sm" icon="trash" icon-only :label="`Delete ${t.name}`" @click="emit('delete-topic', t)" />
            </div>
            <div class="flex flex-wrap gap-1.5">
              <span v-for="q in t.queries ?? []" :key="q" :class="CHIP"><span class="truncate">{{ q }}</span></span>
              <span v-if="feedsOf(t)" :class="[CHIP, 'text-accent-brand']">{{ topicFollowsWords(feedsOf(t)) }}</span>
            </div>
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
    </Panel>

    <Panel :title="EDITIONS_TITLE">
      <template #actions>
        <BaseButton size="sm" icon="plus" :disabled="editingEdition === NEW || !topics.length" @click="editingEdition = NEW">{{ NEW_EDITION_LABEL }}</BaseButton>
      </template>
      <NewsroomBriefEditionEditor
        v-if="editingEdition === NEW"
        :edition="null"
        :topics="topics"
        :saving="saving"
        :error="error"
        :class="editions.length ? NEW_FORM_SPACING : ''"
        @save="async (edit) => { if (await saveEdition(null, edit)) editingEdition = null }"
        @cancel="editingEdition = null"
      />
      <p v-else-if="!topics.length" class="m-0 text-muted">{{ NEED_TOPIC_FIRST }}</p>
      <EmptyState v-else-if="!editions.length" :title="NO_EDITIONS_TITLE" :body="NO_EDITIONS_SETUP_BODY" />
      <ul v-if="editions.length" class="m-0 grid list-none gap-3 p-0" aria-label="Editions">
        <li v-for="e in editions" :key="e.id">
          <NewsroomBriefEditionEditor
            v-if="editingEdition === e.id"
            :edition="e"
            :topics="topics"
            :saving="saving"
            :error="error"
            :class="NEW_FORM_SPACING"
            @save="async (edit) => { if (await saveEdition(idOf(editingEdition), edit)) editingEdition = null }"
            @cancel="editingEdition = null"
          />
          <div v-else :class="CARD" data-test="edition-card">
            <div class="flex items-center gap-2">
              <span :class="['h-2 w-2 flex-none rounded-full', e.enabled ? 'bg-state-success' : 'bg-faint']" aria-hidden="true"></span>
              <span class="min-w-0 flex-1 break-words text-[length:var(--text-body)] font-medium text-primary">{{ e.name }}</span>
              <span class="text-[length:var(--text-small)] text-faint">{{ e.enabled ? EDITION_ON_BADGE : EDITION_OFF_LABEL }}</span>
              <BaseButton variant="ghost" size="sm" icon="edit" icon-only :label="`Edit ${e.name}`" @click="editingEdition = e.id" />
              <BaseButton variant="ghost" size="sm" icon="trash" icon-only :label="`Delete ${e.name}`" @click="emit('delete-edition', e)" />
            </div>
            <p class="m-0 text-[length:var(--text-small)] text-secondary">{{ editionCardWords(e.ready_by, e.days) }}</p>
            <div class="flex flex-wrap gap-1.5"><span v-for="s in e.sections" :key="s.topic_id" :class="CHIP">{{ topicName(s.topic_id) }}</span></div>
            <p class="m-0 text-[length:var(--text-small)] text-faint">{{ gather(e) }}</p>
          </div>
        </li>
      </ul>
    </Panel>

    <Panel :title="MY_SOURCES_TITLE">
      <NewsroomBriefMySources :sources="sources" :saving="saving" :error="error" :add="addSource" :remove="removeSource" />
    </Panel>

    <Panel v-if="settings" :title="MODEL_LIMIT_TITLE">
      <NewsroomBriefLimit :settings="settings" :saving="saving" :save="saveLimit" />
    </Panel>
  </div>
</template>
