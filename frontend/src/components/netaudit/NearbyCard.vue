<script setup lang="ts">
import { onMounted } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import ModuleSwitchNote from "../stacks/ModuleSwitchNote.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import { useNearby } from "../../composables/netaudit/useNearby"
import { NEARBY_DETAILS, NEARBY_HINT, NEARBY_READ_FAILED_TITLE, NEARBY_SETTINGS_TITLE } from "../../constants/netaudit"

// Network · Settings · Nearby devices: what it sends is said here; it is switched on the Modules tab. The devices themselves are on the Devices tab,
// with the Check now button; no other tab changes what is sent.
const { state, error, load } = useNearby()
onMounted(load)
</script>

<template>
  <Panel :title="NEARBY_SETTINGS_TITLE">
    <div class="flex flex-col gap-3" data-test="nearby-card">
      <ErrorState v-if="error && !state" :title="NEARBY_READ_FAILED_TITLE" :cause="error" next="Check that the server is running, then retry.">
        <template #action><BaseButton variant="secondary" icon="refresh" class-name="max-sm:!h-11" @click="load">Retry</BaseButton></template>
      </ErrorState>
      <LoadingState v-else-if="!state" label="Reading the setting" />
      <template v-else>
        <ModuleSwitchNote :enabled="state.enabled" :hint="NEARBY_HINT" :details="NEARBY_DETAILS" />
        <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
      </template>
    </div>
  </Panel>
</template>
