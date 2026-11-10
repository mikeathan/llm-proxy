<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import StatusTag from "../common/display/StatusTag.vue"
import CollapsibleCard from "../common/layout/CollapsibleCard.vue"
import { formatPorts } from "../../utils/netaudit/expectations"
import { deviceLabel, isCheck, isGuessedService, isLowDetail, isScannable, portCount, portLabel, serviceName, severityState } from "../../utils/netaudit/friendly"
import { formatAbsoluteTime } from "../../utils/format/time"
import { MORE_ACTIONS_LABEL } from "../../constants/netaudit"
import DeepCheckResult from "./DeepCheckResult.vue"
import { cooldownLeftMs, cooldownText } from "../../utils/netaudit/deepCheck"
import type { DeepCheck, DeviceExpectation, DeviceGroup, ScanFinding } from "../../types/netaudit"

// One device in the Advanced report: a header that always shows the counts, and its ports underneath, folded when the
// device has nothing to look at. Ports the scan could say nothing about (a name taken from the port table, no version)
// fold into one line, because a long run of "http (guess), version unknown" says nothing a count does not. The problems
// themselves are listed once in ProblemGroups; a check row here is only a pointer, kept so an accepted one can be undone.
const props = defineProps<{ group: DeviceGroup<ScanFinding>; open: boolean; baseline: boolean; problems: number; expected?: DeviceExpectation; deep?: DeepCheck; deepRunning?: boolean; deepBusy?: boolean }>()
const emit = defineEmits<{
  (e: "toggle"): void
  (e: "accept", finding: ScanFinding): void
  (e: "revoke", finding: ScanFinding): void
  (e: "rescan", device: string): void
  (e: "expect", device: string): void
  (e: "edit-expected", device: string): void
  (e: "unexpect", device: string): void
  (e: "deep", device: string): void
}>()

// A device is checked once in a cool-down, judged from the stored time of its last check (the server enforces it too).
const wait = computed(() => cooldownLeftMs(props.deep?.at, Date.now()))
const deepTitle = computed(() => (wait.value > 0 ? cooldownText(wait.value) : props.deepBusy ? "Another device is being checked." : ""))

// Folding one port into a line saves nothing; a few do.
const MIN_FOLDED = 2

// Every action on a device (rescan, expected ports, deep check) sits behind one "⋯", so a header of many devices stays readable.
const more = ref(false)
// The computer the program runs on has no network address to scan, expect ports on or deep check: it shows its problems only.
const isHost = computed(() => !isScannable(props.group.device))
const showLowDetail = ref(false)
const lowDetail = computed(() => props.group.items.filter(isLowDetail))
const foldLowDetail = computed(() => lowDetail.value.length >= MIN_FOLDED)
const rows = computed(() => (foldLowDetail.value && !showLowDetail.value ? props.group.items.filter((f) => !isLowDetail(f)) : props.group.items))
const foldedPorts = computed(() => lowDetail.value.map((f) => f.port).join(", "))

const portsLabel = (n: number) => `${n} open ${n === 1 ? "port" : "ports"}`
// On the first scan every port is the baseline, so no port is "new" and a row says nothing about its state.
const stateOf = (f: ScanFinding) => (f.accepted ? "accepted" : props.baseline && !isCheck(f) ? "" : f.new ? "new" : f.changed ? "changed" : "unchanged")
// Only a state worth a look is drawn; "unchanged" is the usual case and says nothing.
const STATE_TONE: Record<string, "info" | "warning" | "neutral"> = { new: "info", changed: "warning", accepted: "neutral" }
</script>

<template>
  <CollapsibleCard :open="open" :label="group.device" data-test="device" @update:open="emit('toggle')">
    <template #title>
      <span class="min-w-0 break-all font-mono text-[length:var(--text-heading)] text-primary">{{ deviceLabel(group.device) }}</span>
      <template v-if="!isHost">
        <span class="inline-flex h-5 items-center rounded-[var(--radius-sm)] border border-hairline px-1.5 text-[length:var(--text-micro)] text-secondary">{{ portsLabel(portCount(group.items)) }}</span>
        <StatusTag v-if="problems" state="warning" :label="`${problems} ${problems === 1 ? 'problem' : 'problems'}`" />
      </template>
      <template v-if="!baseline">
        <StatusTag v-if="group.items.some((f) => f.new && !f.accepted)" state="info" :label="`${group.items.filter((f) => f.new && !f.accepted).length} new`" />
        <StatusTag v-if="group.items.some((f) => f.changed && !f.accepted)" state="warning" :label="`${group.items.filter((f) => f.changed && !f.accepted).length} changed`" />
      </template>
    </template>
    <template v-if="!isHost" #aside>
      <BaseButton variant="ghost" size="sm" icon="more" icon-only :label="MORE_ACTIONS_LABEL" class-name="max-sm:!h-11 max-sm:!w-11" data-test="more-actions" :aria-expanded="more" @click="more = !more" />
    </template>

    <div v-if="more && !isHost" data-test="device-more" class="flex flex-wrap items-center gap-2 border-b border-hairline bg-surface-raised px-4 py-2.5">
      <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" data-test="rescan-host" @click="emit('rescan', group.device)">Rescan this host</BaseButton>
      <template v-if="expected">
        <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" data-test="edit-expected" @click="emit('edit-expected', group.device)">Edit expected</BaseButton>
        <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" data-test="remove-expected" @click="emit('unexpect', group.device)">Remove expected</BaseButton>
      </template>
      <BaseButton v-else variant="secondary" size="sm" class-name="max-sm:!h-11" data-test="set-expected" @click="emit('expect', group.device)">Set as expected</BaseButton>
      <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" :loading="deepRunning" :disabled="deepBusy || wait > 0 || portCount(group.items) === 0" :title="deepTitle" data-test="deep-check" @click="emit('deep', group.device)">{{ deepRunning ? "Checking…" : "Deep check" }}</BaseButton>
    </div>
    <p v-if="expected" class="m-0 break-words border-b border-hairline px-4 py-2.5 text-[length:var(--text-small)] text-secondary" data-test="expected">Expects {{ formatPorts(expected.ports) }}. Anything else open, or a listed port that does not answer, is reported as a problem.</p>
    <DeepCheckResult v-if="deep" :check="deep" />
    <p v-if="deepRunning" role="status" class="m-0 border-b border-hairline px-4 py-2.5 text-[length:var(--text-small)] text-secondary" data-test="deep-running">Checking {{ group.device }} slowly. This can take a few minutes; the page stays usable.</p>
    <ul class="m-0 list-none p-0">
      <li v-for="f in rows" :key="f.key" :data-key="f.key" class="grid grid-cols-[4.5rem_minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 border-t border-hairline px-4 py-2.5 first:border-t-0 sm:grid-cols-[6rem_minmax(0,1fr)_auto_auto]">
        <span class="font-mono text-[length:var(--text-small)] text-primary">{{ portLabel(f) }}</span>
        <span v-if="isCheck(f)" class="flex min-w-0 flex-wrap items-center gap-2 break-words text-[length:var(--text-small)] text-primary" data-test="check">
          <StatusTag :state="f.accepted ? 'neutral' : severityState(f.severity)" :label="`Problem · ${f.severity}`" />
          {{ f.evidence || "A check found a problem" }}
        </span>
        <span v-else class="min-w-0 break-words text-[length:var(--text-small)] text-secondary">
          <span class="text-primary">{{ serviceName(f.service) }}</span><template v-if="isGuessedService(f.service)"> (guess)</template>
          <span class="block text-muted sm:ml-2 sm:inline"><template v-if="f.changed && f.previous_version"><s>{{ f.previous_version }}</s> </template>{{ f.version || "version unknown" }}</span>
        </span>
        <span v-if="stateOf(f) && stateOf(f) !== 'unchanged'" class="max-sm:col-start-2 max-sm:row-start-2 max-sm:col-span-2">
          <StatusTag :state="STATE_TONE[stateOf(f)] ?? 'neutral'" :label="stateOf(f)" />
          <span v-if="f.accepted" class="ml-2 text-[length:var(--text-small)] text-muted">until {{ formatAbsoluteTime(f.accepted_until ?? "") }}<template v-if="f.accepted_note"> · “{{ f.accepted_note }}”</template></span>
        </span>
        <span v-else class="max-sm:hidden"></span>
        <span class="flex justify-end max-sm:col-start-3 max-sm:row-start-1">
          <BaseButton v-if="f.accepted" variant="secondary" size="sm" class-name="max-sm:!h-11" @click="emit('revoke', f)">Undo</BaseButton>
          <BaseButton v-else variant="secondary" size="sm" class-name="max-sm:!h-11" @click="emit('accept', f)">Accept…</BaseButton>
        </span>
      </li>
      <li v-if="foldLowDetail" data-test="low-detail" class="flex flex-wrap items-center justify-between gap-2 border-t border-hairline px-4 py-2.5 text-[length:var(--text-small)] text-muted">
        <span class="min-w-0 break-words">{{ showLowDetail ? "Ports the scan could not identify are listed above." : `${lowDetail.length} ports the scan could not identify: ${foldedPorts}` }}</span>
        <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" :aria-expanded="showLowDetail" @click="showLowDetail = !showLowDetail">{{ showLowDetail ? "Fold them" : "Show them" }}</BaseButton>
      </li>
    </ul>
  </CollapsibleCard>
</template>
