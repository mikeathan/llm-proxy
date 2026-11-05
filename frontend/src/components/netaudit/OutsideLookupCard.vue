<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import ModuleSwitchNote from "../stacks/ModuleSwitchNote.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import { useOutsideLookup } from "../../composables/netaudit/useOutsideLookup"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import {
  OUTSIDE_LOOKUP_ADDRESS_HINT, OUTSIDE_LOOKUP_ADDRESS_LABEL, OUTSIDE_LOOKUP_ADDRESS_SAVE, OUTSIDE_LOOKUP_DETAILS, OUTSIDE_LOOKUP_DIFFERS, OUTSIDE_LOOKUP_HINT, OUTSIDE_LOOKUP_INTERNET_OFF, OUTSIDE_LOOKUP_OPEN_SETTINGS, OUTSIDE_LOOKUP_READ_FAILED_NEXT, OUTSIDE_LOOKUP_READ_FAILED_TITLE, OUTSIDE_LOOKUP_READING,
  OUTSIDE_LOOKUP_RETRY, OUTSIDE_LOOKUP_TITLE, OUTSIDE_LOOKUP_URL_HINT, OUTSIDE_LOOKUP_URL_LABEL, OUTSIDE_LOOKUP_URL_SAVE,
} from "../../constants/netaudit"
import { outsideLookupLine, outsideLookupNeedsAddress } from "../../utils/netaudit/outsideLookup"
import { formatRelativeTime } from "../../utils/format/time"
import { toSettings } from "../../router/routes"

// Network → Settings → Internet view: asking an outside scan database what the internet can reach (switched on the Modules tab), and what the
// last look did. Findings appear with the others. Simple is the switch and one status line; Advanced adds which service is asked.
const { state, error, busy, load, setUrl, setAddress } = useOutsideLookup()
const { advanced } = useNetauditViewMode()
const url = ref("")
const address = ref("")
// The address box shows when it helps (the router could not give one) or is already in use, and always in Advanced.
const showAddress = computed(() => !!state.value?.enabled && (advanced.value || !!state.value.address || outsideLookupNeedsAddress(state.value.last)))

watch(state, (s) => {
  if (!s) return
  url.value = s.url === s.default_url ? "" : s.url
  address.value = s.address
}, { immediate: true })
onMounted(load)
</script>

<template>
  <Panel :title="OUTSIDE_LOOKUP_TITLE">
    <div class="flex flex-col gap-3" data-test="outside-lookup">
      <ErrorState v-if="error && !state" :title="OUTSIDE_LOOKUP_READ_FAILED_TITLE" :cause="error" :next="OUTSIDE_LOOKUP_READ_FAILED_NEXT">
        <template #action><BaseButton variant="secondary" icon="refresh" class-name="max-sm:!h-11" @click="load">{{ OUTSIDE_LOOKUP_RETRY }}</BaseButton></template>
      </ErrorState>
      <LoadingState v-else-if="!state" :label="OUTSIDE_LOOKUP_READING" />

      <template v-else>
        <ModuleSwitchNote :enabled="state.enabled" :hint="OUTSIDE_LOOKUP_HINT" :details="OUTSIDE_LOOKUP_DETAILS" />

        <p v-if="state.enabled" class="m-0 text-[length:var(--text-small)] text-secondary" data-test="outside-lookup-status">{{ outsideLookupLine(state.last, state.last.outcome ? formatRelativeTime(state.last.at) : "") }}</p>

        <p v-if="state.enabled && state.last.differs" class="m-0 text-[length:var(--text-small)] text-state-error" data-test="outside-lookup-differs">{{ OUTSIDE_LOOKUP_DIFFERS }}</p>

        <form v-if="showAddress" class="flex flex-col gap-2" @submit.prevent="setAddress(address)">
          <FormField :label="OUTSIDE_LOOKUP_ADDRESS_LABEL" :hint="OUTSIDE_LOOKUP_ADDRESS_HINT">
            <template #default="{ id, describedBy }">
              <input :id="id" v-model="address" type="text" inputmode="decimal" autocomplete="off" spellcheck="false" placeholder="203.0.113.7" :aria-describedby="describedBy" data-test="outside-lookup-address" class="h-11 min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 font-mono text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2 sm:h-8" />
            </template>
          </FormField>
          <BaseButton variant="secondary" size="sm" type="submit" class-name="self-start max-sm:!h-11" :disabled="busy" data-test="outside-lookup-address-save">{{ OUTSIDE_LOOKUP_ADDRESS_SAVE }}</BaseButton>
        </form>

        <RouterLink v-if="state.enabled && state.last.reason === OUTSIDE_LOOKUP_INTERNET_OFF" :to="toSettings('guardrails')" data-test="outside-lookup-settings" class="inline-flex min-h-11 items-center self-start text-[length:var(--text-small)] text-accent-info-text underline-offset-2 hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring sm:min-h-8">{{ OUTSIDE_LOOKUP_OPEN_SETTINGS }}</RouterLink>

        <form v-if="advanced" class="flex flex-col gap-2" @submit.prevent="setUrl(url)">
          <FormField :label="OUTSIDE_LOOKUP_URL_LABEL" :hint="OUTSIDE_LOOKUP_URL_HINT">
            <template #default="{ id, describedBy }">
              <input :id="id" v-model="url" type="text" inputmode="url" autocomplete="off" spellcheck="false" :placeholder="state.default_url" :aria-describedby="describedBy" data-test="outside-lookup-url" class="h-11 min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 font-mono text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2 sm:h-8" />
            </template>
          </FormField>
          <BaseButton variant="secondary" size="sm" type="submit" class-name="self-start max-sm:!h-11" :disabled="busy" data-test="outside-lookup-save">{{ OUTSIDE_LOOKUP_URL_SAVE }}</BaseButton>
        </form>

        <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
      </template>
    </div>
  </Panel>
</template>
