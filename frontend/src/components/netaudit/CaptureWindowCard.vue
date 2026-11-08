<script setup lang="ts">
import { onMounted } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import { useCaptureWindow } from "../../composables/netaudit/useCaptureWindow"
import {
  CAPTURE_BUTTON, CAPTURE_CONFLICT_NOTE, CAPTURE_DHCP_NOTE, CAPTURE_NEVER, CAPTURE_PASSPHRASE_NOTE, CAPTURE_RUNNING, CAPTURE_SNAPSHOT_NOTE, CAPTURE_STOPPED, CAPTURE_TITLE,
} from "../../constants/netaudit"
import { formatRelativeTime } from "../../utils/format/time"

// Network · Devices · Advanced: a manual 60-second listen on this computer's own link, through the privileged helper. It shows when the
// helper is ready and this action is on, says plainly that it is a snapshot, and shows the last result as counts. It makes no finding.
const { available, summary, loaded, running, error, load, listen } = useCaptureWindow()
onMounted(load)
</script>

<template>
  <Panel v-if="loaded && (available || summary)" :title="CAPTURE_TITLE">
    <div class="flex flex-col gap-3" data-test="capture">
      <p class="m-0 text-[length:var(--text-small)] text-secondary" data-test="capture-note">{{ CAPTURE_SNAPSHOT_NOTE }}</p>
      <div v-if="available" class="flex flex-wrap items-center gap-2">
        <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" :disabled="running" data-test="capture-listen" @click="listen">{{ running ? CAPTURE_RUNNING : CAPTURE_BUTTON }}</BaseButton>
        <span class="text-[length:var(--text-small)] text-muted">{{ CAPTURE_PASSPHRASE_NOTE }}</span>
      </div>
      <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error" data-test="capture-error">{{ error }}</p>

      <p v-if="!summary" class="m-0 text-[length:var(--text-small)] text-muted" data-test="capture-never">{{ CAPTURE_NEVER }}</p>
      <div v-else class="flex flex-col gap-1.5 text-[length:var(--text-small)] text-secondary" data-test="capture-result">
        <span class="text-muted">{{ summary.interface }}{{ summary.wireless ? ' (Wi-Fi)' : '' }} · {{ formatRelativeTime(summary.at) }} · {{ CAPTURE_STOPPED[summary.stopped_by] ?? summary.stopped_by }}</span>
        <span data-test="capture-frames">{{ summary.frames }} frames heard, at most {{ summary.max_per_second }} in one second<template v-if="summary.kernel_dropped"> · {{ summary.kernel_dropped }} dropped because they came too fast</template></span>
        <ul class="m-0 flex list-none flex-wrap gap-x-4 gap-y-0.5 p-0" data-test="capture-counts">
          <li>Address requests {{ summary.counts.arp_requests }}</li>
          <li>Address answers {{ summary.counts.arp_replies }}</li>
          <li>Self-announcements {{ summary.counts.arp_gratuitous }}</li>
          <li>DHCP from a server {{ summary.counts.dhcp_from_server }}</li>
          <li>DHCP from a device {{ summary.counts.dhcp_from_client }}</li>
          <li>Device discovery (mDNS) {{ summary.counts.mdns }}</li>
          <li>IPv6 neighbour messages {{ summary.counts.nd_router_solicit + summary.counts.nd_router_advert + summary.counts.nd_neighbor_solicit + summary.counts.nd_neighbor_advert + summary.counts.nd_redirect }}</li>
          <li>Not readable {{ summary.counts.unreadable }}</li>
        </ul>
        <p v-if="summary.dhcp_server_macs.length > 1" class="m-0 text-state-running" data-test="capture-dhcp">{{ summary.dhcp_server_macs.length }} different sources answered as a DHCP server. {{ CAPTURE_DHCP_NOTE }}</p>
        <p v-if="summary.conflicting_addresses" class="m-0 text-state-running" data-test="capture-conflict">{{ CAPTURE_CONFLICT_NOTE(summary.conflicting_addresses) }}</p>
      </div>
    </div>
  </Panel>
</template>
