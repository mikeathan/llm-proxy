<script setup lang="ts">
import { ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Panel from "../common/layout/Panel.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import NewsroomBriefTopicEditor from "./NewsroomBriefTopicEditor.vue"
import NewsroomBriefEditionEditor from "./NewsroomBriefEditionEditor.vue"
import NewsroomBriefLimit from "./NewsroomBriefLimit.vue"
import { formatRelativeTime } from "../../utils/format/time"
import {
  EDITIONS_TITLE, MODEL_CALLS_TITLE, NEED_TOPIC_FIRST, NEW_EDITION_LABEL, NEW_TOPIC_LABEL, NO_EDITIONS_SETUP_BODY, NO_EDITIONS_TITLE, NO_TOPICS_BODY, READ_NOW_LABEL, TOPICS_TITLE,
  briefAsOfWords, editionSummaryWords,
} from "../../constants/newsroom"
import type { BriefEdition, BriefEditionEdit, BriefSettings, BriefTopic, BriefTopicEdit } from "../../types/newsroom"

// Topics and editions: what is read, and what is written from it. The editing state is here; the page owns the data and does the saving, deleting and
// reading (it asks before anything is deleted), and each action answers whether it worked so the form closes only on success.
const NEW = "new" as const
// A new form sits above the list, so it is set apart from the first row.
const NEW_FORM_SPACING = "mb-4 border-b border-hairline pb-4"
type Target = number | typeof NEW | null
defineProps<{
  topics: BriefTopic[]
  editions: BriefEdition[]
  settings: BriefSettings | null
  saving: boolean
  error: string
  saveTopic: (id: number | null, edit: BriefTopicEdit) => Promise<boolean>
  saveEdition: (id: number | null, edit: BriefEditionEdit) => Promise<boolean>
  saveLimit: (calls: number) => Promise<boolean>
}>()
const emit = defineEmits<{
  (e: "delete-topic", topic: BriefTopic): void
  (e: "delete-edition", edition: BriefEdition): void
  (e: "read-topic", topic: BriefTopic): void
}>()

const editingTopic = ref<Target>(null)
const editingEdition = ref<Target>(null)
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
        :saving="saving"
        :error="error"
        :class="topics.length ? NEW_FORM_SPACING : ''"
        @save="async (edit) => { if (await saveTopic(null, edit)) editingTopic = null }"
        @cancel="editingTopic = null"
      />
      <p v-else-if="!topics.length" class="m-0 text-muted">{{ NO_TOPICS_BODY }}</p>
      <ul v-if="topics.length" class="m-0 flex list-none flex-col divide-y divide-hairline p-0" aria-label="Topics">
        <li v-for="t in topics" :key="t.id" class="flex flex-col gap-2 py-3 first:pt-0 last:pb-0">
          <NewsroomBriefTopicEditor
            v-if="editingTopic === t.id"
            :topic="t"
            :saving="saving"
            :error="error"
            @save="async (edit) => { if (await saveTopic(t.id, edit)) editingTopic = null }"
            @cancel="editingTopic = null"
          />
          <template v-else>
            <div class="flex flex-wrap items-center gap-2">
              <span class="min-w-0 flex-1 break-words font-medium text-primary">{{ t.name }}</span>
              <BaseButton variant="ghost" size="sm" :disabled="saving" @click="emit('read-topic', t)">{{ READ_NOW_LABEL }}</BaseButton>
              <BaseButton variant="ghost" size="sm" icon="edit" icon-only :label="`Edit ${t.name}`" @click="editingTopic = t.id" />
              <BaseButton variant="ghost" size="sm" icon="trash" icon-only :label="`Delete ${t.name}`" @click="emit('delete-topic', t)" />
            </div>
            <p v-if="t.last_collected" class="m-0 text-[length:var(--text-small)] text-faint">{{ briefAsOfWords(formatRelativeTime(t.last_collected)) }}</p>
            <ul v-if="t.last_problems?.length" class="m-0 list-none p-0 text-[length:var(--text-small)] text-state-error" role="status">
              <li v-for="p in t.last_problems" :key="p" class="break-words">{{ p }}</li>
            </ul>
          </template>
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
      <ul v-if="editions.length" class="m-0 flex list-none flex-col divide-y divide-hairline p-0" aria-label="Editions">
        <li v-for="e in editions" :key="e.id" class="flex flex-col gap-2 py-3 first:pt-0 last:pb-0">
          <NewsroomBriefEditionEditor
            v-if="editingEdition === e.id"
            :edition="e"
            :topics="topics"
            :saving="saving"
            :error="error"
            @save="async (edit) => { if (await saveEdition(idOf(editingEdition), edit)) editingEdition = null }"
            @cancel="editingEdition = null"
          />
          <div v-else class="flex flex-wrap items-center gap-2">
            <span class="min-w-0 flex-1 break-words"><span class="font-medium text-primary">{{ e.name }}</span> <span class="text-[length:var(--text-small)] text-faint">{{ editionSummaryWords(e.ready_by, e.sections.length) }}</span></span>
            <BaseButton variant="ghost" size="sm" icon="edit" icon-only :label="`Edit ${e.name}`" @click="editingEdition = e.id" />
            <BaseButton variant="ghost" size="sm" icon="trash" icon-only :label="`Delete ${e.name}`" @click="emit('delete-edition', e)" />
          </div>
        </li>
      </ul>
    </Panel>

    <Panel v-if="settings" :title="MODEL_CALLS_TITLE">
      <NewsroomBriefLimit :settings="settings" :saving="saving" :save="saveLimit" />
    </Panel>
  </div>
</template>
