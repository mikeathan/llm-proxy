<script setup lang="ts">
import { useRouter } from "vue-router"
import Callout from "../common/feedback/Callout.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { SCAN_TIME_ANCHOR } from "../../constants/netaudit"
import { toSettings } from "../../router/routes"
import { unscannedSentence } from "../../utils/netaudit/friendly"

// A scan that ran out of time on a device knows nothing about its ports, which is not the same as none being open.
// `hasEarlier` is true when there are findings from a scan before, which are kept. The time allowed per device lives
// in the Advanced settings, so the button switches to Advanced and goes straight to it.
defineProps<{ devices: readonly string[]; hasEarlier: boolean }>()

const router = useRouter()
const view = useNetauditViewMode()

function allowMoreTime() {
  view.set("advanced")
  void router.push({ ...toSettings("network-scan"), hash: `#${SCAN_TIME_ANCHOR}` })
}
</script>

<template>
  <Callout v-if="devices.length" tone="warning" :title="unscannedSentence(devices.length)">
    <span class="break-words font-mono">{{ devices.join(", ") }}</span> ran out of time, even after a second try. Their ports are unknown, not "none".<template v-if="hasEarlier"> Their earlier findings are kept.</template>
    <template #actions>
      <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" @click="allowMoreTime">Allow more time</BaseButton>
    </template>
  </Callout>
</template>
