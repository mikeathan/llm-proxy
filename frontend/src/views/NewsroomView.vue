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
import NewsroomBriefs from "../components/newsroom/NewsroomBriefs.vue"
import NewsroomBriefFollowing from "../components/newsroom/NewsroomBriefFollowing.vue"
import NewsroomFollowForm from "../components/newsroom/NewsroomFollowForm.vue"
import { useFollow } from "../composables/newsroom/useFollow"
import SegmentedControl from "../components/common/forms/SegmentedControl.vue"
import NewsroomFeed from "../components/newsroom/NewsroomFeed.vue"
import NewsroomLeftOut from "../components/newsroom/NewsroomLeftOut.vue"
import { useAlertsUnread } from "../composables/newsroom/useAlertsUnread"
import NewsroomWatchEditor from "../components/newsroom/NewsroomWatchEditor.vue"
import NewsroomWatchRow from "../components/newsroom/NewsroomWatchRow.vue"
import { useNewsroom } from "../composables/newsroom/useNewsroom"
import { useNewsroomChannels } from "../composables/newsroom/useNewsroomChannels"
import { useBriefUnread } from "../composables/newsroom/useBriefUnread"
import { useConfirm } from "../composables/ui/useConfirm"
import { useToast } from "../composables/useToast"
import {
  clearedWords, clearMessage, clearTitle, deleteWatchMessage, deleteWatchTitle, FEED_EMPTY_BODY, FEED_EMPTY_TITLE, FOLLOWING_KIND_LABELS, FOLLOWING_VIEW_LABEL, FOLLOW_LABEL, FOLLOW_TITLE, followDoneWords, NEW_WATCH_LABEL, NEWSROOM_RELOAD_HINT, NEWSROOM_SUBTITLE, NEWSROOM_TAB_LABELS,
  NEWSROOM_TITLE, NO_WATCHES_ACTION, NO_WATCHES_BODY, NO_WATCHES_TITLE, runDoneWords, unreadBriefWords, ALERTS_VIEW_LABEL, ALERT_PROBLEMS_LINK, KEPT_TOAST, leftOutViewWords, unreadAlertsWords,
} from "../constants/newsroom"
import { toFollow, toFollowing, toNewsroom } from "../router/routes"
import { dayEntries } from "../utils/newsroom/dayEntries"
import { alertProblems } from "../utils/newsroom/alertProblems"
import type { StackTabItem } from "../types/stacks"
import type { BriefFollowEdit, NewsroomWatch, NewsroomWatchEdit } from "../types/newsroom"

// The Newsroom (ADR-0086): the brief first, then Alerts (what the watches found the moment they found it), Following (everything you follow: the brief's topics
// and editions, and the watches that alert you) and the stack's modules (ADR-0068, ADR-0070: pages control, cards only show).
const route = useRoute()
const router = useRouter()
const tab = computed<"brief" | "alerts" | "following" | "settings">(() => {
  const asked = route.params.tab
  return asked === "alerts" || asked === "following" || asked === "settings" ? asked : "brief"
})
// What the Following tab shows: the brief's topics and editions, or the watches that alert you. It is in the address (?kind=alerts), so a link can name it.
const kind = computed<"brief" | "alerts">({
  get: () => (route.query.kind === "alerts" ? "alerts" : "brief"),
  set: (k) => void router.replace(toFollowing(k)),
})
const KINDS = [{ value: "brief", label: FOLLOWING_KIND_LABELS.brief }, { value: "alerts", label: FOLLOWING_KIND_LABELS.alerts }]
// A dot on Brief while a written brief has not been read (the state is the server's, shared with the dashboard).
const { unreadCount } = useBriefUnread()
// The unread alerts are counted on the server and shared with the sidebar, so the tab shows them while the brief is read.
const { unread: unreadAlerts, leftOut: leftOutCount } = useAlertsUnread()
const tabs = computed<readonly StackTabItem[]>(() => [
  { id: "brief", label: NEWSROOM_TAB_LABELS.brief, to: toNewsroom(), badge: { count: unreadCount.value, words: unreadBriefWords(unreadCount.value) } },
  { id: "alerts", label: NEWSROOM_TAB_LABELS.alerts, to: toNewsroom("alerts"), badge: { count: unreadAlerts.value, words: unreadAlertsWords(unreadAlerts.value) } },
  { id: "following", label: NEWSROOM_TAB_LABELS.following, to: toNewsroom("following") },
  { id: "settings", label: NEWSROOM_TAB_LABELS.settings, to: toNewsroom("settings") },
])

const NEW = "new" as const
// The watches and the feed are read only on the tabs that show them, not while the owner reads the brief.
const { watches, feed, filter, loading, error, actionError, saving, runningId, load, create, save, remove, markRead, keep, clear, run } = useNewsroom(() => tab.value === "alerts" || tab.value === "following")
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
const startFollow = () => void router.push(toFollow("", tab.value === "following" ? kind.value : "alerts"))
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

// Clearing deletes entries from storage for good, so it asks first and says how many.
async function askClear(what: { ids: number[] } | { scope: "read" | "all" }) {
  const entries = feed.value.flatMap(dayEntries)
  const count = "ids" in what ? what.ids.length : what.scope === "read" ? entries.filter((e) => e.read).length : entries.length
  if (!count) return
  const kind = "ids" in what ? "selected" : what.scope
  // With a filter on, "all" and "read" mean what is on screen: the scope would reach entries the owner cannot see, so the visible ids are sent instead.
  const filtered = filter.value.watch !== null || filter.value.unreadOnly || filter.value.days !== null
  if ("scope" in what && filtered) {
    const onlyRead = what.scope === "read"
    what = { ids: entries.filter((e) => !onlyRead || e.read).map((e) => e.id) }
  }
  const sure = await confirm({ title: clearTitle, message: clearMessage(kind, count), type: "warning", confirmText: "Delete" })
  if (!sure) return
  const gone = await clear(what)
  if (gone === null) toast.error(actionError.value)
  else toast.success(clearedWords(gone))
}

// Alerts, or what the model left out of them: the same list read with a flag, so the filter keeps both in one place.
const alertsView = computed({
  get: () => (filter.value.leftOut ? "left_out" : "alerts"),
  // The Alerts filters (watch, unread, period) have no controls on the left-out list, so switching starts from no filter rather than a hidden one.
  set: (v: string) => (filter.value = { watch: null, unreadOnly: false, days: null, leftOut: v === "left_out" }),
})
const alertsViews = computed(() => [{ value: "alerts", label: NEWSROOM_TAB_LABELS.alerts }, { value: "left_out", label: leftOutViewWords(leftOutCount.value) }])
// Only what needs a look: nothing is said while every watch reads, compares and sends.
const problems = computed(() => alertProblems(watches.value))
async function putBack(id: number) {
  if (await keep([id])) toast.success(KEPT_TOAST)
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
      <template v-if="tab === 'alerts' || tab === 'following'" #actions>
        <BaseButton size="sm" icon="plus" :disabled="followText !== null" data-test="follow-open" @click="startFollow">{{ FOLLOW_LABEL }}</BaseButton>
      </template>
    </PageHeader>
    <StackTabs label="Newsroom" :tabs="tabs" :active="tab" />
    <!-- The brief is a page to read and uses the whole width; the forms and lists of the other tabs keep the usual page width. -->
    <div :class="['flex min-w-0 flex-col gap-4', tab === 'brief' ? '' : 'max-w-[1072px]']" data-test="newsroom-body">
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
      <StackModules v-if="tab === 'settings'" stack="newsroom" />
      <NewsroomBriefs v-else-if="tab === 'brief'" />
      <template v-else-if="tab === 'following' && kind === 'brief'">
        <SegmentedControl v-model="kind" class="self-start" :options="KINDS" :label="FOLLOWING_VIEW_LABEL" variant="compact" />
        <NewsroomBriefFollowing :key="followKey" />
      </template>

      <template v-else>
        <SegmentedControl v-if="tab === 'following'" v-model="kind" class="self-start" :options="KINDS" :label="FOLLOWING_VIEW_LABEL" variant="compact" />
        <LoadingState v-if="loading" label="Loading the Newsroom" :rows="3" />
        <ErrorState v-else-if="error" title="The Newsroom is unavailable" :cause="error" :next="NEWSROOM_RELOAD_HINT" />

        <template v-else-if="tab === 'alerts'">
          <EmptyState v-if="!watches.length" :title="NO_WATCHES_TITLE" :body="NO_WATCHES_BODY">
            <template #action>
              <BaseButton size="sm" icon="plus" @click="startFollow">{{ NO_WATCHES_ACTION }}</BaseButton>
            </template>
          </EmptyState>
          <template v-else>
            <p v-if="problems.length" role="status" class="m-0 flex flex-wrap items-center gap-x-2 text-[length:var(--text-small)] text-state-running" data-test="alert-problems">
              {{ problems.join(" · ") }}
              <RouterLink :to="toFollowing('alerts')" class="text-accent-info-text hover:underline">{{ ALERT_PROBLEMS_LINK }}</RouterLink>
            </p>
            <SegmentedControl v-if="watches.some((w) => w.look_for)" v-model="alertsView" class="self-start" :options="alertsViews" :label="ALERTS_VIEW_LABEL" variant="compact" />
            <NewsroomLeftOut v-if="filter.leftOut" :days="feed" :watches="watches" :busy="saving" @keep="putBack" />
            <EmptyState v-else-if="!feed.length && !filter.unreadOnly && filter.watch === null && filter.days === null" :title="FEED_EMPTY_TITLE" :body="FEED_EMPTY_BODY" />
            <NewsroomFeed v-else :days="feed" :watches="watches" :filter="filter" @update:filter="filter = $event" @mark-read="markRead($event, true)" @clear="askClear" />
          </template>
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
                <NewsroomWatchRow v-else :watch="t" :busy="saving" :running="runningId === t.id" @run="check(t)" @edit="open(t.id)" @delete="askDelete(t)" />
              </li>
            </ul>
          </Panel>
        </template>
      </template>
    </div>
  </div>
</template>
