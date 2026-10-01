<script setup lang="ts">
import { ref } from "vue"
import type { DataTableColumn, StatusState } from "../../types/ui"
import { formatCost, formatTokenCount } from "../../utils/format/units"
import { formatDuration } from "../../utils/format/time"
import BaseButton from "../../components/common/buttons/BaseButton.vue"
import MicroLabel from "../../components/common/display/MicroLabel.vue"
import StatusTag from "../../components/common/display/StatusTag.vue"
import IdChip from "../../components/common/display/IdChip.vue"
import DataTable from "../../components/common/display/DataTable.vue"
import Panel from "../../components/common/layout/Panel.vue"
import PageHeader from "../../components/common/layout/PageHeader.vue"
import EmptyState from "../../components/common/feedback/EmptyState.vue"
import ErrorState from "../../components/common/feedback/ErrorState.vue"
import ConfirmDialog from "../../components/ui/ConfirmDialog.vue"
import StatCard from "../../components/common/display/StatCard.vue"
import Sparkline from "../../components/common/display/Sparkline.vue"
import Meter from "../../components/common/display/Meter.vue"
import SlotBar from "../../components/common/display/SlotBar.vue"
import LogViewer from "../../components/common/display/LogViewer.vue"
import SegmentedControl from "../../components/common/forms/SegmentedControl.vue"
import SearchInput from "../../components/common/forms/SearchInput.vue"
import SelectInput from "../../components/common/forms/SelectInput.vue"
import BaseToggle from "../../components/common/buttons/BaseToggle.vue"
import Callout from "../../components/common/feedback/Callout.vue"
import UnsavedTag from "../../components/common/display/UnsavedTag.vue"

// /design → Primitives (plan D23): every Phase 5 primitive in each state —
// ready, loading, empty, error, disabled, overflow — with fixture data.

interface FixtureRun {
  id: string
  automation: string
  state: StatusState
  status: string
  tokens: number
  cost: number
  durationMs: number
}

const STATES: { state: StatusState; label: string }[] = [
  { state: "success", label: "Completed" },
  { state: "running", label: "Running" },
  { state: "queued", label: "Queued #2" },
  { state: "error", label: "Failed" },
  { state: "info", label: "Info" },
  { state: "neutral", label: "Idle" },
]
const VARIANTS = ["primary", "secondary", "ghost", "danger"] as const

const RUNS: FixtureRun[] = [
  { id: "run_7f3a9c2e41", automation: "nightly-digest", state: "success", status: "Completed", tokens: 334_100_000, cost: 12.3457, durationMs: 11_220_000 },
  { id: "run_0b91d7aa02", automation: "a-very-long-automation-name-that-overflows-its-column", state: "error", status: "Failed", tokens: 1_500, cost: 0.00123, durationMs: 8_403 },
  { id: "run_c44e019f7d", automation: "hourly-sync", state: "running", status: "Running", tokens: 999, cost: 0.5, durationMs: 125_000 },
]
const COLUMNS: DataTableColumn<FixtureRun>[] = [
  { key: "automation", label: "Automation", value: (r) => r.automation },
  { key: "status", label: "Status" },
  { key: "id", label: "Run" },
  { key: "tokens", label: "Tokens", numeric: true, value: (r) => formatTokenCount(r.tokens) },
  { key: "cost", label: "Cost", numeric: true, value: (r) => formatCost(r.cost) },
  { key: "duration", label: "Duration", numeric: true, value: (r) => formatDuration(r.durationMs) },
]
const runKey = (r: FixtureRun) => r.id

const dialogOpen = ref(false)
const segment = ref("runs")
const query = ref("")
const status = ref("")
const toggleOn = ref(true)
const toggleOff = ref(false)
const LOG_SAMPLE = "10:02:11 INFO  model qwen3-8b ready on :9001\n10:02:14 WARN  slot 1 idle for 300s\n10:03:40 ERROR request timed out after 45s"
</script>

<template>
  <section aria-labelledby="primitives-heading" class="flex flex-col gap-6 [counter-reset:section]">
    <h2 id="primitives-heading" class="font-mono text-[length:var(--text-small)] uppercase tracking-[var(--tracking-micro)] text-primary">
      05 ── Primitives
    </h2>

    <PageHeader eyebrow="Page header" title="Automations" subtitle="Scheduled and on-demand tasks across every workspace.">
      <template #actions>
        <BaseButton variant="secondary" icon="refresh">Refresh</BaseButton>
        <BaseButton icon="plus">New automation</BaseButton>
      </template>
    </PageHeader>

    <Panel title="Buttons">
      <div class="flex flex-col gap-3">
        <div v-for="variant in VARIANTS" :key="variant" data-test="button-row" class="flex flex-wrap items-center gap-2">
          <MicroLabel>{{ variant }}</MicroLabel>
          <BaseButton :variant="variant" size="sm">Small</BaseButton>
          <BaseButton :variant="variant">Default</BaseButton>
          <BaseButton :variant="variant" size="lg">Large</BaseButton>
          <BaseButton :variant="variant" disabled>Disabled</BaseButton>
          <BaseButton :variant="variant" loading>Saving</BaseButton>
          <BaseButton :variant="variant" icon="close" icon-only :label="`Close (${variant})`" />
        </div>
      </div>
    </Panel>

    <Panel title="Status and identity">
      <template #actions><MicroLabel>Text always present</MicroLabel></template>
      <div class="flex flex-col gap-3">
        <div class="flex flex-wrap gap-2">
          <StatusTag v-for="s in STATES" :key="s.state" :state="s.state" :label="s.label" />
        </div>
        <div class="flex flex-wrap items-center gap-3">
          <IdChip id="run_7f3a9c2e41b8d0" />
          <IdChip id="short" />
        </div>
      </div>
    </Panel>

    <Panel title="Metrics">
      <div class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,200px),1fr))] gap-px border border-hairline bg-border-hairline">
        <StatCard label="Throughput" value="48.6" unit="tok/s" caption="since page load">
          <Sparkline :values="[12, 20, 31, 28, 40, 44, 38, 48.6]" label="Tokens per second" unit="tok/s" />
        </StatCard>
        <StatCard label="CPU load" value="42" unit="%" caption="no history yet">
          <Sparkline :values="[]" label="CPU load" unit="%" state="info" />
        </StatCard>
        <StatCard label="Memory" value="8.0" unit="/ 16.0 GB">
          <Meter label="Memory used" :value="50" />
        </StatCard>
        <StatCard label="Apple GPU" value="61" unit="°C">
          <Meter label="VRAM used" :value="92" text="7.4 / 8.0 GB" state="running" />
        </StatCard>
      </div>
      <div class="mt-4 grid grid-cols-[repeat(auto-fit,minmax(min(100%,200px),1fr))] gap-4">
        <SlotBar label="Local lane" :used="1" :limit="1" />
        <SlotBar label="Cloud lane" :used="1" :limit="3" />
        <SlotBar label="Idle lane" :used="0" :limit="4" />
      </div>
    </Panel>

    <Panel title="Controls">
      <div class="flex flex-wrap items-center gap-3">
        <SegmentedControl
          v-model="segment"
          :options="[{ value: 'runs', label: 'Runs' }, { value: 'app', label: 'App log' }, { value: 'process', label: 'Process log' }]"
          label="Sample view"
        />
        <SelectInput v-model="status" :options="[{ value: '', label: 'Any status' }, { value: 'failed', label: 'Failed' }]" label="Sample status" />
        <SearchInput v-model="query" label="Search runs" />
      </div>
      <div class="mt-4 flex flex-wrap items-center gap-6">
        <BaseToggle v-model="toggleOn" label="Run logging" />
        <BaseToggle v-model="toggleOff" label="Queue waiting requests" />
        <BaseToggle :model-value="false" label="Disabled switch" disabled />
      </div>
    </Panel>

    <Panel title="Notices">
      <template #actions><UnsavedTag /></template>
      <div class="flex flex-col gap-3">
        <Callout tone="info" title="Applies on restart">The egress proxy starts with the backend.</Callout>
        <Callout tone="warning" title="Agent network is unrestricted">
          Restricting is recommended.
          <template #actions><BaseButton size="sm">Restrict network</BaseButton><BaseButton variant="secondary" size="sm">Keep allowed</BaseButton></template>
        </Callout>
        <Callout tone="error" title="Terminal provider error">Single-shot execution is used until it starts.</Callout>
      </div>
    </Panel>

    <Panel title="Log viewer">
      <LogViewer :text="LOG_SAMPLE" label="Sample log" />
    </Panel>

    <Panel title="Data table" flush>
      <DataTable :columns="COLUMNS" :rows="RUNS" :row-key="runKey" caption="Recent runs" activatable>
        <template #cell-status="{ row }"><StatusTag :state="row.state" :label="row.status" /></template>
        <template #cell-id="{ row }"><IdChip :id="row.id" /></template>
      </DataTable>
    </Panel>

    <div class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,280px),1fr))] gap-4">
      <Panel title="Loading">
        <DataTable :columns="COLUMNS" :rows="[]" :row-key="runKey" caption="Recent runs" loading />
      </Panel>
      <Panel title="Empty">
        <EmptyState title="No automations yet" body="An automation runs a workspace task file on a schedule or on demand.">
          <template #action><BaseButton icon="plus">Create an automation</BaseButton></template>
        </EmptyState>
      </Panel>
      <Panel title="Error">
        <ErrorState title="Could not load runs" cause="The server did not respond within 10 seconds." next="Check that the proxy is running, then retry.">
          <template #action><BaseButton variant="secondary" icon="refresh">Retry</BaseButton></template>
        </ErrorState>
      </Panel>
    </div>

    <Panel title="Confirmation">
      <BaseButton variant="danger" icon="trash" @click="dialogOpen = true">Delete automation</BaseButton>
      <ConfirmDialog
        v-model="dialogOpen"
        type="warning"
        title="Delete nightly-digest?"
        message="Its schedule stops and its run history is removed. This cannot be undone."
        confirm-text="Delete"
      />
    </Panel>
  </section>
</template>
