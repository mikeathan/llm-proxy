<script setup lang="ts">
import { computed, ref, watch as onChange } from "vue"
import { RouterLink, useRoute, useRouter } from "vue-router"
import PageHeader from "../components/common/layout/PageHeader.vue"
import Panel from "../components/common/layout/Panel.vue"
import BaseButton from "../components/common/buttons/BaseButton.vue"
import LoadingState from "../components/common/feedback/LoadingState.vue"
import EmptyState from "../components/common/feedback/EmptyState.vue"
import ErrorState from "../components/common/feedback/ErrorState.vue"
import StackTabs from "../components/stacks/StackTabs.vue"
import StackModules from "../components/stacks/StackModules.vue"
import StackBoardLink from "../components/stacks/StackBoardLink.vue"
import NewsroomBriefFollowing from "../components/newsroom/NewsroomBriefFollowing.vue"
import NewsroomFollowForm from "../components/newsroom/NewsroomFollowForm.vue"
import { useFollow } from "../composables/newsroom/useFollow"
import SegmentedControl from "../components/common/forms/SegmentedControl.vue"
import NewsroomWatchEditor from "../components/newsroom/NewsroomWatchEditor.vue"
import NewsroomWatchRow from "../components/newsroom/NewsroomWatchRow.vue"
import { useNewsroom } from "../composables/newsroom/useNewsroom"
import { useNewsroomChannels } from "../composables/newsroom/useNewsroomChannels"
import { useConfirm } from "../composables/ui/useConfirm"
import { useToast } from "../composables/useToast"
import {
  deleteWatchMessage, deleteWatchTitle, FOLLOWING_KIND_LABELS, FOLLOWING_VIEW_LABEL, FOLLOW_LABEL, FOLLOW_TITLE, followDoneWords, NEW_WATCH_LABEL, NEWSROOM_RELOAD_HINT, NEWSROOM_SUBTITLE, NEWSROOM_TAB_LABELS,
  NEWSROOM_TITLE, NO_WATCHES_ACTION, NO_WATCHES_BODY, NO_WATCHES_TITLE, runDoneWords, FOLLOWING_READ_LINKS,
} from "../constants/newsroom"
import { toBrief, toFeed, toFollow, toFollowing, toNewsroom } from "../router/routes"
import type { StackTabItem } from "../types/stacks"
import type { BriefFollowEdit, NewsroomWatch, NewsroomWatchEdit } from "../types/newsroom"

// The Newsroom's stack page (ADR-0089): everything to set it up and nothing to read. Following (the page without a tab) holds what is followed, the
// brief's topics and editions and the watches that fill the feed; Settings is the stack's modules. The brief and the feed are read on the dashboard's
// Newsroom tab, which the line under the tabs links to.
const route = useRoute()
const router = useRouter()
const tab = computed<"following" | "settings">(() => (route.params.tab === "settings" ? "settings" : "following"))
// What the Following tab shows: the brief's topics and editions, or the watches that fill the feed. It is in the address (?kind=alerts), so a link can name it.
const kind = computed<"brief" | "alerts">({
  get: () => (route.query.kind === "alerts" ? "alerts" : "brief"),
  set: (k) => void router.replace(toFollowing(k)),
})
const KINDS = [{ value: "brief", label: FOLLOWING_KIND_LABELS.brief }, { value: "alerts", label: FOLLOWING_KIND_LABELS.alerts }]
const tabs: readonly StackTabItem[] = [
  { id: "following", label: NEWSROOM_TAB_LABELS.following, to: toNewsroom() },
  { id: "settings", label: NEWSROOM_TAB_LABELS.settings, to: toNewsroom("settings") },
]

const NEW = "new" as const
// The watches are read only on Following, where they are listed.
const { watches, loading, error, actionError, saving, runningId, load, create, save, remove, run } = useNewsroom(() => tab.value === "following")
const channels = useNewsroomChannels()
const { confirm } = useConfirm()
const toast = useToast()

// Which watch's form is open: a watch id, or NEW for the "New watch" form.
const editing = ref<number | typeof NEW | null>(null)

// The one way to follow something (ADR-0086): the Follow form on Following, open while the address carries ?follow= (a story's words, or nothing).
const f = useFollow()
const followText = computed(() => (typeof route.query.follow === "string" ? route.query.follow : null))
// Bumped after a follow so the brief's setup reads the new topic.
const followKey = ref(0)
const startFollow = () => void router.push(toFollow("", kind.value))
// Where the shown half of Following is read: the brief, or the feed, on the dashboard.
const readTo = computed(() => (kind.value === "alerts" ? toFeed() : toBrief()))
onChange(followText, (t) => { if (t !== null) void f.loadSections() }, { immediate: true })
const closeFollow = (to: "brief" | "alerts" = kind.value) => void router.replace(toFollowing(to))
async function saveFollow(edit: BriefFollowEdit) {
  const done = await f.follow(edit)
  if (!done) return
  toast.success(followDoneWords(edit.mode, done.joined))
  followKey.value++
  closeFollow(edit.mode === "alerts" ? "alerts" : "brief")
  await load()
}
async function followAdvanced() {
  closeFollow("alerts")
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
        <!-- A small outline, not the page's loudest thing; the area around it answers a finger (44 px: the inset counts from inside the border). -->
        <BaseButton size="sm" variant="secondary" icon="plus" :disabled="followText !== null" class-name="relative before:absolute before:-inset-x-1 before:-inset-y-[10px] before:content-['']" data-test="follow-open" @click="startFollow">{{ FOLLOW_LABEL }}</BaseButton>
      </template>
    </PageHeader>
    <StackTabs label="Newsroom" :tabs="tabs" :active="tab" />
    <StackBoardLink stack="newsroom" />
    <div class="flex min-w-0 max-w-[1072px] flex-col gap-4" data-test="newsroom-body">
      <Panel v-if="tab === 'following' && followText !== null" :title="FOLLOW_TITLE">
        <NewsroomFollowForm
          :initial-text="followText"
          :initial-mode="kind"
          :sections="f.sections.value"
          :channels="channels"
          :saving="f.saving.value"
          :error="f.error.value"
          :preview="f.preview.value"
          @save="saveFollow"
          @preview="f.lookUp"
          @cancel="closeFollow()"
          @advanced="followAdvanced"
        />
      </Panel>
      <!-- What you follow, and one click to where it is read (and, for alerts, to a new watch), so neither half hides behind the other. -->
      <div v-if="tab === 'following'" class="flex flex-wrap items-center gap-2">
        <SegmentedControl v-model="kind" :options="KINDS" :label="FOLLOWING_VIEW_LABEL" variant="compact" />
        <BaseButton v-if="kind === 'alerts' && editing !== NEW" size="sm" variant="secondary" icon="plus" data-test="new-watch" @click="open(NEW)">{{ NEW_WATCH_LABEL }}</BaseButton>
        <RouterLink :to="readTo" class="ml-auto text-[length:var(--text-small)] text-accent-info-text hover:underline" data-test="following-read">{{ FOLLOWING_READ_LINKS[kind] }} →</RouterLink>
      </div>
      <StackModules v-if="tab === 'settings'" stack="newsroom" />
      <template v-else-if="kind === 'brief'">
        <NewsroomBriefFollowing :key="followKey" />
      </template>

      <template v-else>
        <LoadingState v-if="loading" label="Loading the Newsroom" :rows="3" />
        <ErrorState v-else-if="error" title="The Newsroom is unavailable" :cause="error" :next="NEWSROOM_RELOAD_HINT" />

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
                <NewsroomWatchRow v-else :watch="t" :busy="saving" :running="runningId === t.id" @run="check(t)" @edit="open(t.id)" @delete="askDelete(t)" />
              </li>
            </ul>
          </Panel>
        </template>
      </template>
    </div>
  </div>
</template>