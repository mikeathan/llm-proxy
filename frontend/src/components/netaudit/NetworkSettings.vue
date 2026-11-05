<script setup lang="ts">
import ScanScopeSettings from "./ScanScopeSettings.vue"
import ScanLimitsSettings from "./ScanLimitsSettings.vue"
import VulnDataCard from "./VulnDataCard.vue"
import UbuntuDataCard from "./UbuntuDataCard.vue"
import DeviceCheckSettings from "./DeviceCheckSettings.vue"
import DecoysCard from "./DecoysCard.vue"
import RouterLogCard from "./RouterLogCard.vue"
import OutsideLookupCard from "./OutsideLookupCard.vue"
import DnsCheckCard from "./DnsCheckCard.vue"
import NearbyCard from "./NearbyCard.vue"
import Panel from "../common/layout/Panel.vue"
import ToggleField from "../common/forms/ToggleField.vue"
import { useNetauditSwitch } from "../../composables/netaudit/useNetauditSwitch"
import { NetauditService } from "../../services/netaudit/netauditService"
import { SETTINGS_EXTRAS_TITLE, SETTINGS_SAVES_AT_ONCE, SETTINGS_WATCHING_TITLE, SUMMARY_DETAILS, SUMMARY_HINT, SUMMARY_LABEL, SUMMARY_LOAD_FAILED, SUMMARY_SAVE_FAILED,
  CHECK_IN_DETAILS, CHECK_IN_HINT, CHECK_IN_LABEL, CHECK_IN_LOAD_FAILED, CHECK_IN_SAVE_FAILED, MESSAGES_PANEL_TITLE } from "../../constants/netaudit"

// Network · Settings: every switch and limit of the Network page in one place, so no other tab carries a control that
// changes how the network is watched. In the order the person decides things: what may be scanned, what is watched for, the
// optional extras that look further, the vulnerability data, and (Advanced) the scan limits. Each part saves at once.
const summary = useNetauditSwitch(() => NetauditService.weeklySummary(), (on) => NetauditService.setWeeklySummary(on), { load: SUMMARY_LOAD_FAILED, save: SUMMARY_SAVE_FAILED })
const checkIn = useNetauditSwitch(() => NetauditService.dailyCheckIn(), (on) => NetauditService.setDailyCheckIn(on), { load: CHECK_IN_LOAD_FAILED, save: CHECK_IN_SAVE_FAILED })
const GROUP_HEADING = "m-0 font-mono text-[length:var(--text-micro)] font-medium uppercase tracking-[var(--tracking-micro)] text-muted"
</script>

<template>
  <div class="flex flex-col gap-4" data-test="network-settings">
    <p class="m-0 text-[length:var(--text-small)] text-muted">{{ SETTINGS_SAVES_AT_ONCE }}</p>
    <ScanScopeSettings />

    <section :aria-label="SETTINGS_WATCHING_TITLE" class="flex flex-col gap-3" data-test="group-watching">
      <h2 :class="GROUP_HEADING">{{ SETTINGS_WATCHING_TITLE }}</h2>
      <DeviceCheckSettings />
      <Panel :title="MESSAGES_PANEL_TITLE">
        <div class="flex flex-col gap-3">
          <div class="flex flex-col gap-1">
            <ToggleField :model-value="summary.enabled.value" :label="SUMMARY_LABEL" :hint="SUMMARY_HINT" :details="SUMMARY_DETAILS" :disabled="summary.busy.value" @update:model-value="summary.set" />
            <p v-if="summary.error.value" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ summary.error.value }}</p>
          </div>
          <div class="flex flex-col gap-1" data-test="daily-check-in">
            <ToggleField :model-value="checkIn.enabled.value" :label="CHECK_IN_LABEL" :hint="CHECK_IN_HINT" :details="CHECK_IN_DETAILS" :disabled="checkIn.busy.value" @update:model-value="checkIn.set" />
            <p v-if="checkIn.error.value" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ checkIn.error.value }}</p>
          </div>
        </div>
      </Panel>
    </section>

    <section :aria-label="SETTINGS_EXTRAS_TITLE" class="flex flex-col gap-3" data-test="group-extras">
      <h2 :class="GROUP_HEADING">{{ SETTINGS_EXTRAS_TITLE }}</h2>
      <DecoysCard />
      <RouterLogCard />
      <OutsideLookupCard />
      <DnsCheckCard />
      <NearbyCard />
    </section>

    <VulnDataCard />
    <UbuntuDataCard />
    <ScanLimitsSettings />
  </div>
</template>
