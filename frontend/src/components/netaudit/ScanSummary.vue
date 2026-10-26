<script setup lang="ts">
import StatusTag from "../common/display/StatusTag.vue"
import { toSettings } from "../../router/routes"
import type { ScanSettings } from "../../types/netaudit"

// What an audit scans, as tags that are quick to read: how hard, whether the safe checks run, the ports when it is not
// the common set, and the networks. With no networks named it says "every allowed network" and links to where those
// are chosen.
defineProps<{ scan?: ScanSettings }>()

const TAG_CLASS = "inline-flex h-5 items-center border border-hairline px-[7px] font-mono text-[length:var(--text-micro)] text-secondary"
</script>

<template>
  <span class="flex flex-wrap items-center gap-1.5">
    <StatusTag data-test="scan-tag" state="info" :label="scan?.depth === 'quick' ? 'Quick scan' : 'Thorough scan'" />
    <StatusTag data-test="scan-tag" :state="scan?.checks === false ? 'neutral' : 'success'" :label="scan?.checks === false ? 'Checks off' : 'Safe checks on'" />
    <span v-if="scan?.ports" data-test="scan-tag" :class="TAG_CLASS">ports {{ scan.ports }}</span>
    <template v-if="scan?.targets?.length">
      <span v-for="t in scan.targets" :key="t" data-test="scan-tag" :class="TAG_CLASS">{{ t }}</span>
    </template>
    <RouterLink v-else :to="toSettings('network-scan')" class="text-[length:var(--text-small)] text-accent-info-text underline-offset-2 hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring" title="Choose which networks may be scanned">Every allowed network</RouterLink>
  </span>
</template>
