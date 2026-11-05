<script setup lang="ts">
import { onMounted, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import { toNetwork } from "../../router/routes"
import { MODULES_LINK_LABEL } from "../../constants/stacks"
import { useKnownBad } from "../../composables/netaudit/useKnownBad"
import { formatRelativeTime, isRecorded } from "../../utils/format/time"
import {
  KNOWN_BAD_ADDRESS_HINT, KNOWN_BAD_ADDRESS_LABEL, KNOWN_BAD_ADDRESS_SAVED, KNOWN_BAD_HOWTO, KNOWN_BAD_INTRO, KNOWN_BAD_LINK_LABEL, KNOWN_BAD_LINK_URL,
  KNOWN_BAD_NO_ADDRESS, KNOWN_BAD_SWITCH_NOTE, KNOWN_BAD_TITLE, knownBadLine,
} from "../../constants/netaudit"

// Network · Settings · Known-bad names: what the list holds, where to get the address (a link to abuse.ch and a short how-to), the field
// for that address (a secret: kept encrypted, never shown again) and the one button that downloads the list. The comparison itself is
// switched on the Modules tab; this card says only what it needs.
const { state, error, busy, load, saveAddress, removeAddress, download } = useKnownBad()
const address = ref("")
const howto = ref(false)

async function save() {
  if (await saveAddress(address.value)) address.value = ""
}
onMounted(load)
</script>

<template>
  <Panel :title="KNOWN_BAD_TITLE">
    <div class="flex flex-col gap-3" data-test="known-bad">
      <ErrorState v-if="error && !state" title="The known-bad names could not be read" :cause="error" next="Check that the server is running, then retry.">
        <template #action><BaseButton variant="secondary" icon="refresh" class-name="max-sm:!h-11" @click="load">Retry</BaseButton></template>
      </ErrorState>
      <LoadingState v-else-if="!state" label="Reading the known-bad names" />
      <template v-else>
        <p class="m-0 text-secondary">{{ KNOWN_BAD_INTRO }}</p>
        <p class="m-0 flex flex-wrap items-center gap-x-3">
          <a :href="KNOWN_BAD_LINK_URL" target="_blank" rel="noopener noreferrer" class="inline-flex min-h-11 items-center text-accent-info-text underline-offset-2 hover:underline sm:min-h-8" data-test="known-bad-link">{{ KNOWN_BAD_LINK_LABEL }}</a>
          <button type="button" class="inline-flex min-h-11 items-center text-[length:var(--text-small)] text-accent-info-text underline-offset-2 hover:underline sm:min-h-8" :aria-expanded="howto" data-test="known-bad-howto-toggle" @click="howto = !howto">How to</button>
        </p>
        <p v-if="howto" class="m-0 text-[length:var(--text-small)] text-muted" data-test="known-bad-howto">{{ KNOWN_BAD_HOWTO }}</p>

        <form class="flex flex-col gap-2" @submit.prevent="save">
          <FormField :label="KNOWN_BAD_ADDRESS_LABEL" :hint="KNOWN_BAD_ADDRESS_HINT">
            <template #default="{ id, describedBy }">
              <input :id="id" v-model="address" type="password" inputmode="url" autocomplete="off" spellcheck="false" placeholder="https://urlhaus-api.abuse.ch/…" :aria-describedby="describedBy" data-test="known-bad-address" class="h-11 min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 font-mono text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2 sm:h-8" />
            </template>
          </FormField>
          <div class="flex flex-wrap items-center gap-2">
            <BaseButton variant="secondary" size="sm" type="submit" class-name="max-sm:!h-11" :disabled="busy || !address.trim()" data-test="known-bad-save">Save address</BaseButton>
            <BaseButton v-if="state.has_address" variant="ghost" size="sm" class-name="max-sm:!h-11" :disabled="busy" data-test="known-bad-remove" @click="removeAddress">Remove address</BaseButton>
            <span class="text-[length:var(--text-small)] text-muted" data-test="known-bad-address-state">{{ state.has_address ? KNOWN_BAD_ADDRESS_SAVED : KNOWN_BAD_NO_ADDRESS }}</span>
          </div>
        </form>

        <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <p class="m-0 min-w-0 text-primary" data-test="known-bad-line">{{ knownBadLine(state.count, isRecorded(state.loaded_at ?? "") ? formatRelativeTime(state.loaded_at!) : "earlier") }}</p>
          <BaseButton variant="primary" class-name="max-sm:!h-11" :loading="busy" :disabled="!state.has_address" data-test="known-bad-download" @click="download">{{ state.count > 0 ? "Refresh list" : "Download list" }}</BaseButton>
        </div>
        <p v-if="state.reason" class="m-0 text-[length:var(--text-small)] text-state-running" data-test="known-bad-reason">{{ state.reason }}</p>
        <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
        <p class="m-0 text-[length:var(--text-small)] text-muted">{{ KNOWN_BAD_SWITCH_NOTE }} <RouterLink :to="toNetwork('modules')" class="text-accent-info-text underline-offset-2 hover:underline">{{ MODULES_LINK_LABEL }}</RouterLink></p>
      </template>
    </div>
  </Panel>
</template>
