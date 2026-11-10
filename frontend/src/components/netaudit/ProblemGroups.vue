<script setup lang="ts">
import { computed, ref, watch } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import CollapsibleCard from "../common/layout/CollapsibleCard.vue"
import StatusTag from "../common/display/StatusTag.vue"
import Disclosure from "../common/layout/Disclosure.vue"
import FixGuidePanel from "./FixGuidePanel.vue"
import FixGuideSteps from "./FixGuideSteps.vue"
import FoundBy from "./FoundBy.vue"
import { placeSentence, serviceName, severityState } from "../../utils/netaudit/friendly"
import { flawsOf, flawSummary, nvdUrl } from "../../utils/netaudit/flaws"
import { formatAbsoluteTime } from "../../utils/format/time"
import { MORE_ACTIONS_LABEL, SIMPLE_ACCEPT, VULN_NOT_PROOF } from "../../constants/netaudit"
import { isScannable } from "../../utils/netaudit/friendly"
import { sharedEvidence } from "../../utils/netaudit/report"
import type { ProblemGroup, ScanFinding } from "../../types/netaudit"

// The problems the checks found, one card per kind of problem: what it is and how to fix it written once, then every
// place it was found as a row: its name and evidence, one main button ("This is fine" / "Accept…") and a "⋯" that opens the
// rest ("I've done this", a rescan) under that row, one row at a time. A short list (FOLD_ABOVE cards or fewer) is open; a
// longer one starts folded to its titles, so the reader sees every kind of problem on one screen and opens the one to act on
// (vertex, 2026-11-05: nine open cards made a 5,000 px page on a phone). `fold` folds or opens them all (the page's Collapse
// all). The fix is said once: as the guide's steps when there is a guide, as a line when there is not. Simple shows the plain
// line; Advanced adds the software that was compared, the full flaw list and the commands.
// `names` maps a device address to what the person calls it (or its maker), shown before the place. `notes` is the quiet list: every
// card starts folded, however short the list, because a note needs no reading until the reader chooses to. A kind with several places
// says the line they all share once, and offers one button to accept every place.
const props = defineProps<{ groups: readonly ProblemGroup[]; advanced: boolean; fold?: { all: boolean; n: number }; names?: Readonly<Record<string, string>>; notes?: boolean }>()
const emit = defineEmits<{
  (e: "accept", finding: ScanFinding): void
  (e: "accept-many", findings: ScanFinding[]): void
  (e: "mark", finding: ScanFinding): void
  (e: "unmark", finding: ScanFinding): void
  (e: "rescan", device: string): void
}>()

const KNOWN_VULNS = "known_vulns"
// FOLD_ABOVE is the most cards that start open.
const FOLD_ABOVE = 2
const ADVANCED_ACCEPT = "Accept…"
const ACCEPT_ALL = "This is fine for all"
const NAME_SEPARATOR = " · "
// Every place of a check carries the same guide (it is keyed by the check), so the group shows the first one it finds.
const guideOf = (g: ProblemGroup) => g.items.find((f) => f.guide)?.guide
// A known-flaws place says only how many flaws it has; the caution that a match is not proof is said once per card.
const placeLine = (f: ScanFinding) => (f.check === KNOWN_VULNS ? flawSummary(f) : "") || f.evidence || "A check found a problem"
const placesLabel = (n: number) => `${n} ${n === 1 ? "place" : "places"}`
// Which findings have their flaw list open; folded by default, since a port can have dozens.
const flawsOpen = ref<Record<string, boolean>>({})
// A card the reader opened or folded by hand; every other card follows the list's length. The row whose "⋯" is open is the only
// one open.
const toggled = ref<Record<string, boolean>>({})
const startOpen = computed(() => !props.notes && props.groups.length <= FOLD_ABOVE)
const placeLabel = (f: ScanFinding) => [props.names?.[f.device], placeSentence(f)].filter(Boolean).join(NAME_SEPARATOR)
// The line every place shares is said once. A known-flaws place says its own count instead, so it never shares one.
const sharedOf = (g: ProblemGroup) => (g.check === KNOWN_VULNS ? "" : sharedEvidence(g.items))
const canAcceptAll = (g: ProblemGroup) => g.items.length > 1 && g.items.every((f) => !f.deep_at)
const isOpen = (check: string) => toggled.value[check] ?? startOpen.value
const moreOpen = ref("")
const toggleMore = (key: string) => { moreOpen.value = moreOpen.value === key ? "" : key }
watch(() => props.fold?.n, () => { toggled.value = Object.fromEntries(props.groups.map((g) => [g.check, props.fold?.all !== true])) })
</script>

<template>
  <section v-if="groups.length" aria-label="Problems found by checks" class="flex flex-col gap-3" data-test="problem-groups">
    <CollapsibleCard
      v-for="g in groups"
      :key="g.check"
      :data-severity="g.severity"
      :data-check="g.check"
      :open="isOpen(g.check)"
      @update:open="toggled[g.check] = $event"
    >
      <template #title>
        <StatusTag :state="severityState(g.severity)" :label="g.severity" />
        <span class="min-w-0 break-words font-semibold text-primary" data-test="problem-title">{{ g.title }}</span>
        <span v-if="g.items.length > 1" class="text-[length:var(--text-small)] text-muted">{{ placesLabel(g.items.length) }}</span>
      </template>
      <div class="flex flex-col gap-2 px-4 pb-1 pt-3">
        <FoundBy :check="g.check" />
        <p v-if="g.fix && !guideOf(g)" class="m-0 break-words text-secondary" data-test="fix"><span class="text-muted">How to fix it:</span> {{ g.fix }}</p>
        <p v-if="g.check === KNOWN_VULNS" class="m-0 text-[length:var(--text-small)] text-muted" data-test="vuln-caution">{{ VULN_NOT_PROOF }}</p>
        <p v-if="sharedOf(g)" class="m-0 break-words text-secondary" data-test="shared-evidence">{{ sharedOf(g) }}</p>
        <FixGuideSteps v-if="guideOf(g)" :guide="guideOf(g)!" :advanced="advanced" />
        <div v-if="canAcceptAll(g)" class="flex">
          <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" data-test="accept-all" @click="emit('accept-many', g.items)">{{ ACCEPT_ALL }} {{ g.items.length }}</BaseButton>
        </div>
      </div>
      <ul class="m-0 flex list-none flex-col p-0">
        <li v-for="f in g.items" :key="f.key" :data-key="f.key" data-test="problem" class="flex flex-col gap-2 border-t border-hairline px-4 py-3 first:border-t-0">
          <div class="grid grid-cols-1 gap-y-1 sm:grid-cols-[minmax(0,1.3fr)_minmax(0,1.7fr)_auto] sm:items-center sm:gap-x-4">
            <span class="min-w-0 break-words font-medium text-primary" data-test="problem-place">{{ placeLabel(f) }}</span>
            <span class="flex min-w-0 flex-col gap-1 break-words text-[length:var(--text-small)] text-secondary" data-test="check">
              <template v-if="!sharedOf(g)">{{ placeLine(f) }}</template>
              <template v-if="advanced && f.check === KNOWN_VULNS">
                <span class="break-words font-mono text-muted" data-test="vuln-software">Compared: {{ [serviceName(f.service), f.version].filter(Boolean).join(" · ") }}</span>
              </template>
            </span>
            <span class="flex flex-none items-center gap-1.5 max-sm:-ml-2 sm:justify-end">
              <BaseButton v-if="!f.deep_at" variant="ghost" size="sm" class-name="max-sm:!h-11" @click="emit('accept', f)">{{ advanced ? ADVANCED_ACCEPT : SIMPLE_ACCEPT }}</BaseButton>
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
          <div v-if="moreOpen === f.key && isScannable(f.device)" class="flex flex-wrap items-center gap-2" data-test="problem-extras">
            <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" @click="emit('rescan', f.device)">Rescan this host</BaseButton>
          </div>
        </li>
      </ul>
    </CollapsibleCard>
  </section>
</template>
