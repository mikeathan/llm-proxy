<script setup lang="ts">
import { ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Callout from "../common/feedback/Callout.vue"
import FixGuideSteps from "./FixGuideSteps.vue"
import { NEARBY_ADVERTISES_AS, NEARBY_FIX, NEARBY_NOT_LISTED } from "../../constants/netaudit"
import { nearbyKindLabel, nearbyName, plain, serviceLabel } from "../../utils/netaudit/nearby"
import { formatRelativeTime } from "../../utils/format/time"
import type { NearbyDevice } from "../../types/netaudit"

// One device's details: what it said about itself, plainly, and the fixed sentence and guide for a problem. Every text the
// device sent is shown with {{ }} only, so it is text and never markup. Used in the side panel and in the phone's sheet.
const props = defineProps<{ device: NearbyDevice; advanced: boolean }>()
const guideOpen = ref(false)

const facts = () => [
  ["Address", props.device.ip],
  ["Hardware", props.device.mac || NEARBY_NOT_LISTED],
  ["Heard by", (props.device.sources ?? []).map((s) => (s === "mdns" ? "mDNS" : "UPnP")).join(" and ") || "—"],
  ["Last heard", formatRelativeTime(props.device.last_heard)],
  ["Maker", plain(props.device.maker) || "—"],
  ["Model", plain(props.device.model) || "—"],
  ["Services", (props.device.services ?? []).map(serviceLabel).join(", ") || "—"],
]
</script>

<template>
  <div class="flex min-w-0 flex-col gap-2" data-test="nearby-detail">
    <p class="m-0 break-words text-primary font-semibold">{{ nearbyName(device) }}</p>
    <p class="m-0 text-[length:var(--text-small)] text-muted">{{ NEARBY_ADVERTISES_AS }} {{ nearbyKindLabel(device) }}</p>
    <Callout v-if="device.problem" tone="error" :title="device.problem.title" data-test="nearby-problem">
      {{ device.problem.text }}
      <template v-if="device.problem.guide" #actions>
        <BaseButton variant="secondary" class-name="max-sm:!h-11" data-test="nearby-fix" :aria-expanded="guideOpen" @click="guideOpen = !guideOpen">{{ NEARBY_FIX }}</BaseButton>
      </template>
    </Callout>
    <FixGuideSteps v-if="device.problem?.guide && guideOpen" :guide="device.problem.guide" :advanced="advanced" />
    <dl class="m-0 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 text-[length:var(--text-small)]">
      <template v-for="[label, value] in facts()" :key="label">
        <dt class="text-muted">{{ label }}</dt>
        <dd class="m-0 break-words text-secondary">{{ value }}</dd>
      </template>
    </dl>
  </div>
</template>
