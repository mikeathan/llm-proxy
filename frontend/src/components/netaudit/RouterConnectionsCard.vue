<script setup lang="ts">
import Panel from "../common/layout/Panel.vue"
import TrafficRouterSetup from "./TrafficRouterSetup.vue"
import { ROUTER_PANEL_TITLE } from "../../constants/netaudit"

// Network · Settings · Router connections: how the router is set up to be read, so the Traffic by device card can show where each device talks to.
</script>

<template>
  <Panel :title="ROUTER_PANEL_TITLE">
    <TrafficRouterSetup />
  </Panel>
</template>
