<script setup lang="ts">
import { computed } from "vue"
import StatusTag from "../common/display/StatusTag.vue"
import FixGuideSteps from "./FixGuideSteps.vue"
import { deepOutcome } from "../../utils/netaudit/deepCheck"
import { severityState } from "../../utils/netaudit/friendly"
import { formatAbsoluteTime } from "../../utils/format/time"
import type { DeepCheck } from "../../types/netaudit"
import type { StatusState } from "../../types/ui"

// The last deep check of a device: when, what answered and what it found. "Nothing to check" is its own answer, never worded
// as fine. Each problem carries its fixed-text fix and guide, like the problems of an audit.
const props = defineProps<{ check: DeepCheck }>()
const outcome = computed(() => deepOutcome(props.check))
const TAG: Record<string, { state: StatusState; label: string }> = {
  problems: { state: "error", label: "Problems" },
  clean: { state: "success", label: "Nothing wrong" },
  nothing: { state: "neutral", label: "Nothing to check" },
}
</script>

<template>
  <div class="flex flex-col gap-2 border-t border-hairline bg-surface px-3 py-2" data-test="deep-result">
    <p class="m-0 flex flex-wrap items-center gap-2 text-[length:var(--text-small)] text-secondary">
      <StatusTag :state="TAG[outcome.state]!.state" :label="TAG[outcome.state]!.label" />
      <span><span class="font-semibold text-primary">Deep check</span>, {{ formatAbsoluteTime(check.at) }}. {{ outcome.text }}</span>
    </p>
    <ul v-if="check.findings.length" class="m-0 flex list-none flex-col gap-2 p-0">
      <li v-for="f in check.findings" :key="`${f.port}-${f.check}`" class="flex flex-col gap-1 border-l-4 border-hairline pl-3" data-test="deep-finding">
        <span class="flex flex-wrap items-center gap-2 text-[length:var(--text-small)] font-semibold text-primary">
          <StatusTag :state="severityState(f.severity)" :label="`Problem · ${f.severity}`" />
          {{ f.check_title || f.check }} · {{ f.port }}/{{ f.proto || "tcp" }}
        </span>
        <span class="text-[length:var(--text-small)] text-secondary">{{ f.evidence }}</span>
        <span v-if="f.fix" class="text-[length:var(--text-small)] text-muted">{{ f.fix }}</span>
        <FixGuideSteps v-if="f.guide" :guide="f.guide" advanced />
      </li>
    </ul>
  </div>
</template>
