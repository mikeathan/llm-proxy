<script setup lang="ts">
import { computed, ref } from "vue"
import { useRoute, useRouter } from "vue-router"
import PageHeader from "../components/common/layout/PageHeader.vue"
import Panel from "../components/common/layout/Panel.vue"
import BaseButton from "../components/common/buttons/BaseButton.vue"
import LoadingState from "../components/common/feedback/LoadingState.vue"
import EmptyState from "../components/common/feedback/EmptyState.vue"
import ErrorState from "../components/common/feedback/ErrorState.vue"
import StackTabs from "../components/stacks/StackTabs.vue"
import StackModules from "../components/stacks/StackModules.vue"
import NewsroomHelp from "../components/newsroom/NewsroomHelp.vue"
import NewsroomSources from "../components/newsroom/NewsroomSources.vue"
import NewsroomFeed from "../components/newsroom/NewsroomFeed.vue"
import NewsroomTopicEditor from "../components/newsroom/NewsroomTopicEditor.vue"
import NewsroomTopicRow from "../components/newsroom/NewsroomTopicRow.vue"
import { useNewsroom } from "../composables/newsroom/useNewsroom"
import { useNewsroomChannels } from "../composables/newsroom/useNewsroomChannels"
import { useConfirm } from "../composables/ui/useConfirm"
import { useToast } from "../composables/useToast"
import {
  deleteTopicMessage, deleteTopicTitle, FEED_EMPTY_BODY, FEED_EMPTY_TITLE, NEW_TOPIC_LABEL, NEWSROOM_RELOAD_HINT, NEWSROOM_SUBTITLE, NEWSROOM_TAB_LABELS,
  NEWSROOM_TITLE, NO_SOURCES_BODY, NO_SOURCES_TITLE, NO_TOPICS_ACTION, NO_TOPICS_BODY, NO_TOPICS_TITLE, runDoneWords,
} from "../constants/newsroom"
import { toNewsroom } from "../router/routes"
import type { StackTabItem } from "../types/stacks"
import type { NewsroomTopic, NewsroomTopicEdit } from "../types/newsroom"

// The Newsroom: what is new in the topics you follow (the feed), the topics themselves, and the stack's modules (ADR-0068, ADR-0070: pages control,
// cards only show). Everything on the page comes from one composable that refreshes while the page is open.
const route = useRoute()
const router = useRouter()
const tab = computed<"feed" | "topics" | "sources" | "settings" | "help">(() => {
  const asked = route.params.tab
  return asked === "topics" || asked === "sources" || asked === "settings" || asked === "help" ? asked : "feed"
})
const tabs: readonly StackTabItem[] = [
  { id: "feed", label: NEWSROOM_TAB_LABELS.feed, to: toNewsroom() },
  { id: "topics", label: NEWSROOM_TAB_LABELS.topics, to: toNewsroom("topics") },
  { id: "sources", label: NEWSROOM_TAB_LABELS.sources, to: toNewsroom("sources") },
  { id: "settings", label: NEWSROOM_TAB_LABELS.settings, to: toNewsroom("settings") },
  { id: "help", label: NEWSROOM_TAB_LABELS.help, to: toNewsroom("help") },
]

const NEW = "new" as const
const { topics, feed, filter, loading, error, actionError, saving, create, save, remove, markRead, run } = useNewsroom()
const channels = useNewsroomChannels()
const { confirm } = useConfirm()
const toast = useToast()

// Which topic's form is open: a topic id, or NEW for the "New topic" form.
const editing = ref<number | typeof NEW | null>(null)

// The "New topic" form lives on the Topics tab, so starting one from the feed goes there first.
async function startNew() {
  await router.push(toNewsroom("topics"))
  open(NEW)
}

function open(what: number | typeof NEW) {
  actionError.value = ""
  editing.value = what
}

async function submit(target: number | typeof NEW, edit: NewsroomTopicEdit) {
  const saved = target === NEW ? await create(edit) : await save(target, edit)
  if (!saved) return
  editing.value = null
  toast.success(target === NEW ? "Topic added" : "Topic saved")
}

async function askDelete(t: NewsroomTopic) {
  const sure = await confirm({ title: deleteTopicTitle, message: deleteTopicMessage(t.name), type: "warning", confirmText: "Delete" })
  if (!sure) return
  if (await remove(t.id)) toast.success("Topic deleted")
  else toast.error(actionError.value)
}

async function check(t: NewsroomTopic) {
  const outcome = await run(t.id)
  if (outcome) toast.success(runDoneWords(outcome.new))
  else toast.error(actionError.value)
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader eyebrow="Stacks" :title="NEWSROOM_TITLE" :subtitle="NEWSROOM_SUBTITLE">
      <template v-if="tab !== 'settings' && tab !== 'help'" #actions>
        <BaseButton size="sm" icon="plus" :disabled="editing === NEW" @click="startNew">{{ NEW_TOPIC_LABEL }}</BaseButton>
      </template>
    </PageHeader>
    <StackTabs label="Newsroom" :tabs="tabs" :active="tab" />
    <StackModules v-if="tab === 'settings'" stack="newsroom" />
    <NewsroomHelp v-else-if="tab === 'help'" />

    <template v-else>
      <LoadingState v-if="loading" label="Loading the Newsroom" :rows="3" />
      <ErrorState v-else-if="error" title="The Newsroom is unavailable" :cause="error" :next="NEWSROOM_RELOAD_HINT" />

      <template v-else-if="tab === 'feed'">
        <EmptyState v-if="!topics.length" :title="NO_TOPICS_TITLE" :body="NO_TOPICS_BODY">
          <template #action>
            <BaseButton size="sm" icon="plus" @click="startNew">{{ NO_TOPICS_ACTION }}</BaseButton>
          </template>
        </EmptyState>
        <EmptyState v-else-if="!feed.length && !filter.unreadOnly && filter.topic === null" :title="FEED_EMPTY_TITLE" :body="FEED_EMPTY_BODY" />
        <NewsroomFeed v-else :days="feed" :topics="topics" :filter="filter" @update:filter="filter = $event" @mark-read="markRead($event, true)" />
        <p v-if="actionError" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ actionError }}</p>
      </template>

      <template v-else-if="tab === 'sources'">
        <EmptyState v-if="!topics.length" :title="NO_SOURCES_TITLE" :body="NO_SOURCES_BODY">
          <template #action>
            <BaseButton size="sm" icon="plus" @click="startNew">{{ NO_TOPICS_ACTION }}</BaseButton>
          </template>
        </EmptyState>
        <NewsroomSources v-else :topics="topics" :busy="saving" @check="check" />
        <p v-if="actionError" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ actionError }}</p>
      </template>

      <template v-else>
        <Panel v-if="editing === NEW" :title="NEW_TOPIC_LABEL">
          <NewsroomTopicEditor :topic="null" :channels="channels" :saving="saving" :error="actionError" @save="submit(NEW, $event)" @cancel="editing = null" />
        </Panel>
        <EmptyState v-if="!topics.length && editing !== NEW" :title="NO_TOPICS_TITLE" :body="NO_TOPICS_BODY">
          <template #action>
            <BaseButton size="sm" icon="plus" @click="open(NEW)">{{ NO_TOPICS_ACTION }}</BaseButton>
          </template>
        </EmptyState>
        <p v-if="actionError && editing === null" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ actionError }}</p>
        <Panel v-if="topics.length" title="Topics">
          <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0" aria-label="Topics you follow">
            <li v-for="t in topics" :key="t.id" class="flex flex-col gap-2 py-3 first:pt-0 last:pb-0">
              <NewsroomTopicEditor v-if="editing === t.id" :topic="t" :channels="channels" :saving="saving" :error="actionError" @save="submit(t.id, $event)" @cancel="editing = null" />
              <NewsroomTopicRow v-else :topic="t" :busy="saving" @run="check(t)" @edit="open(t.id)" @delete="askDelete(t)" />
            </li>
          </ul>
        </Panel>
      </template>
    </template>
  </div>
</template>
