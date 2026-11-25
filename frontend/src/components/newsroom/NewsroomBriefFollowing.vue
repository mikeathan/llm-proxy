<script setup lang="ts">
import { computed } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import NewsroomBriefSetup from "./NewsroomBriefSetup.vue"
import { editionJoining } from "../../utils/newsroom/briefForm"
import { useBriefs } from "../../composables/newsroom/useBriefs"
import { useConfirm } from "../../composables/ui/useConfirm"
import { useToast } from "../../composables/useToast"
import {
  DELETE_ALL_DONE, DELETE_ALL_LABEL, DELETE_ALL_MESSAGE, NEWSROOM_RELOAD_HINT, deleteAllTitle, deleteEditionMessage, deleteEditionTitle, deleteTopicMessage, deleteTopicTitle, topicReadWords,
} from "../../constants/newsroom"
import type { BriefEdition, BriefEditionEdit, BriefTopic, BriefTopicEdit } from "../../types/newsroom"

// What goes in the brief, on the Following tab: the topics under their sections, the editions, the owner's own sources and the daily limit. Reading the brief
// is on the Brief tab; this view only sets it up. "Delete all briefs" is here, where the things it deletes are.
const b = useBriefs()
const { confirm } = useConfirm()
const toast = useToast()

const saved = (ok: boolean, words: string): boolean => {
  if (ok) toast.success(words)
  return ok
}
// A topic made in a section joins the editions that already read that section, so adding Greece to News puts it in every brief that has News.
async function joinEditions(known: ReadonlySet<number>) {
  const added = b.topics.value.find((t) => !known.has(t.id))
  if (!added) return
  for (const e of b.editions.value) {
    const join = editionJoining(e, b.topics.value, added)
    if (join && !(await b.saveEdition(e.id, join))) toast.error(b.actionError.value)
  }
}
const saveTopic = async (id: number | null, edit: BriefTopicEdit): Promise<boolean> => {
  if (id !== null) return saved(await b.saveTopic(id, edit), "Topic saved")
  const known = new Set(b.topics.value.map((t) => t.id))
  const ok = saved(await b.createTopic(edit), "Topic saved")
  if (ok) await joinEditions(known)
  return ok
}
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
// Anything to delete: a topic, an edition, or a source the owner added.
const hasContent = computed(() => b.topics.value.length > 0 || b.editions.value.length > 0 || b.sources.value.some((s) => !s.builtin))
async function deleteAll() {
  if (!(await confirm({ title: deleteAllTitle, message: DELETE_ALL_MESSAGE, type: "error", confirmText: DELETE_ALL_LABEL }))) return
  if (await b.deleteAll()) toast.success(DELETE_ALL_DONE)
  else toast.error(b.actionError.value)
}
async function readTopic(t: BriefTopic) {
  const done = await b.collectTopic(t.id)
  if (done) toast.success(topicReadWords(done.new))
  else toast.error(b.actionError.value)
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <LoadingState v-if="b.loading.value" label="Loading the brief's setup" :rows="3" />
    <ErrorState v-else-if="b.error.value" title="The brief's setup is unavailable" :cause="b.error.value" :next="NEWSROOM_RELOAD_HINT" />
    <template v-else>
      <NewsroomBriefSetup
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
        :import-sources="b.importSources"
        @delete-topic="deleteTopic"
        @delete-edition="deleteEdition"
        @read-topic="readTopic"
      />
      <BaseButton v-if="hasContent" class-name="self-start" variant="danger" size="sm" :disabled="b.saving.value" data-test="delete-all-briefs" @click="deleteAll">{{ DELETE_ALL_LABEL }}</BaseButton>
    </template>
  </div>
</template>
