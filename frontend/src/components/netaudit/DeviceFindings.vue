<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import StatusTag from "../common/display/StatusTag.vue"
import { formatPorts } from "../../utils/netaudit/expectations"
import { isCheck, isGuessedService, isLowDetail, portCount, serviceName, severityState } from "../../utils/netaudit/friendly"
import { formatAbsoluteTime } from "../../utils/format/time"
import { PLAIN_CARD_CLASS, SEVERITY_CARD_CLASS } from "../../constants/netaudit"
import type { DeviceExpectation, DeviceGroup, ScanFinding } from "../../types/netaudit"

// One device in the Advanced report: a header that always shows the counts, and its ports underneath, folded when the
// device has nothing to look at. Ports the scan could say nothing about (a name taken from the port table, no version)
// fold into one line, because a long run of "http (guess), version unknown" says nothing a count does not. The problems
// themselves are listed once in ProblemGroups; a check row here is only a pointer, kept so an accepted one can be undone.
const props = defineProps<{ group: DeviceGroup<ScanFinding>; open: boolean; baseline: boolean; problems: number; expected?: DeviceExpectation }>()
const emit = defineEmits<{
  (e: "toggle"): void
  (e: "accept", finding: ScanFinding): void
  (e: "revoke", finding: ScanFinding): void
  (e: "rescan", device: string): void
  (e: "expect", device: string): void
  (e: "edit-expected", device: string): void
  (e: "unexpect", device: string): void
}>()

// Folding one port into a line saves nothing; a few do.
const MIN_FOLDED = 2

const showLowDetail = ref(false)
const lowDetail = computed(() => props.group.items.filter(isLowDetail))
const foldLowDetail = computed(() => lowDetail.value.length >= MIN_FOLDED)
const rows = computed(() => (foldLowDetail.value && !showLowDetail.value ? props.group.items.filter((f) => !isLowDetail(f)) : props.group.items))
const foldedPorts = computed(() => lowDetail.value.map((f) => f.port).join(", "))

const portsLabel = (n: number) => `${n} open ${n === 1 ? "port" : "ports"}`
// On the first scan every port is the baseline, so no port is "new" and a row says nothing about its state.
const stateOf = (f: ScanFinding) => (f.accepted ? "accepted" : props.baseline && !isCheck(f) ? "" : f.new ? "new" : f.changed ? "changed" : "unchanged")
const STATE_CLASS: Record<string, string> = {
  new: "bg-accent-info/15 text-accent-info-text",
  changed: "border border-control text-primary",
  accepted: "border border-dashed border-control text-secondary",
  unchanged: "text-muted",
}
</script>

<template>
  <section :aria-label="group.device" class="min-w-0 rounded-[var(--radius-md)] border border-hairline" data-test="device">
    <header class="flex flex-wrap items-center justify-between gap-2 bg-surface-raised px-3 py-2">
      <span class="min-w-0 break-all"><span class="font-mono font-semibold text-primary">{{ group.device }}</span> <span class="text-muted">· {{ portsLabel(portCount(group.items)) }}<template v-if="problems">, {{ problems }} {{ problems === 1 ? "problem" : "problems" }}</template></span></span>
      <span class="flex flex-wrap items-center gap-1.5 text-[length:var(--text-micro)] text-muted">
        <template v-if="!baseline">
          <span v-if="group.items.some((f) => f.new && !f.accepted)">{{ group.items.filter((f) => f.new && !f.accepted).length }} new</span>
          <span v-if="group.items.some((f) => f.changed && !f.accepted)">{{ group.items.filter((f) => f.changed && !f.accepted).length }} changed</span>
        </template>
        <template v-if="expected">
          <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" data-test="edit-expected" @click="emit('edit-expected', group.device)">Edit expected</BaseButton>
          <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" data-test="remove-expected" @click="emit('unexpect', group.device)">Remove expected</BaseButton>
        </template>
        <BaseButton v-else variant="ghost" size="sm" class-name="max-sm:!h-11" data-test="set-expected" @click="emit('expect', group.device)">Set as expected</BaseButton>
        <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" data-test="rescan-host" @click="emit('rescan', group.device)">Rescan this host</BaseButton>
        <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" data-test="toggle-device" :aria-expanded="open" @click="emit('toggle')">{{ open ? "Hide ports" : "Show ports" }}</BaseButton>
      </span>
      <p v-if="expected" class="m-0 basis-full break-words text-[length:var(--text-small)] text-secondary" data-test="expected">Expects {{ formatPorts(expected.ports) }}. Anything else open, or a listed port that does not answer, is reported as a problem.</p>
    </header>
    <ul v-if="open" class="m-0 list-none p-0">
      <li v-for="f in rows" :key="f.key" :data-key="f.key" :class="['flex flex-wrap items-center gap-x-4 gap-y-1 border-t border-hairline px-3 py-2', isCheck(f) ? `border-l-4 ${SEVERITY_CARD_CLASS[f.severity] ?? PLAIN_CARD_CLASS}` : '']">
        <span class="w-20 flex-none font-mono text-[length:var(--text-small)] text-primary">{{ f.port }}/{{ f.proto || "tcp" }}</span>
        <span v-if="isCheck(f)" class="flex min-w-0 flex-1 flex-wrap items-center gap-2 break-words text-[length:var(--text-small)] font-semibold text-primary" data-test="check">
          <StatusTag :state="severityState(f.severity)" :label="`Problem · ${f.severity}`" />
          {{ f.evidence || "A check found a problem" }}
        </span>
        <span v-else class="min-w-0 flex-1 break-words font-mono text-[length:var(--text-small)] text-secondary">
          {{ serviceName(f.service) }}<template v-if="isGuessedService(f.service)"> (guess)</template> ·
          <template v-if="f.changed && f.previous_version"><s class="text-muted">{{ f.previous_version }}</s> </template>{{ f.version || "version unknown" }}
        </span>
        <span v-if="stateOf(f)" :class="['inline-flex min-h-5 items-center px-2 font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)]', STATE_CLASS[stateOf(f)]]">
          <template v-if="f.accepted">Accepted until {{ formatAbsoluteTime(f.accepted_until ?? "") }}<template v-if="f.accepted_note"> · “{{ f.accepted_note }}”</template></template>
          <template v-else>{{ stateOf(f) }}</template>
        </span>
        <BaseButton v-if="f.accepted" variant="secondary" size="sm" class-name="max-sm:!h-11" @click="emit('revoke', f)">Undo</BaseButton>
        <BaseButton v-else variant="secondary" size="sm" class-name="max-sm:!h-11" @click="emit('accept', f)">Accept…</BaseButton>
      </li>
      <li v-if="foldLowDetail" data-test="low-detail" class="flex flex-wrap items-center justify-between gap-2 border-t border-hairline px-3 py-2 text-[length:var(--text-small)] text-muted">
        <span class="min-w-0 break-words">{{ showLowDetail ? "Ports the scan could not identify are listed above." : `${lowDetail.length} ports the scan could not identify: ${foldedPorts}` }}</span>
        <BaseButton variant="ghost" size="sm" class-name="max-sm:!h-11" :aria-expanded="showLowDetail" @click="showLowDetail = !showLowDetail">{{ showLowDetail ? "Fold them" : "Show them" }}</BaseButton>
      </li>
    </ul>
  </section>
</template>
