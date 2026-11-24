<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import NewsroomEdition from "./NewsroomEdition.vue"
import NewsroomBriefSetup from "./NewsroomBriefSetup.vue"
import { editionJoining } from "../../utils/newsroom/briefForm"
import { useBriefs } from "../../composables/newsroom/useBriefs"
import { useBriefUnread } from "../../composables/newsroom/useBriefUnread"
import { useConfirm } from "../../composables/ui/useConfirm"
import { useToast } from "../../composables/useToast"
import {
  BRIEFS_TAB_LABELS, BRIEFS_VIEW_LABEL, BRIEF_EDITION_LABEL, MAKE_NOW_LABEL, MAKING_LABEL, NEWSROOM_RELOAD_HINT, NO_EDITIONS_BODY, NO_EDITIONS_TITLE, NO_ISSUE_BODY, NO_ISSUE_HEADLINE, READ_EDITION_READY,
  NEW_BRIEF_LABEL, DELETE_ALL_DONE, DELETE_ALL_LABEL, DELETE_ALL_MESSAGE, deleteAllTitle, deleteEditionMessage, deleteEditionTitle, deleteTopicMessage, deleteTopicTitle, topicReadWords,
} from "../../constants/newsroom"
import type { BriefEdition, BriefEditionEdit, BriefTopic, BriefTopicEdit } from "../../types/newsroom"

// The Briefs tab: the edition the owner picked, as it was last written (reading asks nothing of the model, except when the owner presses "Write it now"),
// and the setup of topics and editions. One composable feeds both.
const b = useBriefs()
const { confirm } = useConfirm()
const toast = useToast()
const mode = ref<"read" | "setup">("read")
// An edition on screen, in the read view, on a page the owner can see, counts as read: the server keeps it, and the dots on the tabs go. A brief written while the
// page is open (by hand or on schedule) is read the same way, and a page left in a hidden tab does not read what arrives.
const unread = useBriefUnread()
// With several briefs the page opens on the first one that is unread (so a link from a dot lands on what the dot is about), and nothing is marked before
// both lists are known: otherwise the first brief would flash up and be marked read before the switch.
function markShown() {
  if (mode.value !== "read" || b.loading.value || !unread.loaded.value || document.visibilityState !== "visible") return
  if (b.pickedId.value === null) {
    const first = b.editions.value.find((e) => unread.isUnread(e.id))
    if (first) b.pickedId.value = first.id
  }
  const id = b.issue.value?.id
  if (id !== undefined) void unread.markRead(id)
}
watch([() => b.issue.value?.id, mode, () => b.loading.value, unread.loaded], markShown)
onMounted(() => document.addEventListener("visibilitychange", markShown))
onBeforeUnmount(() => document.removeEventListener("visibilitychange", markShown))
const MODES = [{ value: "read", label: BRIEFS_TAB_LABELS.read }, { value: "setup", label: BRIEFS_TAB_LABELS.setup }]

async function write() {
  if (!(await b.makeNow())) toast.error(b.actionError.value)
}

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
  if (await b.deleteAll()) {
    mode.value = "read"
    toast.success(DELETE_ALL_DONE)
  } else toast.error(b.actionError.value)
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
      <div class="flex flex-wrap items-center justify-between gap-3">
        <SegmentedControl v-model="mode" :options="MODES" :label="BRIEFS_VIEW_LABEL" variant="compact" />
        <BaseButton v-if="hasContent" variant="danger" size="sm" :disabled="b.saving.value" data-test="delete-all-briefs" @click="deleteAll">{{ DELETE_ALL_LABEL }}</BaseButton>
      </div>
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
            :options="b.editions.value.map((e) => ({ value: String(e.id), label: unread.isUnread(e.id) ? `${e.name} · ${NEW_BRIEF_LABEL}` : e.name }))"
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
