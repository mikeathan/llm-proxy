<script setup lang="ts">
import ScanScopeSettings from "./ScanScopeSettings.vue"
import ScanLimitsSettings from "./ScanLimitsSettings.vue"
import VulnDataCard from "./VulnDataCard.vue"
import UbuntuDataCard from "./UbuntuDataCard.vue"
import WebTemplatesCard from "./WebTemplatesCard.vue"
import KnownBadCard from "./KnownBadCard.vue"
import DeviceCheckSettings from "./DeviceCheckSettings.vue"
import DecoysCard from "./DecoysCard.vue"
import RouterLogCard from "./RouterLogCard.vue"
import OutsideLookupCard from "./OutsideLookupCard.vue"
import DnsCheckCard from "./DnsCheckCard.vue"
import NearbyCard from "./NearbyCard.vue"
import { SETTINGS_EXTRAS_TITLE, SETTINGS_SAVES_AT_ONCE, SETTINGS_WATCHING_TITLE } from "../../constants/netaudit"

// Network · Settings: the limits and details of the Network page in one place: what may be scanned, how the checks are doing, the
// optional extras that look further, the vulnerability data, and (Advanced) the scan limits. Every switch is on the Modules tab.
// Each part saves at once.
const GROUP_HEADING = "m-0 font-mono text-[length:var(--text-micro)] font-medium uppercase tracking-[var(--tracking-micro)] text-muted"
</script>

<template>
  <div class="flex flex-col gap-4" data-test="network-settings">
    <p class="m-0 text-[length:var(--text-small)] text-muted">{{ SETTINGS_SAVES_AT_ONCE }}</p>
    <ScanScopeSettings />

    <section :aria-label="SETTINGS_WATCHING_TITLE" class="flex flex-col gap-3" data-test="group-watching">
      <h2 :class="GROUP_HEADING">{{ SETTINGS_WATCHING_TITLE }}</h2>
      <DeviceCheckSettings />
    </section>

    <section :aria-label="SETTINGS_EXTRAS_TITLE" class="flex flex-col gap-3" data-test="group-extras">
      <h2 :class="GROUP_HEADING">{{ SETTINGS_EXTRAS_TITLE }}</h2>
      <DecoysCard />
      <RouterLogCard />
      <OutsideLookupCard />
      <DnsCheckCard />
      <NearbyCard />
      <KnownBadCard />
    </section>

    <VulnDataCard />
    <UbuntuDataCard />
    <WebTemplatesCard />
    <ScanLimitsSettings />
  </div>
</template>
