<script setup lang="ts">
import { computed, onMounted, ref, toRef } from "vue"
import Panel from "../common/layout/Panel.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import StatusTag from "../common/display/StatusTag.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import { useNetworkActivity } from "../../composables/netaudit/useNetworkActivity"
import { useTimelineSummary } from "../../composables/netaudit/useTimelineSummary"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { activityText, clockTime, filterRows, groupByDay } from "../../utils/netaudit/activity"
import {
  ACTIVITY_EMPTY, ACTIVITY_EMPTY_FILTERED, ACTIVITY_FILTER_LABEL, ACTIVITY_FILTER_OPTIONS, ACTIVITY_LIMIT_ADVANCED, ACTIVITY_LIMIT_SIMPLE,
  ACTIVITY_RELATED_NOTE, ACTIVITY_RELATED_TAG, ACTIVITY_TITLE, TIMELINE_EXPLAIN_HINT, TIMELINE_EXPLAIN_LABEL, TIMELINE_EXPLAIN_NOTE, TIMELINE_EXPLAINING, TIMELINE_EXPLAIN_NOTHING,
} from "../../constants/netaudit"
import type { ActivityFilter, ActivityRow, NetworkAudit } from "../../types/netaudit"

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

const groups = computed(() => groupByDay(filterRows(rows.value, filter.value)))
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
      <section v-for="g in groups" :key="g.heading" :aria-label="g.heading" data-test="activity-day">
        <h3 class="m-0 px-4 pb-1 pt-3 font-mono text-[length:var(--text-micro)] font-medium uppercase tracking-[var(--tracking-micro)] text-muted">{{ g.heading }}</h3>
        <ul class="m-0 flex list-none flex-col p-0">
          <li
            v-for="(r, i) in g.rows"
            :key="`${r.kind}|${r.at}|${r.mac ?? r.ip ?? ''}|${i}`"
            :data-kind="r.kind"
            class="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 border-b border-hairline/60 px-4 py-2 last:border-b-0"
          >
            <div class="flex min-w-0 flex-col gap-1">
              <span class="break-words text-primary">{{ activityText(r) }}</span>
              <span v-if="r.kind === 'related'" class="flex flex-wrap items-center gap-2 text-[length:var(--text-small)] text-muted">
                <StatusTag state="warning" :label="ACTIVITY_RELATED_TAG" />{{ ACTIVITY_RELATED_NOTE }}
              </span>
              <span v-if="showsAddress(r)" class="break-all font-mono text-[length:var(--text-small)] text-muted" data-test="activity-address">{{ [r.mac, r.ip].filter(Boolean).join(" · ") }}</span>
            </div>
            <span v-if="clockTime(r)" class="font-mono text-[length:var(--text-small)] text-muted">{{ clockTime(r) }}</span>
          </li>
        </ul>
      </section>
    </template>
  </Panel>
</template>
