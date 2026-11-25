<script setup lang="ts">
import { computed, ref } from "vue"
import Icon from "../icons/Icon.vue"
import Sparkline from "../common/display/Sparkline.vue"
import NewsroomPlate from "./NewsroomPlate.vue"
import { useModelUse } from "../../composables/newsroom/useModelUse"
import {
  MODEL_USE_CALLS_TODAY, MODEL_USE_CHART_SUMMARY, MODEL_USE_CLOUD, MODEL_USE_COLUMNS, MODEL_USE_DAYS_HEADING, MODEL_USE_FAILED, MODEL_USE_FOLD_LABEL, MODEL_USE_HOURS_HEADING, MODEL_USE_HOUR_LABELS, hourLabel, hourTitle, MODEL_USE_KIND, MODEL_USE_LABEL,
  MODEL_USE_LOADING, MODEL_USE_LOCAL, MODEL_USE_NO_CALLS, MODEL_USE_NO_TOKENS, MODEL_USE_NONE_TODAY, MODEL_USE_OPEN_LABEL, MODEL_USE_TODAY_HEADING, MODEL_USE_UNAVAILABLE,
} from "../../constants/modelStats"
import { formatTokenCount } from "../../utils/format/units"
import { callsToday, chartColumns, hourColumns, speedOf, tokensOf, trendOf, weekTotals, weekdayOf } from "../../utils/modelUse"

// How much each model was used: a line with today's calls and a small trend, which opens into the week as stacked columns (local and cloud) and today's models with
// their tokens and speed. Only numbers: no prompt, reply or key is counted (ADR-0085). The numbers are live, not part of the edition, so this block is the same in an
// old edition as in today's. A server that cannot answer is said, and there is nothing to fold.
const { report, failed } = useModelUse()
const open = ref(false)

const total = computed(() => (report.value ? callsToday(report.value) : 0))
const columns = computed(() => chartColumns(report.value?.days ?? []))
const trend = computed(() => trendOf(report.value?.days ?? []))
const summary = computed(() => {
  const t = weekTotals(report.value?.days ?? [])
  return MODEL_USE_CHART_SUMMARY(columns.value.length, t.local, t.cloud)
})
// Today's calls by the hour they ended. Only drawn when there was a call today, and only four hours are named so the labels fit a phone.
const hours = computed(() => hourColumns(report.value?.hours ?? []))
const hasHours = computed(() => hours.value.some((c) => c.total > 0))
const hourTitleOf = (i: number): string => {
  const h = report.value?.hours?.[i]
  return h ? hourTitle(i, h.local, h.cloud) : ""
}
const dayTitle = (i: number): string => {
  const d = report.value?.days[i]
  return d ? `${weekdayOf(d.date)}: ${d.local} ${MODEL_USE_LOCAL.toLowerCase()}, ${d.cloud} ${MODEL_USE_CLOUD.toLowerCase()}` : ""
}
const toggle = () => (open.value = !open.value)
</script>

<template>
  <section class="flex min-w-0 flex-col rounded-[var(--radius-md)] bg-surface-raised" :aria-label="MODEL_USE_LABEL" data-test="model-use-block">
    <p v-if="failed" class="m-0 p-3 text-[length:var(--text-small)] text-state-running" data-test="model-use-unavailable">{{ MODEL_USE_UNAVAILABLE }}</p>
    <template v-else-if="report">
      <button
        type="button"
        class="flex w-full min-w-0 items-center gap-3 rounded-[var(--radius-md)] border-0 bg-transparent p-3 text-left focus-visible:outline-none focus-visible:ring-2"
        :aria-expanded="open"
        :aria-label="open ? MODEL_USE_FOLD_LABEL : MODEL_USE_OPEN_LABEL"
        data-test="model-use-toggle"
        @click="toggle"
      >
        <NewsroomPlate :label="MODEL_USE_LABEL" kind="economy" />
        <span class="flex min-w-0 flex-1 flex-col">
          <span class="text-[length:var(--text-body)] font-medium text-primary">{{ MODEL_USE_LABEL }}</span>
          <span class="text-[length:var(--text-small)] text-faint" data-test="model-use-total">{{ total ? MODEL_USE_CALLS_TODAY(total) : MODEL_USE_NO_CALLS }}</span>
        </span>
        <span class="w-14 flex-none [&>div]:mt-0"><Sparkline :values="trend" :label="MODEL_USE_LABEL" unit="calls" state="info" /></span>
        <Icon name="chevron-down" size="xs" :class-name="['flex-none text-faint transition-transform duration-fast', open ? '' : '-rotate-90'].join(' ')" />
      </button>
      <div v-if="open" class="flex min-w-0 flex-col gap-4 border-t border-hairline p-3" data-test="model-use-body">
        <div class="flex min-w-0 flex-col gap-2">
          <h3 class="m-0 text-[length:var(--text-small)] font-medium text-secondary">{{ MODEL_USE_DAYS_HEADING(columns.length) }}</h3>
          <div role="img" :aria-label="summary" class="flex h-24 items-end gap-1.5" data-test="model-use-chart">
            <div v-for="(c, i) in columns" :key="c.date" class="flex h-full min-w-0 flex-1 flex-col items-center justify-end gap-1" :title="dayTitle(i)" data-test="model-use-column">
              <div class="flex w-full max-w-7 flex-col-reverse overflow-hidden rounded-sm bg-surface-hover" :style="{ height: `${Math.max(c.heightPercent * 0.78, c.total > 0 ? 4 : 2)}%` }">
                <span class="block w-full bg-accent-brand" :style="{ height: `${c.localPercent}%` }"></span>
                <span class="block w-full bg-accent-info" :style="{ height: `${c.cloudPercent}%` }"></span>
              </div>
              <span :class="['text-[length:var(--text-small)] leading-none', i === columns.length - 1 ? 'font-medium text-primary' : 'text-faint']">{{ weekdayOf(c.date) }}</span>
            </div>
          </div>
          <p class="m-0 flex gap-4 text-[length:var(--text-small)] text-faint" data-test="model-use-legend">
            <span class="flex items-center gap-1.5"><span class="inline-block h-2.5 w-2.5 rounded-sm bg-accent-brand" aria-hidden="true"></span>{{ MODEL_USE_LOCAL }}</span>
            <span class="flex items-center gap-1.5"><span class="inline-block h-2.5 w-2.5 rounded-sm bg-accent-info" aria-hidden="true"></span>{{ MODEL_USE_CLOUD }}</span>
          </p>
        </div>
        <div v-if="hasHours" class="flex min-w-0 flex-col gap-2" data-test="model-use-hours">
          <h3 class="m-0 text-[length:var(--text-small)] font-medium text-secondary">{{ MODEL_USE_HOURS_HEADING }}</h3>
          <div role="img" :aria-label="MODEL_USE_HOURS_HEADING" class="flex h-16 items-end gap-0.5">
            <div v-for="(c, i) in hours" :key="i" class="flex h-full min-w-0 flex-1 flex-col justify-end" :title="hourTitleOf(i)" data-test="model-use-hour">
              <div class="flex w-full flex-col-reverse overflow-hidden rounded-sm bg-surface-hover" :style="{ height: `${Math.max(c.heightPercent, c.total > 0 ? 6 : 2)}%` }">
                <span class="block w-full bg-accent-brand" :style="{ height: `${c.localPercent}%` }"></span>
                <span class="block w-full bg-accent-info" :style="{ height: `${c.cloudPercent}%` }"></span>
              </div>
            </div>
          </div>
          <div class="flex text-[length:var(--text-small)] leading-none text-faint" aria-hidden="true">
            <span v-for="h in 24" :key="h" class="min-w-0 flex-1 overflow-visible whitespace-nowrap">{{ MODEL_USE_HOUR_LABELS.includes(h - 1) ? hourLabel(h - 1) : "" }}</span>
          </div>
        </div>
        <div class="flex min-w-0 flex-col gap-1">
          <h3 class="m-0 flex items-baseline justify-between text-[length:var(--text-small)] font-medium text-secondary">
            <span>{{ MODEL_USE_TODAY_HEADING }}</span><span class="font-normal text-faint">{{ MODEL_USE_COLUMNS.calls }}</span>
          </h3>
          <p v-if="!report.today.length" class="m-0 py-2 text-[length:var(--text-small)] text-muted" data-test="model-use-none">{{ MODEL_USE_NONE_TODAY }}</p>
          <ul v-else class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
            <li v-for="m in report.today" :key="m.model" class="flex min-w-0 flex-col gap-0.5 py-2" :data-kind="m.local ? 'local' : 'cloud'" data-test="model-use-row">
              <span class="flex min-w-0 items-baseline justify-between gap-3">
                <span class="flex min-w-0 items-baseline gap-2">
                  <span :class="['mt-1 inline-block h-2.5 w-2.5 flex-none rounded-sm', m.local ? 'bg-accent-brand' : 'bg-accent-info']" :title="MODEL_USE_KIND(m.local)" aria-hidden="true"></span>
                  <span class="min-w-0 break-words text-[length:var(--text-small)] text-primary">{{ m.model }}<span class="sr-only"> ({{ MODEL_USE_KIND(m.local) }})</span></span>
                </span>
                <span class="flex-none font-medium tabular-nums text-primary">{{ m.calls }}</span>
              </span>
              <span class="pl-[1.125rem] text-[length:var(--text-small)] text-faint">
                {{ tokensOf(m) > 0 ? `${formatTokenCount(tokensOf(m))} ${MODEL_USE_COLUMNS.tokens.toLowerCase()}` : MODEL_USE_NO_TOKENS }} · {{ speedOf(m) }}<template v-if="m.failed"> · <span class="text-state-error">{{ MODEL_USE_FAILED(m.failed) }}</span></template>
              </span>
            </li>
          </ul>
        </div>
      </div>
    </template>
    <p v-else class="m-0 p-3 text-[length:var(--text-small)] text-faint">{{ MODEL_USE_LOADING }}</p>
  </section>
</template>
