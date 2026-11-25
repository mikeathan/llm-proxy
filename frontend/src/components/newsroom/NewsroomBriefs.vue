<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, watch } from "vue"
import { RouterLink, useRoute, useRouter } from "vue-router"
import BaseButton from "../common/buttons/BaseButton.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import NewsroomEdition from "./NewsroomEdition.vue"
import { briefStatus } from "../../utils/newsroom/briefStatus"
import { motionScroll } from "../../utils/motion"
import { useBriefs } from "../../composables/newsroom/useBriefs"
import { useBriefUnread } from "../../composables/newsroom/useBriefUnread"
import { useToast } from "../../composables/useToast"
import { toFollow, toNewsroom } from "../../router/routes"
import { lessLikeThis, moreLikeThis } from "../../utils/newsroom/storyTuning"
import {
  BRIEF_EARLIER_LABEL, BRIEF_EDITION_LABEL, FOLLOWING_VIEW_LABEL, MAKE_ANOTHER_LABEL, MAKE_NOW_LABEL, NEWSROOM_RELOAD_HINT, NEW_BRIEF_LABEL, NO_EDITIONS_BODY, NO_EDITIONS_TITLE, NO_ISSUE_BODY, NO_ISSUE_HEADLINE,
  briefEarlierOption, writeStepWords, STORY_TUNE_FULL, storyTunedWords,
} from "../../constants/newsroom"

// The Brief tab: the edition the owner picked, as it was last written. Reading asks nothing of the model; "Write a new one" starts a write on the server and
// the button says how far it has got. Setting up topics and editions is on the Following tab. A link can name the edition, a section or a story
// (?edition=&section=&story=): the page opens that edition and scrolls to the place.
const b = useBriefs()
const route = useRoute()
const router = useRouter()
const toast = useToast()
// An edition on screen, in a page the owner can see, counts as read: the server keeps it and the dots go. A page left in a hidden tab does not read what arrives.
const unread = useBriefUnread()

const queryNumber = (key: string): number | null => {
  const v = route.query[key]
  const n = typeof v === "string" ? Number(v) : NaN
  return Number.isInteger(n) && n > 0 ? n : null
}
// The edition a link names wins; otherwise, with several briefs, the page opens on the first unread one, so a link from a dot lands on what the dot is about.
watch([() => route.query.edition, () => b.editions.value.length], () => {
  const asked = queryNumber("edition")
  if (asked !== null && b.editions.value.some((e) => e.id === asked)) b.pickedId.value = asked
}, { immediate: true })

function markShown() {
  if (b.viewingEarlier.value || b.loading.value || !unread.loaded.value || document.visibilityState !== "visible") return
  if (b.pickedId.value === null) {
    const first = b.editions.value.find((e) => unread.isUnread(e.id))
    if (first) b.pickedId.value = first.id
  }
  const id = b.issue.value?.id
  if (id !== undefined) void unread.markRead(id)
}
watch([() => b.issue.value?.id, () => b.loading.value, unread.loaded], markShown)
onMounted(() => document.addEventListener("visibilitychange", markShown))
onBeforeUnmount(() => document.removeEventListener("visibilitychange", markShown))

// Scrolls to the section or story a link names, once the issue it is in is on screen.
watch([() => b.issue.value?.id, () => route.query.section, () => route.query.story], async () => {
  if (!b.issue.value) return
  const story = queryNumber("story")
  const section = queryNumber("section")
  const id = story !== null ? `brief-story-${story}` : section !== null ? `brief-section-${section}` : null
  if (id === null) return
  await nextTick()
  document.getElementById(id)?.scrollIntoView({ behavior: motionScroll("smooth"), block: "start" })
}, { immediate: true })

const status = computed(() => (b.edition.value ? briefStatus(b.edition.value, b.issue.value) : briefStatus({ ready_by: "", days: [], enabled: false }, null)))
const isNew = computed(() => (b.edition.value ? unread.isUnread(b.edition.value.id) : false))
const writeLabel = computed(() => (b.making.value && b.job.value ? writeStepWords(b.job.value.step) : b.issue.value ? MAKE_ANOTHER_LABEL : MAKE_NOW_LABEL))
const earlierOptions = computed(() =>
  b.editionIssues.value.map((i, n) => ({ value: String(i.id), label: briefEarlierOption(new Date(i.made_at).toLocaleString(undefined, { weekday: "short", day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" }), n === 0) })),
)
const editionOptions = computed(() => b.editions.value.map((e) => ({ value: String(e.id), label: unread.isUnread(e.id) ? `${e.name} · ${NEW_BRIEF_LABEL}` : e.name })))

// The name of each topic, so a story can say which topic "more" or "less like this" changes.
const topicNames = computed(() => Object.fromEntries(b.topics.value.map((t) => [t.id, t.name])))
async function tune(topicId: number, more: boolean, words: string[]) {
  const topic = b.topics.value.find((t) => t.id === topicId)
  const edit = topic ? (more ? moreLikeThis(topic, words) : lessLikeThis(topic, words)) : null
  if (!topic || !edit) {
    toast.info(STORY_TUNE_FULL)
    return
  }
  if (await b.saveTopic(topic.id, edit)) toast.success(storyTunedWords(more, topic.name))
  else toast.error(b.actionError.value)
}
const followStory = (words: string[]) => void router.push(toFollow(words.join(" ")))

async function write() {
  if (!(await b.makeNow())) toast.error(b.actionError.value)
}
// A write that failed on the server says why, once.
watch(() => b.job.value?.state, (state) => { if (state === "failed" && b.actionError.value) toast.error(b.actionError.value) })
</script>

<template>
  <div class="flex flex-col gap-4">
    <LoadingState v-if="b.loading.value" label="Loading the brief" :rows="3" />
    <ErrorState v-else-if="b.error.value" title="The brief is unavailable" :cause="b.error.value" :next="NEWSROOM_RELOAD_HINT" />
    <EmptyState v-else-if="!b.editions.value.length" :title="NO_EDITIONS_TITLE" :body="NO_EDITIONS_BODY">
      <template #action><RouterLink :to="toNewsroom('following')" class="text-accent-brand hover:underline">{{ FOLLOWING_VIEW_LABEL }} →</RouterLink></template>
    </EmptyState>
    <template v-else-if="b.edition.value">
      <NewsroomEdition v-if="b.issue.value" :issue="b.issue.value" :status="status" :is-new="isNew" :topics="topicNames" @tune="tune" @follow="followStory">
        <template #masthead-end>
          <SegmentedControl v-if="b.editions.value.length > 1" :model-value="String(b.edition.value.id)" :options="editionOptions" :label="BRIEF_EDITION_LABEL" variant="compact" @update:model-value="b.pickedId.value = Number($event)" />
          <div v-if="b.editionIssues.value.length > 1" data-test="earlier-briefs">
            <SelectInput :model-value="String(b.issue.value.id)" :options="earlierOptions" :label="BRIEF_EARLIER_LABEL" @update:model-value="b.pickedIssueId.value = Number($event)" />
          </div>
          <BaseButton size="sm" variant="secondary" :disabled="b.making.value" data-test="write-now" @click="write">{{ writeLabel }}</BaseButton>
        </template>
      </NewsroomEdition>
      <LoadingState v-else-if="b.editionIssues.value.length" label="Reading the brief" :rows="3" />
      <SegmentedControl v-if="!b.editionIssues.value.length && b.editions.value.length > 1" class="self-start" :model-value="String(b.edition.value.id)" :options="editionOptions" :label="BRIEF_EDITION_LABEL" variant="compact" @update:model-value="b.pickedId.value = Number($event)" />
      <EmptyState v-if="!b.editionIssues.value.length" :title="NO_ISSUE_HEADLINE" :body="NO_ISSUE_BODY">
        <template #action><BaseButton size="sm" :disabled="b.making.value" data-test="write-now" @click="write">{{ writeLabel }}</BaseButton></template>
      </EmptyState>
    </template>
  </div>
</template>
