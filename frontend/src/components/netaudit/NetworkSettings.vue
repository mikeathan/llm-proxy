<script setup lang="ts">
import ScanScopeSettings from "./ScanScopeSettings.vue"
import ScanLimitsSettings from "./ScanLimitsSettings.vue"
import SettingsGroupNav from "./SettingsGroupNav.vue"
import VulnDataCard from "./VulnDataCard.vue"
import UbuntuDataCard from "./UbuntuDataCard.vue"
import KnownBadCard from "./KnownBadCard.vue"
import DeviceCheckSettings from "./DeviceCheckSettings.vue"
import DecoysCard from "./DecoysCard.vue"
import RouterLogCard from "./RouterLogCard.vue"
import OutsideLookupCard from "./OutsideLookupCard.vue"
import DnsCheckCard from "./DnsCheckCard.vue"
import FootprintCard from "./FootprintCard.vue"
import NearbyCard from "./NearbyCard.vue"
import WifiCard from "./WifiCard.vue"
import HelperCard from "./HelperCard.vue"
import { SETTINGS_GROUP_COPY, SETTINGS_SAVES_AT_ONCE } from "../../constants/netaudit"
import type { NetworkSettingsGroup } from "../../types/routes"

// Network · Settings: the limits and details of the Network page, in five groups named for their job, one shown at a time. What may be scanned and
// the scan limits come first, the checks that watch next, then the looks that ask an outside service, the downloaded tables and the privileged
// helper. Every switch is on the Modules tab. Each part saves at once. The group is chosen in the address (`?group=`), so a link can open it.
defineProps<{ group: NetworkSettingsGroup }>()
</script>

<template>
  <div class="flex flex-col gap-4" data-test="network-settings">
    <SettingsGroupNav :active="group" />
    <p class="m-0 text-[length:var(--text-small)] text-muted" data-test="settings-hint">{{ SETTINGS_GROUP_COPY[group].hint }} {{ SETTINGS_SAVES_AT_ONCE }}</p>

    <section :aria-label="SETTINGS_GROUP_COPY[group].label" class="flex flex-col gap-4" :data-test="`group-${group}`">
      <template v-if="group === 'scanning'">
        <ScanScopeSettings />
        <ScanLimitsSettings />
      </template>
      <template v-else-if="group === 'watching'">
        <DeviceCheckSettings />
        <DecoysCard />
        <RouterLogCard />
        <DnsCheckCard />
        <NearbyCard />
        <WifiCard />
      </template>
      <template v-else-if="group === 'outside'">
        <OutsideLookupCard />
        <FootprintCard />
        <KnownBadCard />
      </template>
      <template v-else-if="group === 'data'">
        <VulnDataCard />
        <UbuntuDataCard />
      </template>
      <HelperCard v-else />
    </section>
  </div>
</template>
