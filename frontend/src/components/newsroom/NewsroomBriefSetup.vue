<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Panel from "../common/layout/Panel.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import NewsroomBriefTopics from "./NewsroomBriefTopics.vue"
import NewsroomBriefEditionEditor from "./NewsroomBriefEditionEditor.vue"
import NewsroomBriefLimit from "./NewsroomBriefLimit.vue"
import NewsroomBriefMySources from "./NewsroomBriefMySources.vue"
import { EDITIONS_TITLE, EDITION_OFF_LABEL, EDITION_ON_BADGE, MODEL_LIMIT_TITLE, MY_SOURCES_TITLE, NEW_EDITION_LABEL, NO_EDITIONS_SETUP_BODY, NO_EDITIONS_TITLE, editionCardWords, gatherWords } from "../../constants/newsroom"
import { editionBlocks, hoursOf, sectionGroups } from "../../utils/newsroom/briefForm"
import type { BriefEdition, BriefEditionEdit, BriefSettings, BriefSource, BriefTopic, BriefTopicEdit } from "../../types/newsroom"

// Topics and editions: what is read (the Topics panel), and what is written from it. The edition editing state is here; the page owns the data and does the saving, deleting and
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

const editingEdition = ref<Target>(null)
const groups = computed(() => sectionGroups(props.topics))
const gather = (e: BriefEdition): string => gatherWords(hoursOf(e.collect_every) ?? 0, hoursOf(e.collect_for) ?? 0)
const idOf = (t: Target): number | null => (typeof t === "number" ? t : null)
</script>

<template>
  <div class="flex flex-col gap-4">
    <NewsroomBriefTopics
      :topics="topics"
      :sources="sources"
      :saving="saving"
      :error="error"
      :save-topic="saveTopic"
      :add-source="addSource"
      @delete-topic="(t) => emit('delete-topic', t)"
      @read-topic="(t) => emit('read-topic', t)"
    />

    <Panel :title="EDITIONS_TITLE">
      <template #actions>
        <BaseButton size="sm" icon="plus" :disabled="editingEdition === NEW" @click="editingEdition = NEW">{{ NEW_EDITION_LABEL }}</BaseButton>
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
            <div class="flex flex-wrap gap-1.5"><span v-for="b in editionBlocks(e, groups)" :key="b" :class="CHIP">{{ b }}</span></div>
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
