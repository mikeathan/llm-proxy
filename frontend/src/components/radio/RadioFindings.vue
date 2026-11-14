<script setup lang="ts">
import { computed, onMounted } from "vue"
import Panel from "../common/layout/Panel.vue"
import Callout from "../common/feedback/Callout.vue"
import { useRadioDigest } from "../../composables/radio/useRadioDigest"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import {
  RADIO_FINDINGS_NONE, RADIO_FINDINGS_THIN, RADIO_FINDINGS_TITLE, RADIO_TIER_HELP, RADIO_TIER_WORDS, RADIO_UNSEEN_TITLE, radioFindingCount, radioUnseenLine,
} from "../../constants/radio"

// Radio · Things to look at: the stack's findings. The sentences are the server's fixed words (never text a device sent); each says how it is known.
// A check that cannot run yet is said out loud, so a quiet list is not mistaken for a clean one.
const d = useRadioDigest()
const { advanced } = useNetauditViewMode()
const view = computed(() => d.digest.value)
const thin = computed(() => !!view.value && view.value.listening.cycles < view.value.listening.min_cycles)

onMounted(() => { void d.load() })
</script>

<template>
  <Panel :title="RADIO_FINDINGS_TITLE">
    <div class="flex flex-col gap-3" data-test="radio-findings">
      <Callout v-if="d.error.value && !view" tone="error" title="Could not read"><span data-test="radio-findings-error">{{ d.error.value }}</span></Callout>
      <template v-else-if="view">
        <ul v-if="view.findings.length" class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
          <li v-for="f in view.findings" :key="f.id" class="flex flex-col gap-1 py-2" data-test="radio-finding" :data-id="f.id">
            <span class="text-primary">{{ f.claim }}</span>
            <span class="flex flex-wrap items-baseline gap-x-3 text-[length:var(--text-small)] text-muted">
              <span class="text-secondary" data-test="radio-finding-tier" :title="RADIO_TIER_HELP[f.tier]">{{ RADIO_TIER_WORDS[f.tier] }}</span>
              <span data-test="radio-finding-count">{{ radioFindingCount(f.count) }}</span>
            </span>
          </li>
        </ul>
        <p v-else-if="thin" class="m-0 text-secondary" data-test="radio-findings-thin">{{ RADIO_FINDINGS_THIN }}</p>
        <p v-else class="m-0 text-secondary" data-test="radio-findings-none">{{ RADIO_FINDINGS_NONE }}</p>
        <p v-if="view.could_not_see.length" class="m-0 text-[length:var(--text-small)] text-muted" data-test="radio-unseen">{{ radioUnseenLine(view.could_not_see.length) }}</p>
        <div v-if="advanced && view.could_not_see.length" class="flex flex-col gap-1">
          <h3 class="m-0 text-[length:var(--text-small)] font-medium text-secondary">{{ RADIO_UNSEEN_TITLE }}</h3>
          <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
            <li v-for="u in view.could_not_see" :key="u.id" class="py-1.5 text-[length:var(--text-small)] text-muted" data-test="radio-unseen-reason">{{ u.reason }}</li>
          </ul>
        </div>
      </template>
    </div>
  </Panel>
</template>
