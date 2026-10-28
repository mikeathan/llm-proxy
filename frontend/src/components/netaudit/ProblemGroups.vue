<script setup lang="ts">
import { ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import StatusTag from "../common/display/StatusTag.vue"
import Disclosure from "../common/layout/Disclosure.vue"
import FixGuidePanel from "./FixGuidePanel.vue"
import FixGuideSteps from "./FixGuideSteps.vue"
import { placeSentence, serviceName, severityState } from "../../utils/netaudit/friendly"
import { flawsOf, nvdUrl } from "../../utils/netaudit/flaws"
import { PLAIN_CARD_CLASS, SEVERITY_CARD_CLASS, SIMPLE_ACCEPT } from "../../constants/netaudit"
import type { ProblemGroup, ScanFinding } from "../../types/netaudit"

// The problems the checks found, one card per kind of problem: what it is and how to fix it written once, then every
// place it was found, each with its own "this is fine" and "I've done this" because those belong to one finding.
// Simple shows the plain line; Advanced adds the software that was compared, the full flaw list and the commands.
defineProps<{ groups: readonly ProblemGroup[]; advanced: boolean }>()
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
const placesLabel = (n: number) => `${n} ${n === 1 ? "place" : "places"}`
// Which findings have their flaw list open; folded by default, since a port can have dozens.
const flawsOpen = ref<Record<string, boolean>>({})
</script>

<template>
  <section v-if="groups.length" aria-label="Problems found by checks" class="flex flex-col gap-3" data-test="problem-groups">
    <article
      v-for="g in groups"
      :key="g.check"
      :data-severity="g.severity"
      :data-check="g.check"
      :class="['flex min-w-0 flex-col gap-2 border-l-4 px-3 py-2.5', SEVERITY_CARD_CLASS[g.severity] ?? PLAIN_CARD_CLASS]"
    >
      <header class="flex flex-wrap items-center gap-2">
        <StatusTag :state="severityState(g.severity)" :label="`Problem · ${g.severity}`" />
        <h3 class="m-0 text-[15px] font-semibold text-primary" data-test="problem-title">{{ g.title }}</h3>
        <span class="text-[length:var(--text-small)] text-muted">{{ placesLabel(g.items.length) }}</span>
      </header>
      <p v-if="g.fix" class="m-0 break-words text-[length:var(--text-small)] text-secondary" data-test="fix"><span class="font-semibold">How to fix it:</span> {{ g.fix }}</p>
      <FixGuideSteps v-if="guideOf(g)" :guide="guideOf(g)!" :advanced="advanced" />
      <ul class="m-0 flex list-none flex-col gap-2 p-0">
        <li v-for="f in g.items" :key="f.key" :data-key="f.key" data-test="problem" class="flex flex-wrap items-center justify-between gap-2 border-t border-hairline pt-2">
          <span class="flex min-w-0 flex-1 flex-col gap-1">
            <span class="break-words font-semibold text-primary" data-test="problem-place">{{ placeSentence(f) }}</span>
            <span class="break-words text-[length:var(--text-small)] text-secondary" data-test="check">{{ f.evidence || "A check found a problem" }}</span>
            <template v-if="advanced && f.check === KNOWN_VULNS">
              <span class="break-words font-mono text-[length:var(--text-small)] text-muted" data-test="vuln-software">Compared: {{ [serviceName(f.service), f.version].filter(Boolean).join(" · ") }}</span>
              <Disclosure v-if="flawsOf(f).length" class="w-full font-normal" data-test="vuln-flaws" :title="`All ${flawsOf(f).length} known flaws`" :open="!!flawsOpen[f.key]" @update:open="flawsOpen[f.key] = $event">
                <ul class="m-0 flex list-none flex-col gap-1 p-0">
                  <li v-for="flaw in flawsOf(f)" :key="flaw.id" data-test="vuln-flaw" class="flex flex-wrap items-center gap-x-3 font-mono text-[length:var(--text-small)] font-normal">
                    <a :href="nvdUrl(flaw.id)" target="_blank" rel="noopener noreferrer" class="text-accent-info-text underline-offset-2 hover:underline">{{ flaw.id }}</a>
                    <span class="text-muted">{{ flaw.score === null ? "no score" : `score ${flaw.score}` }}</span>
                    <span v-if="flaw.exploited" class="text-state-error">exploited</span>
                  </li>
                </ul>
              </Disclosure>
            </template>
            <FixGuidePanel :finding="f" :advanced="advanced" :steps="false" @mark="emit('mark', $event)" @unmark="emit('unmark', $event)" @rescan="emit('rescan', $event)" />
          </span>
          <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" @click="emit('accept', f)">{{ advanced ? ADVANCED_ACCEPT : SIMPLE_ACCEPT }}</BaseButton>
        </li>
      </ul>
    </article>
  </section>
</template>
