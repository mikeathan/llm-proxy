<script setup lang="ts">
import { computed, reactive } from "vue"
import Disclosure from "../common/layout/Disclosure.vue"
import MarkdownViewer from "../common/display/MarkdownViewer.vue"
import StatCard from "../common/display/StatCard.vue"
import StatusTag from "../common/display/StatusTag.vue"
import { deviceLabel, severityState } from "../../utils/netaudit/friendly"
import { moduleLine, severityCounts, vulnLine, windowSummary } from "../../utils/netaudit/auditRecord"
import { formatAbsoluteTime } from "../../utils/format/time"
import type { AuditRecord, AuditRecordChange, AuditRecordWindow } from "../../types/auditRecord"

// The overview of one audit run, above the problems: the assessment, the figures, and the parts a reader opens when they want the
// detail (how the network moved over the window, the exposed services, what the background checks did, the notes). It reads the typed record the
// audit saved beside its report, so nothing here is parsed out of text. Parts fold to one line that says what is inside, as the
// saved report does. Presentation only.
const props = defineProps<{ record: AuditRecord; names?: Readonly<Record<string, string>> }>()

const SECTION = { window: "window", exposed: "exposed", modules: "modules", notes: "notes" } as const
const open = reactive<Record<string, boolean>>({})
const setOpen = (key: string, value: boolean) => { open[key] = value }

const severities = computed(() => severityCounts(props.record.counts))
const window = computed(() => props.record.window)
const modules = computed(() => window.value?.modules ?? [])
const exposed = computed(() => props.record.exposed)
const vuln = computed(() => vulnLine(props.record.vulns))
const nameOf = (device: string) => props.names?.[device]
const placeOf = (c: AuditRecordChange) => [deviceLabel(c.device), c.port ? `port ${c.port}` : ""].filter(Boolean).join(" · ")
const changeGroups = computed(() => {
  const w = window.value
  if (!w) return []
  return [
    { label: "New", items: w.new_in_window, more: w.new_more ?? 0 },
    { label: "Gone", items: w.gone_in_window, more: w.gone_more ?? 0 },
    { label: "Came back", items: w.returned_in_window, more: w.returned_more ?? 0 },
  ].filter((g) => g.items.length > 0)
})
const ACTIVITY_LABELS: [keyof NonNullable<AuditRecordWindow["activity"]>, string][] = [
  ["new_devices", "new devices"], ["address_changes", "address changes"], ["ports_opened", "ports opened"],
  ["ports_closed", "ports closed"], ["router_refused_attempts", "connection attempts the router refused"],
]
const activity = computed<[string, number][]>(() => {
  const a = window.value?.activity
  if (!a) return []
  return ACTIVITY_LABELS.map(([key, label]): [string, number] => [label, a[key] ?? 0]).filter(([, n]) => n > 0)
})
const modulesSummary = computed(() => modules.value.map((m) => m.title).join(" · "))
const exposedSummary = computed(() => `${exposed.value.length} ${exposed.value.length === 1 ? "service" : "services"}`)
const notesSummary = computed(() => `${props.record.notes.length} ${props.record.notes.length === 1 ? "note" : "notes"}`)
const hasWindowBody = computed(() => changeGroups.value.length > 0 || activity.value.length > 0 || (window.value?.unread.length ?? 0) > 0)
</script>

<template>
  <section aria-label="Audit overview" class="flex min-w-0 flex-col gap-4" data-test="audit-overview">
    <div v-if="record.assessment" class="flex min-w-0 flex-col gap-2 border border-hairline bg-surface p-4" data-test="overview-assessment">
      <h3 class="m-0 font-mono text-[length:var(--text-small)] font-medium uppercase tracking-[0.04em] text-primary">Assessment</h3>
      <MarkdownViewer :content="record.assessment" />
    </div>

    <div class="grid grid-cols-2 gap-px border border-hairline bg-border-hairline sm:grid-cols-4" data-test="overview-stats">
      <StatCard label="Open problems" :value="String(record.counts.open)">
        <p v-if="severities.length" class="m-0 flex flex-wrap gap-1" data-test="severity-split">
          <StatusTag v-for="s in severities" :key="s.severity" :state="severityState(s.severity)" :label="`${s.count} ${s.severity}`" />
        </p>
      </StatCard>
      <StatCard label="New" :value="String(record.counts.new)" caption="since the scan before" />
      <StatCard label="Resolved" :value="String(record.counts.resolved)" caption="since the scan before" />
      <StatCard label="Devices up" :value="String(record.up)" :unit="`of ${record.scanned}`" :caption="`${record.counts.open_ports} open ports`" />
    </div>

    <p v-if="vuln" class="m-0 text-[length:var(--text-small)] text-secondary" data-test="overview-vulns">{{ vuln }}</p>

    <div class="flex min-w-0 flex-col gap-2">
      <Disclosure v-if="window" :title="window.window" :summary="windowSummary(window)" :expandable="hasWindowBody" :open="!!open[SECTION.window]" data-test="overview-window" @update:open="setOpen(SECTION.window, $event)">
        <div class="flex flex-col gap-3">
          <p v-if="window.unread.length" class="m-0 text-[length:var(--text-small)] text-state-warning" data-test="window-unread">Could not be read, so these figures are incomplete: {{ window.unread.join(", ") }}.</p>
          <div v-for="g in changeGroups" :key="g.label" class="flex flex-col gap-1" data-test="window-changes">
            <span class="text-[length:var(--text-small)] text-muted">{{ g.label }}</span>
            <ul class="m-0 flex list-none flex-col gap-1 p-0">
              <li v-for="(c, i) in g.items" :key="`${c.device}|${c.port}|${c.check}|${i}`" class="flex min-w-0 flex-wrap items-baseline gap-x-3 text-[length:var(--text-small)]">
                <span class="min-w-0 break-words text-primary">{{ c.check }}</span>
                <span class="min-w-0 break-all font-mono text-secondary">{{ placeOf(c) }}</span>
                <span class="text-muted">{{ formatAbsoluteTime(c.at) }}</span>
              </li>
              <li v-if="g.more" class="text-[length:var(--text-small)] text-muted">and {{ g.more }} more</li>
            </ul>
          </div>
          <ul v-if="activity.length" class="m-0 flex list-none flex-wrap gap-x-4 gap-y-1 p-0 text-[length:var(--text-small)] text-secondary" data-test="window-activity">
            <li v-for="[label, n] in activity" :key="label"><span class="font-mono text-primary">{{ n }}</span> {{ label }}</li>
          </ul>
        </div>
      </Disclosure>

      <Disclosure v-if="exposed.length" title="Exposed services worth reviewing" :summary="exposedSummary" :open="!!open[SECTION.exposed]" data-test="overview-exposed" @update:open="setOpen(SECTION.exposed, $event)">
        <ul class="m-0 flex list-none flex-col p-0">
          <li v-for="e in exposed" :key="`${e.device}|${e.port}|${e.proto}`" class="flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-0.5 border-t border-hairline py-2 text-[length:var(--text-small)] first:border-t-0" data-test="exposed-service">
            <span class="min-w-0 break-all font-mono text-primary">{{ deviceLabel(e.device) }}</span>
            <span v-if="nameOf(e.device)" class="min-w-0 break-words text-muted">{{ nameOf(e.device) }}</span>
            <span class="font-mono text-secondary">{{ e.port }}/{{ e.proto }}</span>
            <span class="min-w-0 break-words text-secondary">{{ [e.service, e.detail].filter(Boolean).join(" · ") }}</span>
            <span class="basis-full text-muted">{{ e.kind }}</span>
          </li>
        </ul>
      </Disclosure>

      <Disclosure v-if="modules.length" title="Background checks" :summary="modulesSummary" :open="!!open[SECTION.modules]" data-test="overview-modules" @update:open="setOpen(SECTION.modules, $event)">
        <ul class="m-0 flex list-none flex-col p-0">
          <li v-for="m in modules" :key="m.title" class="flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-0.5 border-t border-hairline py-2 text-[length:var(--text-small)] first:border-t-0" data-test="module-tally">
            <span class="text-primary">{{ m.title }}</span>
            <span class="min-w-0 break-words text-secondary">{{ moduleLine(m) }}</span>
          </li>
        </ul>
      </Disclosure>

      <Disclosure v-if="record.notes.length" title="Notes" :summary="notesSummary" :open="!!open[SECTION.notes]" data-test="overview-notes" @update:open="setOpen(SECTION.notes, $event)">
        <ul class="m-0 flex list-none flex-col p-0">
          <li v-for="(n, i) in record.notes" :key="i" class="break-words border-t border-hairline py-2 text-[length:var(--text-small)] text-secondary first:border-t-0" data-test="overview-note">{{ n }}</li>
        </ul>
      </Disclosure>
    </div>
  </section>
</template>
