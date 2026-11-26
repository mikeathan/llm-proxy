<script setup lang="ts">
import { computed } from "vue"
import { RouterLink, useRouter } from "vue-router"
import BaseButton from "../common/buttons/BaseButton.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import NewsroomFeed from "./NewsroomFeed.vue"
import NewsroomLeftOut from "./NewsroomLeftOut.vue"
import { useAlertsUnread } from "../../composables/newsroom/useAlertsUnread"
import { useNewsroom } from "../../composables/newsroom/useNewsroom"
import { useConfirm } from "../../composables/ui/useConfirm"
import { useToast } from "../../composables/useToast"
import {
  clearedWords, clearMessage, clearTitle, FEED_EMPTY_BODY, FEED_EMPTY_TITLE, FEED_LABEL, NEWSROOM_RELOAD_HINT, NO_WATCHES_ACTION, NO_WATCHES_BODY, NO_WATCHES_TITLE,
  ALERTS_VIEW_LABEL, ALERT_PROBLEMS_LINK, ALERTS_SETUP_LINK, KEPT_TOAST, NOT_FOR_ME_TOAST, leftOutViewWords,
} from "../../constants/newsroom"
import { toFollow, toFollowing } from "../../router/routes"
import { dayEntries } from "../../utils/newsroom/dayEntries"
import { alertProblems } from "../../utils/newsroom/alertProblems"

// The feed: what the watches found the moment they found it, on the dashboard's Newsroom tab (ADR-0089). Each alert carries its own decisions (read,
// Not for me, put back), and Clear deletes what is on screen after a confirmation. The watches themselves are set up on the stack page's Following.
const router = useRouter()
const { watches, feed, filter, loading, error, actionError, saving, markRead, keep, skip, clear } = useNewsroom(() => true)
const { leftOut: leftOutCount } = useAlertsUnread()
const { confirm } = useConfirm()
const toast = useToast()

const startFollow = () => void router.push(toFollow("", "alerts"))

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

// The feed, or what the model left out of it: the same list read with a flag, so the filter keeps both in one place.
const feedView = computed({
  get: () => (filter.value.leftOut ? "left_out" : "alerts"),
  // The feed's filters (watch, unread, period) have no controls on the left-out list, so switching starts from no filter rather than a hidden one.
  set: (v: string) => (filter.value = { watch: null, unreadOnly: false, days: null, leftOut: v === "left_out" }),
})
const feedViews = computed(() => [{ value: "alerts", label: FEED_LABEL }, { value: "left_out", label: leftOutViewWords(leftOutCount.value) }])
// Only what needs a look: nothing is said while every watch reads, compares and sends.
const problems = computed(() => alertProblems(watches.value))
// "Not for me": the alert leaves the feed and the watch learns from it; it can be put back from "Left out by the model".
async function notForMe(id: number) {
  if (await skip([id])) toast.success(NOT_FOR_ME_TOAST)
  else toast.error(actionError.value)
}
async function putBack(id: number) {
  if (await keep([id])) toast.success(KEPT_TOAST)
  else toast.error(actionError.value)
}
</script>

<template>
  <div class="flex min-w-0 flex-col gap-4" data-test="newsroom-feed-view">
    <LoadingState v-if="loading" label="Loading the feed" :rows="3" />
    <ErrorState v-else-if="error" title="The Newsroom is unavailable" :cause="error" :next="NEWSROOM_RELOAD_HINT" />
    <EmptyState v-else-if="!watches.length" :title="NO_WATCHES_TITLE" :body="NO_WATCHES_BODY">
      <template #action>
        <BaseButton size="sm" icon="plus" @click="startFollow">{{ NO_WATCHES_ACTION }}</BaseButton>
      </template>
    </EmptyState>
    <template v-else>
      <p v-if="problems.length" role="status" class="m-0 flex flex-wrap items-center gap-x-2 text-[length:var(--text-small)] text-state-running" data-test="alert-problems">
        {{ problems.join(" · ") }}
        <RouterLink :to="toFollowing('alerts')" class="text-accent-info-text hover:underline">{{ ALERT_PROBLEMS_LINK }}</RouterLink>
      </p>
      <div class="flex flex-wrap items-center justify-between gap-2">
        <SegmentedControl v-if="watches.some((w) => w.look_for)" v-model="feedView" :options="feedViews" :label="ALERTS_VIEW_LABEL" variant="compact" />
        <RouterLink :to="toFollowing('alerts')" class="ml-auto text-[length:var(--text-small)] text-accent-info-text hover:underline" data-test="alerts-setup">{{ ALERTS_SETUP_LINK }} →</RouterLink>
      </div>
      <NewsroomLeftOut v-if="filter.leftOut" :days="feed" :watches="watches" :busy="saving" @keep="putBack" />
      <EmptyState v-else-if="!feed.length && !filter.unreadOnly && filter.watch === null && filter.days === null" :title="FEED_EMPTY_TITLE" :body="FEED_EMPTY_BODY" />
      <NewsroomFeed v-else :days="feed" :watches="watches" :filter="filter" @update:filter="filter = $event" @mark-read="markRead($event, true)" @clear="askClear" @skip="notForMe" />
    </template>
    <p v-if="actionError" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ actionError }}</p>
  </div>
</template>
