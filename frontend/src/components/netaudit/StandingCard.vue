<script setup lang="ts">
import { computed } from "vue"
import Panel from "../common/layout/Panel.vue"
import { ageSentence, trendSentence } from "../../utils/netaudit/standing"
import { formatAbsoluteTime } from "../../utils/format/time"
import { STANDING_NO_HISTORY, STANDING_SOURCE, STANDING_TITLE } from "../../constants/netaudit"
import type { NetworkStanding } from "../../types/netaudit"

// The Overview's answer to "are we getting better?": how long the open problems have lasted and how the last week compared with the one
// before. Plain counts from the finding history, each with the sentence saying where it comes from; no single score.
const props = defineProps<{ standing: NetworkStanding }>()
const arrow = computed(() => ({ better: "↓", worse: "↑", same: "→", unknown: "" })[props.standing.trend])
</script>

<template>
  <Panel :title="STANDING_TITLE">
    <div class="flex flex-col gap-2" data-test="standing">
      <p v-if="!standing.history_since" class="m-0 text-muted" data-test="standing-none">{{ STANDING_NO_HISTORY }}</p>
      <template v-else>
        <p class="m-0 text-primary" data-test="standing-age">{{ ageSentence(standing) }}</p>
        <p class="m-0 text-primary" data-test="standing-trend"><span v-if="arrow" aria-hidden="true" class="mr-1.5 font-mono">{{ arrow }}</span>{{ trendSentence(standing) }}</p>
        <p class="m-0 text-[length:var(--text-small)] text-faint" data-test="standing-source">{{ STANDING_SOURCE }} {{ formatAbsoluteTime(standing.history_since) }}.</p>
      </template>
    </div>
  </Panel>
</template>
