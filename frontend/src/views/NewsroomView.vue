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
import NewsroomFeed from "../components/newsroom/NewsroomFeed.vue"
import NewsroomWatchEditor from "../components/newsroom/NewsroomWatchEditor.vue"
import NewsroomWatchRow from "../components/newsroom/NewsroomWatchRow.vue"
import { useNewsroom } from "../composables/newsroom/useNewsroom"
import { useNewsroomChannels } from "../composables/newsroom/useNewsroomChannels"
import { useConfirm } from "../composables/ui/useConfirm"
import { useToast } from "../composables/useToast"
import {
  deleteWatchMessage, deleteWatchTitle, FEED_EMPTY_BODY, FEED_EMPTY_TITLE, NEW_WATCH_LABEL, NEWSROOM_RELOAD_HINT, NEWSROOM_SUBTITLE, NEWSROOM_TAB_LABELS,
  NEWSROOM_TITLE, NO_WATCHES_ACTION, NO_WATCHES_BODY, NO_WATCHES_TITLE, runDoneWords,
} from "../constants/newsroom"
import { toNewsroom } from "../router/routes"
import type { StackTabItem } from "../types/stacks"
import type { NewsroomWatch, NewsroomWatchEdit } from "../types/newsroom"

// The Newsroom: what is new in the watches you follow (the feed), the watches themselves, and the stack's modules (ADR-0068, ADR-0070: pages control,
// cards only show). Everything on the page comes from one composable that refreshes while the page is open.
const route = useRoute()
const router = useRouter()
const tab = computed<"feed" | "watches" | "settings">(() => {
  const asked = route.params.tab
  return asked === "watches" || asked === "settings" ? asked : "feed"
})
const tabs: readonly StackTabItem[] = [
  { id: "feed", label: NEWSROOM_TAB_LABELS.feed, to: toNewsroom() },
  { id: "watches", label: NEWSROOM_TAB_LABELS.watches, to: toNewsroom("watches") },
  { id: "settings", label: NEWSROOM_TAB_LABELS.settings, to: toNewsroom("settings") },
]

const NEW = "new" as const
const { watches, feed, filter, loading, error, actionError, saving, create, save, remove, markRead, run } = useNewsroom()
const channels = useNewsroomChannels()
const { confirm } = useConfirm()
const toast = useToast()

// Which watch's form is open: a watch id, or NEW for the "New watch" form.
const editing = ref<number | typeof NEW | null>(null)

// The "New watch" form lives on the Watches tab, so starting one from the feed goes there first.
async function startNew() {
  await router.push(toNewsroom("watches"))
  open(NEW)
}

function open(what: number | typeof NEW) {
  actionError.value = ""
  editing.value = what
}

async function submit(target: number | typeof NEW, edit: NewsroomWatchEdit) {
  const saved = target === NEW ? await create(edit) : await save(target, edit)
  if (!saved) return
  editing.value = null
  toast.success(target === NEW ? "Watch added" : "Watch saved")
}

async function askDelete(t: NewsroomWatch) {
  const sure = await confirm({ title: deleteWatchTitle, message: deleteWatchMessage(t.name), type: "warning", confirmText: "Delete" })
  if (!sure) return
  if (await remove(t.id)) toast.success("Watch deleted")
  else toast.error(actionError.value)
}

async function check(t: NewsroomWatch) {
  const outcome = await run(t.id)
  if (!outcome) toast.error(actionError.value)
  else if (outcome.status === 'waiting') toast.info(outcome.detail)
  else if (outcome.status === 'failing') toast.error(outcome.detail)
  else toast.success(runDoneWords(outcome.new))
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader eyebrow="Stacks" :title="NEWSROOM_TITLE" :subtitle="NEWSROOM_SUBTITLE">
      <template v-if="tab !== 'settings'" #actions>
        <BaseButton size="sm" icon="plus" :disabled="editing === NEW" @click="startNew">{{ NEW_WATCH_LABEL }}</BaseButton>
      </template>
    </PageHeader>
    <StackTabs label="Newsroom" :tabs="tabs" :active="tab" />
    <StackModules v-if="tab === 'settings'" stack="newsroom" />

    <template v-else>
      <LoadingState v-if="loading" label="Loading the Newsroom" :rows="3" />
      <ErrorState v-else-if="error" title="The Newsroom is unavailable" :cause="error" :next="NEWSROOM_RELOAD_HINT" />

      <template v-else-if="tab === 'feed'">
        <EmptyState v-if="!watches.length" :title="NO_WATCHES_TITLE" :body="NO_WATCHES_BODY">
          <template #action>
            <BaseButton size="sm" icon="plus" @click="startNew">{{ NO_WATCHES_ACTION }}</BaseButton>
          </template>
        </EmptyState>
        <EmptyState v-else-if="!feed.length && !filter.unreadOnly && filter.watch === null" :title="FEED_EMPTY_TITLE" :body="FEED_EMPTY_BODY" />
        <NewsroomFeed v-else :days="feed" :watches="watches" :filter="filter" @update:filter="filter = $event" @mark-read="markRead($event, true)" />
        <p v-if="actionError" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ actionError }}</p>
      </template>

      <template v-else>
        <Panel v-if="editing === NEW" :title="NEW_WATCH_LABEL">
          <NewsroomWatchEditor :watch="null" :channels="channels" :saving="saving" :error="actionError" @save="submit(NEW, $event)" @cancel="editing = null" />
        </Panel>
        <EmptyState v-if="!watches.length && editing !== NEW" :title="NO_WATCHES_TITLE" :body="NO_WATCHES_BODY">
          <template #action>
            <BaseButton size="sm" icon="plus" @click="open(NEW)">{{ NO_WATCHES_ACTION }}</BaseButton>
          </template>
        </EmptyState>
        <p v-if="actionError && editing === null" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ actionError }}</p>
        <Panel v-if="watches.length" title="Watches">
          <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0" aria-label="Watches you follow">
            <li v-for="t in watches" :key="t.id" class="flex flex-col gap-2 py-3 first:pt-0 last:pb-0">
              <NewsroomWatchEditor v-if="editing === t.id" :watch="t" :channels="channels" :saving="saving" :error="actionError" @save="submit(t.id, $event)" @cancel="editing = null" />
              <NewsroomWatchRow v-else :watch="t" :busy="saving" @run="check(t)" @edit="open(t.id)" @delete="askDelete(t)" />
            </li>
          </ul>
        </Panel>
      </template>
    </template>
  </div>
</template>
