<script setup lang="ts">
import { ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import NewsroomEdition from "./NewsroomEdition.vue"
import NewsroomBriefSetup from "./NewsroomBriefSetup.vue"
import { useBriefs } from "../../composables/newsroom/useBriefs"
import { useConfirm } from "../../composables/ui/useConfirm"
import { useToast } from "../../composables/useToast"
import {
  BRIEFS_TAB_LABELS, BRIEFS_VIEW_LABEL, BRIEF_EDITION_LABEL, MAKE_NOW_LABEL, MAKING_LABEL, NEWSROOM_RELOAD_HINT, NO_EDITIONS_BODY, NO_EDITIONS_TITLE, NO_ISSUE_BODY, NO_ISSUE_HEADLINE, READ_EDITION_READY,
  deleteEditionMessage, deleteEditionTitle, deleteTopicMessage, deleteTopicTitle, topicReadWords,
} from "../../constants/newsroom"
import type { BriefEdition, BriefEditionEdit, BriefTopic, BriefTopicEdit } from "../../types/newsroom"

// The Briefs tab: the edition the owner picked, as it was last written (reading asks nothing of the model, except when the owner presses "Write it now"),
// and the setup of topics and editions. One composable feeds both.
const b = useBriefs()
const { confirm } = useConfirm()
const toast = useToast()
const mode = ref<"read" | "setup">("read")
const MODES = [{ value: "read", label: BRIEFS_TAB_LABELS.read }, { value: "setup", label: BRIEFS_TAB_LABELS.setup }]

async function write() {
  if (!(await b.makeNow())) toast.error(b.actionError.value)
}

const saved = (ok: boolean, words: string): boolean => {
  if (ok) toast.success(words)
  return ok
}
const saveTopic = async (id: number | null, edit: BriefTopicEdit): Promise<boolean> => saved(id === null ? await b.createTopic(edit) : await b.saveTopic(id, edit), "Topic saved")
const saveEdition = async (id: number | null, edit: BriefEditionEdit): Promise<boolean> => saved(id === null ? await b.createEdition(edit) : await b.saveEdition(id, edit), "Edition saved")

const addSource = async (label: string, target: string): Promise<boolean> => saved(await b.addSource(label, target), "Source added")
const removeSource = async (id: number): Promise<boolean> => {
  const ok = await b.removeSource(id)
  if (!ok) toast.error(b.actionError.value)
  return ok
}

const saveLimit = async (calls: number): Promise<boolean> => saved(await b.saveSettings(calls), "Limit saved")

async function deleteTopic(t: BriefTopic) {
  if (!(await confirm({ title: deleteTopicTitle, message: deleteTopicMessage(t.name), type: "warning", confirmText: "Delete" }))) return
  if (!(await b.removeTopic(t.id))) toast.error(b.actionError.value)
}
async function deleteEdition(e: BriefEdition) {
  if (!(await confirm({ title: deleteEditionTitle, message: deleteEditionMessage(e.name), type: "warning", confirmText: "Delete" }))) return
  if (!(await b.removeEdition(e.id))) toast.error(b.actionError.value)
}
async function readTopic(t: BriefTopic) {
  const done = await b.collectTopic(t.id)
  if (done) toast.success(topicReadWords(done.new))
  else toast.error(b.actionError.value)
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <LoadingState v-if="b.loading.value" label="Loading the briefs" :rows="3" />
    <ErrorState v-else-if="b.error.value" title="The briefs are unavailable" :cause="b.error.value" :next="NEWSROOM_RELOAD_HINT" />
    <template v-else>
      <SegmentedControl v-model="mode" :options="MODES" :label="BRIEFS_VIEW_LABEL" variant="compact" />
      <NewsroomBriefSetup
        v-if="mode === 'setup'"
        :topics="b.topics.value"
        :editions="b.editions.value"
        :settings="b.settings.value"
        :sources="b.sources.value"
        :saving="b.saving.value"
        :error="b.actionError.value"
        :save-topic="saveTopic"
        :save-edition="saveEdition"
        :save-limit="saveLimit"
        :add-source="addSource"
        :remove-source="removeSource"
        @delete-topic="deleteTopic"
        @delete-edition="deleteEdition"
        @read-topic="readTopic"
      />
      <EmptyState v-else-if="!b.editions.value.length" :title="NO_EDITIONS_TITLE" :body="NO_EDITIONS_BODY">
        <template #action><BaseButton size="sm" @click="mode = 'setup'">{{ BRIEFS_TAB_LABELS.setup }}</BaseButton></template>
      </EmptyState>
      <template v-else-if="b.edition.value">
        <div class="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-[var(--radius-md)] bg-surface-raised p-4" data-test="edition-header">
          <SegmentedControl
            v-if="b.editions.value.length > 1"
            :model-value="String(b.edition.value.id)"
            :options="b.editions.value.map((e) => ({ value: String(e.id), label: e.name }))"
            :label="BRIEF_EDITION_LABEL"
            variant="compact"
            @update:model-value="b.pickedId.value = Number($event)"
          />
          <div v-else class="flex min-w-0 flex-col">
            <h2 class="m-0 text-[length:var(--text-h3,1.25rem)] font-semibold text-primary">{{ b.edition.value.name }}</h2>
            <span class="text-[length:var(--text-small)] text-faint">{{ READ_EDITION_READY(b.edition.value.ready_by, b.edition.value.sections.length) }}</span>
          </div>
          <BaseButton class-name="ml-auto" size="sm" variant="secondary" :disabled="b.making.value" @click="write">{{ b.making.value ? MAKING_LABEL : MAKE_NOW_LABEL }}</BaseButton>
        </div>
        <NewsroomEdition v-if="b.issue.value" :issue="b.issue.value" />
        <EmptyState v-else :title="NO_ISSUE_HEADLINE" :body="NO_ISSUE_BODY">
          <template #action><BaseButton size="sm" :disabled="b.making.value" @click="write">{{ b.making.value ? MAKING_LABEL : MAKE_NOW_LABEL }}</BaseButton></template>
        </EmptyState>
      </template>
    </template>
  </div>
</template>
