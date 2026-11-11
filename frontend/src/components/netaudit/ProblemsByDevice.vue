<script setup lang="ts">
import { ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import StatusTag from "../common/display/StatusTag.vue"
import Disclosure from "../common/layout/Disclosure.vue"
import FixGuideSteps from "./FixGuideSteps.vue"
import { deviceLabel, severityState } from "../../utils/netaudit/friendly"
import { SIMPLE_ACCEPT } from "../../constants/netaudit"
import type { DeviceProblems, ScanFinding } from "../../types/netaudit"

// The problems laid out by device: one card per device with everything wrong with it, the device with the worst problem first. The same
// findings as the by-kind layout, so nothing is lost by switching. `names` maps an address to what the person calls the device.
const props = defineProps<{ devices: readonly DeviceProblems[]; advanced: boolean; names?: Readonly<Record<string, string>> }>()
const emit = defineEmits<{
  (e: "accept", finding: ScanFinding): void
  (e: "accept-many", findings: ScanFinding[]): void
}>()

const ACCEPT_ALL = "This is fine for all"
const NAME_SEPARATOR = " · "
const FALLBACK_TITLE = "A check found a problem"
const nameOf = (device: string) => [props.names?.[device], deviceLabel(device)].filter(Boolean).join(NAME_SEPARATOR)
const titleOf = (f: ScanFinding) => f.check_title || f.evidence || FALLBACK_TITLE
const canAcceptAll = (d: DeviceProblems) => d.items.length > 1 && d.items.every((f) => !f.deep_at)
const fixOpen = ref<Record<string, boolean>>({})
</script>

<template>
  <section v-if="devices.length" aria-label="Problems by device" class="flex flex-col gap-3" data-test="problems-by-device">
    <article
      v-for="d in devices"
      :key="d.device"
      :data-device="d.device"
      data-test="device-card"
      class="flex flex-col gap-2 rounded-[var(--radius-md)] border border-hairline bg-surface px-4 py-3"
    >
      <header class="flex flex-wrap items-center gap-x-3 gap-y-1">
        <StatusTag :state="severityState(d.severity)" :label="d.severity" />
        <h3 class="m-0 min-w-0 break-words text-[length:var(--text-body)] font-semibold text-primary" data-test="device-name">{{ nameOf(d.device) }}</h3>
        <span class="text-[length:var(--text-small)] text-muted">{{ d.items.length }} {{ d.items.length === 1 ? "problem" : "problems" }}</span>
        <span class="flex-1"></span>
        <BaseButton v-if="canAcceptAll(d)" variant="secondary" size="sm" class-name="max-sm:!h-11" data-test="accept-all" @click="emit('accept-many', d.items)">{{ ACCEPT_ALL }} {{ d.items.length }}</BaseButton>
      </header>
      <ul class="m-0 flex list-none flex-col p-0">
        <li v-for="f in d.items" :key="f.key" :data-key="f.key" data-test="device-problem" class="flex flex-col gap-1 border-t border-hairline py-2 first:border-t-0">
          <div class="grid grid-cols-1 gap-y-1 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center sm:gap-x-4">
            <span class="flex min-w-0 flex-col gap-0.5">
              <span class="break-words font-medium text-primary" data-test="device-problem-title">{{ titleOf(f) }}</span>
              <span v-if="f.evidence" class="break-words text-[length:var(--text-small)] text-secondary">{{ f.evidence }}</span>
            </span>
            <BaseButton v-if="!f.deep_at" variant="ghost" size="sm" class-name="max-sm:!h-11 max-sm:-ml-2" data-test="accept" @click="emit('accept', f)">{{ SIMPLE_ACCEPT }}</BaseButton>
          </div>
          <FixGuideSteps v-if="f.guide" :guide="f.guide" :advanced="advanced" />
          <Disclosure v-else-if="f.fix" title="How to fix it" :open="!!fixOpen[f.key]" class="font-normal" @update:open="fixOpen[f.key] = $event">
            <p class="m-0 break-words text-[length:var(--text-small)] text-secondary" data-test="fix">{{ f.fix }}</p>
          </Disclosure>
        </li>
      </ul>
    </article>
  </section>
</template>
