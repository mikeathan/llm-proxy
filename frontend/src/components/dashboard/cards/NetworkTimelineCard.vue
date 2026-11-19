<script setup lang="ts">
import TimelineTab from "../../netaudit/TimelineTab.vue"
import { useNetworkAudit } from "../../../composables/netaudit/useNetworkAudit"

// Dashboard card · Timeline: one list of what happened on the network lately (new devices, changed ports, the last audit, the alerts), by
// day. It reads the records the server already keeps and starts nothing; the audit is the one the owner set up, if any.
const { audit } = useNetworkAudit()
</script>

<template>
  <TimelineTab :audit="audit" />
</template>
