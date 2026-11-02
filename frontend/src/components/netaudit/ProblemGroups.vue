<script setup lang="ts">
import { ref, watch } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import CollapsibleCard from "../common/layout/CollapsibleCard.vue"
import StatusTag from "../common/display/StatusTag.vue"
import Disclosure from "../common/layout/Disclosure.vue"
import FixGuidePanel from "./FixGuidePanel.vue"
import FixGuideSteps from "./FixGuideSteps.vue"
import { placeSentence, serviceName, severityState } from "../../utils/netaudit/friendly"
import { flawsOf, flawSummary, nvdUrl } from "../../utils/netaudit/flaws"
import { formatAbsoluteTime } from "../../utils/format/time"
import { MORE_ACTIONS_LABEL, SIMPLE_ACCEPT, VULN_NOT_PROOF } from "../../constants/netaudit"
import { isHostDevice } from "../../utils/netaudit/friendly"
import type { ProblemGroup, ScanFinding } from "../../types/netaudit"

// The problems the checks found, one card per kind of problem: what it is and how to fix it written once, then every
// place it was found as a row: its name and evidence, one main button ("This is fine" / "Accept…") and a "⋯" that opens the
// rest ("I've done this", a rescan) under that row, one row at a time. Every card is open until the reader folds it; `fold`
// folds or opens them all (the page's Collapse all). Simple shows the plain line; Advanced adds the software that was
// compared, the full flaw list and the commands.
const props = defineProps<{ groups: readonly ProblemGroup[]; advanced: boolean; fold?: { all: boolean; n: number } }>()
const emit = defineEmits<{
  (e: "accept", finding: ScanFinding): void
  (e: "mark", finding: ScanFinding): void
  (e: "unmark", finding: ScanFinding): void
  (e: "rescan", device: string): void
}>()

const KNOWN_VULNS = "known_vulns"
const ADVANCED_ACCEPT = "Accept…"
// Every place of a check carries the same guide (it is keyed by the check), so the group shows the first one it finds.
const guideOf = (g: ProblemGroup) => g.items.find((f) => f.guide)?.guide
// A known-flaws place says only how many flaws it has; the caution that a match is not proof is said once per card.
const placeLine = (f: ScanFinding) => (f.check === KNOWN_VULNS ? flawSummary(f) : "") || f.evidence || "A check found a problem"
const placesLabel = (n: number) => `${n} ${n === 1 ? "place" : "places"}`
// Which findings have their flaw list open; folded by default, since a port can have dozens.
const flawsOpen = ref<Record<string, boolean>>({})
// A card the reader folded by hand; every other card is open. The row whose "⋯" is open is the only one open.
const folded = ref<Record<string, boolean>>({})
const moreOpen = ref("")
const toggleMore = (key: string) => { moreOpen.value = moreOpen.value === key ? "" : key }
watch(() => props.fold?.n, () => { folded.value = Object.fromEntries(props.groups.map((g) => [g.check, props.fold?.all === true])) })
</script>

<template>
  <section v-if="groups.length" aria-label="Problems found by checks" class="flex flex-col gap-3" data-test="problem-groups">
    <CollapsibleCard
      v-for="g in groups"
      :key="g.check"
      :data-severity="g.severity"
      :data-check="g.check"
      :open="!folded[g.check]"
      @update:open="folded[g.check] = !$event"
    >
      <template #title>
        <StatusTag :state="severityState(g.severity)" :label="`Problem · ${g.severity}`" />
        <span class="text-[length:var(--text-heading)] font-semibold text-primary" data-test="problem-title">{{ g.title }}</span>
        <span class="text-[length:var(--text-small)] text-muted">{{ placesLabel(g.items.length) }}</span>
      </template>
      <div class="flex flex-col gap-2 px-4 pb-1 pt-3">
        <p v-if="g.fix" class="m-0 break-words text-secondary" data-test="fix"><span class="text-muted">How to fix it:</span> {{ g.fix }}</p>
        <p v-if="g.check === KNOWN_VULNS" class="m-0 text-[length:var(--text-small)] text-muted" data-test="vuln-caution">{{ VULN_NOT_PROOF }}</p>
        <FixGuideSteps v-if="guideOf(g)" :guide="guideOf(g)!" :advanced="advanced" />
      </div>
      <ul class="m-0 flex list-none flex-col p-0">
        <li v-for="f in g.items" :key="f.key" :data-key="f.key" data-test="problem" class="flex flex-col gap-2 border-t border-hairline px-4 py-3 first:border-t-0">
          <div class="grid grid-cols-[minmax(0,1fr)_auto] items-start gap-x-4 gap-y-1 sm:grid-cols-[minmax(0,1.3fr)_minmax(0,1.7fr)_auto] sm:items-center">
            <span class="min-w-0 break-words font-medium text-primary" data-test="problem-place">{{ placeSentence(f) }}</span>
            <span class="flex min-w-0 flex-col gap-1 break-words text-[length:var(--text-small)] text-secondary max-sm:order-3 max-sm:col-span-2" data-test="check">
              {{ placeLine(f) }}
              <template v-if="advanced && f.check === KNOWN_VULNS">
                <span class="break-words font-mono text-muted" data-test="vuln-software">Compared: {{ [serviceName(f.service), f.version].filter(Boolean).join(" · ") }}</span>
              </template>
            </span>
            <span class="flex flex-none items-center justify-end gap-1.5">
              <BaseButton v-if="!f.deep_at" variant="secondary" size="sm" class-name="max-sm:!h-11" @click="emit('accept', f)">{{ advanced ? ADVANCED_ACCEPT : SIMPLE_ACCEPT }}</BaseButton>
              <BaseButton variant="ghost" size="sm" icon="more" icon-only :label="MORE_ACTIONS_LABEL" class-name="max-sm:!h-11 max-sm:!w-11" data-test="problem-more" :aria-expanded="moreOpen === f.key" @click="toggleMore(f.key)" />
            </span>
          </div>
          <template v-if="advanced && f.check === KNOWN_VULNS && flawsOf(f).length">
            <Disclosure class="w-full font-normal" data-test="vuln-flaws" :title="`All ${flawsOf(f).length} known flaws`" :open="!!flawsOpen[f.key]" @update:open="flawsOpen[f.key] = $event">
              <ul class="m-0 flex list-none flex-col gap-1 p-0">
                <li v-for="flaw in flawsOf(f)" :key="flaw.id" data-test="vuln-flaw" class="flex flex-wrap items-center gap-x-3 font-mono text-[length:var(--text-small)] font-normal">
                  <a :href="nvdUrl(flaw.id)" target="_blank" rel="noopener noreferrer" class="text-accent-info-text underline-offset-2 hover:underline">{{ flaw.id }}</a>
                  <span class="text-muted">{{ flaw.score === null ? "no score" : `score ${flaw.score}` }}</span>
                  <span v-if="flaw.exploited" class="text-state-error">exploited</span>
                </li>
              </ul>
            </Disclosure>
          </template>
          <span v-if="f.deep_at" class="text-[length:var(--text-small)] text-muted" data-test="deep-readonly">From the deep check of {{ formatAbsoluteTime(f.deep_at) }}. You can accept it after the next audit has recorded it.</span>
          <FixGuidePanel v-else :finding="f" :advanced="advanced" :steps="false" :show-mark="moreOpen === f.key" @mark="emit('mark', $event)" @unmark="emit('unmark', $event)" @rescan="emit('rescan', $event)" />
          <div v-if="moreOpen === f.key && !isHostDevice(f.device)" class="flex flex-wrap items-center gap-2" data-test="problem-extras">
            <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" @click="emit('rescan', f.device)">Rescan this host</BaseButton>
          </div>
        </li>
      </ul>
    </CollapsibleCard>
  </section>
</template>
