<script setup lang="ts">
import { ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import TrafficLookups from "./TrafficLookups.vue"
import TrafficRouterConn from "./TrafficRouterConn.vue"
import { OTHER_DEVICES_TITLE, OTHER_DEVICES_VIEW_LABELS, OTHER_DEVICES_VIEWS_LABEL } from "../../constants/netaudit"
import type { ChoiceOption } from "../../types/ui"

// The other devices on this network, in one place with two views: what they looked up (the router's DNS log, Constitution I.8) and where they
// connected (the router's connection table, Constitution I.15). The two are different sources with different setup, so they are one choice here
// and not two panels.
type View = "lookups" | "connections"
const view = ref<View>("lookups")
const views: ChoiceOption[] = (["lookups", "connections"] as const).map((value) => ({ value, label: OTHER_DEVICES_VIEW_LABELS[value] }))
</script>

<template>
  <Panel :title="OTHER_DEVICES_TITLE">
    <template #actions>
      <SegmentedControl v-model="view" :options="views" :label="OTHER_DEVICES_VIEWS_LABEL" variant="compact" data-test="other-devices-views" />
    </template>
    <TrafficLookups v-if="view === 'lookups'" />
    <TrafficRouterConn v-else />
  </Panel>
</template>
