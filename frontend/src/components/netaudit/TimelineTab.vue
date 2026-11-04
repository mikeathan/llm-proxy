<script setup lang="ts">
import { computed, onMounted, ref, toRef } from "vue"
import Panel from "../common/layout/Panel.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import StatusTag from "../common/display/StatusTag.vue"
import Icon from "../icons/Icon.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import { useNetworkActivity } from "../../composables/netaudit/useNetworkActivity"
import { useTimelineSummary } from "../../composables/netaudit/useTimelineSummary"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { activityText, clockTime, daySummary, dropEchoedAlerts, filterRows, foldRows, groupByDay } from "../../utils/netaudit/activity"
import {
  ACTIVITY_EMPTY, ACTIVITY_EMPTY_FILTERED, ACTIVITY_FILTER_LABEL, ACTIVITY_FILTER_OPTIONS, ACTIVITY_LIMIT_ADVANCED, ACTIVITY_LIMIT_SIMPLE,
  ACTIVITY_RELATED_NOTE, ACTIVITY_RELATED_TAG, ACTIVITY_TITLE, TIMELINE_EXPLAIN_HINT, TIMELINE_EXPLAIN_LABEL, TIMELINE_EXPLAIN_NOTE, TIMELINE_EXPLAINING, TIMELINE_EXPLAIN_NOTHING,
} from "../../constants/netaudit"
import type { ActivityFilter, ActivityItem, ActivityRow, NetworkAudit } from "../../types/netaudit"

// The Timeline tab: one timeline of what happened lately, built by the server from records it already keeps (new devices,
// changed ports, the last audit, the alerts). Simple shows the latest few rows; Advanced a longer list and each device's
// addresses. It reads, and starts nothing.
const props = defineProps<{ audit: NetworkAudit | null }>()
const { advanced } = useNetauditViewMode()
const limit = computed(() => (advanced.value ? ACTIVITY_LIMIT_ADVANCED : ACTIVITY_LIMIT_SIMPLE))
const { rows, loaded, loading, error, load } = useNetworkActivity(toRef(props, "audit"), limit)
const filter = ref<ActivityFilter>("all")
const auditRef = toRef(props, "audit")
const explain = useTimelineSummary(auditRef)

// Simple reads like a summary: an alert that only repeats a new-device row is left out, and three or more new devices of one
// day are one line. Advanced lists every row. Either way the days are folded: the latest two are open (all of them when a
// filter is chosen) and an older day shows one line of counts until it is opened.
const OPEN_DAYS = 2
const visibleRows = computed(() => {
  const filtered = filterRows(rows.value, filter.value)
  return advanced.value ? filtered : dropEchoedAlerts(filtered)
})
const groups = computed(() =>
  groupByDay(visibleRows.value).map((g) => ({
    heading: g.heading,
    summary: daySummary(g.rows),
    items: advanced.value ? g.rows.map((row): ActivityItem => ({ kind: "row", row })) : foldRows(g.rows),
  })),
)
const toggled = ref<Record<string, boolean>>({})
const isOpen = (heading: string, index: number) => toggled.value[heading] ?? (filter.value !== "all" || index < OPEN_DAYS)
const toggleDay = (heading: string, index: number) => (toggled.value = { ...toggled.value, [heading]: !isOpen(heading, index) })
const showsAddress = (row: ActivityRow) => advanced.value && (row.kind === "new_device" || row.kind === "address_changed" || row.kind === "related") && (row.mac || row.ip)
onMounted(() => void load())
</script>

<template>
  <Panel :title="ACTIVITY_TITLE" flush data-test="activity">
    <template #actions>
      <BaseButton variant="ghost" size="sm" icon="refresh" :loading="loading" class-name="max-sm:!h-11" @click="load">Refresh</BaseButton>
    </template>
    <div class="border-b border-hairline/60 px-4 py-3">
      <SegmentedControl v-model="filter" :options="ACTIVITY_FILTER_OPTIONS" :label="ACTIVITY_FILTER_LABEL" variant="quiet" touch />
    </div>
    <div v-if="audit" class="flex flex-col gap-2 border-b border-hairline/60 px-4 py-3" data-test="explain">
      <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
        <BaseButton variant="secondary" :loading="explain.loading.value" class-name="max-sm:!h-11" data-test="explain-button" @click="explain.explain">{{ explain.loading.value ? TIMELINE_EXPLAINING : TIMELINE_EXPLAIN_LABEL }}</BaseButton>
        <span class="text-[length:var(--text-small)] text-muted">{{ TIMELINE_EXPLAIN_HINT }}</span>
      </div>
      <p v-if="explain.error.value" role="alert" class="m-0 break-words text-[length:var(--text-small)] text-state-error" data-test="explain-error">{{ explain.error.value }}</p>
      <p v-else-if="explain.nothing.value" class="m-0 text-muted" data-test="explain-nothing">{{ TIMELINE_EXPLAIN_NOTHING }}</p>
      <div v-else-if="explain.summary.value" class="flex flex-col gap-1" data-test="explain-result">
        <p class="m-0 whitespace-pre-line break-words text-primary">{{ explain.summary.value }}</p>
        <p class="m-0 text-[length:var(--text-small)] text-muted">{{ TIMELINE_EXPLAIN_NOTE }}<template v-if="explain.model.value"> ({{ explain.model.value }})</template></p>
      </div>
    </div>
    <ErrorState v-if="error && !loaded" title="The activity could not be read" :cause="error" next="Check that the server is running, then refresh.">
      <template #action><BaseButton variant="secondary" icon="refresh" @click="load">Retry</BaseButton></template>
    </ErrorState>
    <LoadingState v-else-if="!loaded" label="Reading the activity" :rows="3" />
    <EmptyState v-else-if="!groups.length" :title="filter === 'all' ? 'Quiet' : 'Nothing here'" :body="filter === 'all' ? ACTIVITY_EMPTY : ACTIVITY_EMPTY_FILTERED" />
    <template v-else>
      <p v-if="error" role="alert" class="m-0 px-4 py-2 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
      <div class="max-w-3xl">
        <section v-for="(g, gi) in groups" :key="g.heading" :aria-label="g.heading" data-test="activity-day">
          <h3 class="m-0">
            <button
              type="button"
              :aria-expanded="isOpen(g.heading, gi)"
              class="flex min-h-11 w-full items-center gap-2 border-0 bg-transparent px-4 py-2 text-left hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset"
              data-test="day-toggle"
              @click="toggleDay(g.heading, gi)"
            >
              <Icon :name="isOpen(g.heading, gi) ? 'chevron-down' : 'chevron-right'" size="sm" class-name="flex-none text-muted" />
              <span class="flex min-w-0 flex-wrap items-baseline gap-x-3">
                <span class="font-mono text-[length:var(--text-micro)] font-medium uppercase tracking-[var(--tracking-micro)] text-muted">{{ g.heading }}</span>
                <span v-if="!isOpen(g.heading, gi)" class="text-[length:var(--text-small)] font-normal text-secondary" data-test="day-summary">{{ g.summary }}</span>
              </span>
            </button>
          </h3>
          <ul v-if="isOpen(g.heading, gi)" class="m-0 flex list-none flex-col p-0">
            <li
              v-for="(item, i) in g.items"
              :key="`${item.kind}|${item.kind === 'row' ? item.row.kind + item.row.at + (item.row.mac ?? item.row.ip ?? '') : item.text}|${i}`"
              :data-kind="item.kind === 'row' ? item.row.kind : 'fold'"
              class="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 border-b border-hairline/60 py-2 pl-10 pr-4 last:border-b-0"
            >
              <template v-if="item.kind === 'fold'">
                <span class="min-w-0 break-words text-primary">{{ item.text }}</span>
              </template>
              <template v-else>
                <div class="flex min-w-0 flex-col gap-1">
                  <span class="break-words text-primary">{{ activityText(item.row) }}</span>
                  <span v-if="item.row.kind === 'related'" class="flex flex-wrap items-center gap-2 text-[length:var(--text-small)] text-muted">
                    <StatusTag state="warning" :label="ACTIVITY_RELATED_TAG" />{{ ACTIVITY_RELATED_NOTE }}
                  </span>
                  <span v-if="showsAddress(item.row)" class="break-all font-mono text-[length:var(--text-small)] text-muted" data-test="activity-address">{{ [item.row.mac, item.row.ip].filter(Boolean).join(" · ") }}</span>
                </div>
                <span v-if="clockTime(item.row)" class="font-mono text-[length:var(--text-small)] text-muted">{{ clockTime(item.row) }}</span>
              </template>
            </li>
          </ul>
        </section>
      </div>
    </template>
  </Panel>
</template>
