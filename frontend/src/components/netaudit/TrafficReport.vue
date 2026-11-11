<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue"
import Panel from "../common/layout/Panel.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import TrafficDetail from "./TrafficDetail.vue"
import TrafficFlows from "./TrafficFlows.vue"
import TrafficKinds from "./TrafficKinds.vue"
import TrafficTalk from "./TrafficTalk.vue"
import TrafficWhere from "./TrafficWhere.vue"
import { useDeviceNames } from "../../composables/netaudit/useDeviceNames"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import { talkGroups, trafficPlace } from "../../utils/netaudit/traffic"
import { motionScroll } from "../../utils/motion"
import {
  TRAFFIC_ADVANCED_TITLE, TRAFFIC_CONVERSATIONS_TITLE, TRAFFIC_PRIVACY_NOTE, TRAFFIC_TALK_TITLE, TRAFFIC_WHAT_INTRO, TRAFFIC_WHAT_TITLE, TRAFFIC_WHERE_VIEW_LABELS, TRAFFIC_WHERE_VIEWS_LABEL,
} from "../../constants/netaudit"
import type { ChoiceOption } from "../../types/ui"
import type { HostTraffic, TrafficWhereView } from "../../types/netaudit"

// The rest of what one look at this computer saw, under its page: the devices on this network and the broadcast chatter it talked to (the internet
// places are the company groups of the page above), what the traffic was, and, under Advanced, the operator and country lists and the conversation
// table. A place opens its detail: who runs it, where it is used, and the route to it. Names come from DNS answers and TLS handshakes and are shown
// as text only. Nothing here is a finding.
const props = defineProps<{ traffic: HostTraffic }>()
const { advanced } = useNetauditViewMode()

// What the person calls each device on this network, so a row can say "Pixel 8" for 192.168.50.10. A failed read leaves addresses alone.
const { names, load } = useDeviceNames()
onMounted(load)
const groups = computed(() => talkGroups(props.traffic, names.value))
const talkTotal = computed(() => groups.value.flatMap((g) => g.rows).reduce((n, r) => n + r.bytes, 0))
// The internet is drawn as company groups with the map; this list is who else it talked to.
const nearby = computed(() => groups.value.filter((g) => g.id !== "internet"))

// What is open belongs to one look: a new look starts clean.
const openAddress = ref<string | null>(null)
watch(() => props.traffic, () => { openAddress.value = null })
const place = computed(() => (openAddress.value ? trafficPlace(props.traffic, openAddress.value) : null))
const detailEl = ref<HTMLElement | null>(null)
async function openPlace(address: string) {
  openAddress.value = address
  await nextTick()
  detailEl.value?.scrollIntoView?.({ behavior: motionScroll("smooth"), block: "start" })
}

// Advanced: the same internet traffic by operator or by country.
const view = ref<TrafficWhereView>("operators")
const views: ChoiceOption[] = (["operators", "countries"] as const).map((value) => ({ value, label: TRAFFIC_WHERE_VIEW_LABELS[value] }))
</script>

<template>
  <div class="flex flex-col gap-4" data-test="traffic-report">
    <div v-if="place" ref="detailEl" class="scroll-mt-4">
      <TrafficDetail :place="place" @close="openAddress = null" />
    </div>

    <Panel v-if="nearby.some((g) => g.rows.length)" :title="TRAFFIC_TALK_TITLE">
      <TrafficTalk :groups="nearby" :total="talkTotal" :open-address="openAddress" :other-flows="traffic.other_flows" :other-bytes="traffic.other_bytes" @open="openPlace" />
    </Panel>

    <Panel v-if="traffic.kinds.length" :title="TRAFFIC_WHAT_TITLE">
      <div class="flex flex-col gap-3">
        <TrafficKinds :traffic="traffic" />
        <p v-if="!advanced" class="m-0 text-[length:var(--text-small)] text-muted" data-test="kinds-intro">{{ TRAFFIC_WHAT_INTRO }}</p>
      </div>
    </Panel>

    <template v-if="advanced">
      <Panel v-if="traffic.internet_bytes > 0" :title="TRAFFIC_ADVANCED_TITLE">
        <template #actions>
          <SegmentedControl v-model="view" :options="views" :label="TRAFFIC_WHERE_VIEWS_LABEL" variant="compact" data-test="where-views" />
        </template>
        <TrafficWhere :traffic="traffic" :view="view" />
      </Panel>
      <Panel v-if="traffic.flows.length" :title="TRAFFIC_CONVERSATIONS_TITLE">
        <TrafficFlows :traffic="traffic" @open="openPlace" />
      </Panel>
    </template>

    <p class="m-0 text-[length:var(--text-small)] text-muted" data-test="traffic-privacy">{{ TRAFFIC_PRIVACY_NOTE }}</p>
  </div>
</template>
