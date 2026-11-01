<script setup lang="ts">
import { onMounted, ref, watch } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import ToggleField from "../common/forms/ToggleField.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import { useOutsideLookup } from "../../composables/netaudit/useOutsideLookup"
import { useNetauditViewMode } from "../../composables/netaudit/useNetauditViewMode"
import {
  OUTSIDE_LOOKUP_DETAILS, OUTSIDE_LOOKUP_HINT, OUTSIDE_LOOKUP_INTERNET_OFF, OUTSIDE_LOOKUP_LABEL, OUTSIDE_LOOKUP_OPEN_SETTINGS, OUTSIDE_LOOKUP_READ_FAILED_NEXT, OUTSIDE_LOOKUP_READ_FAILED_TITLE, OUTSIDE_LOOKUP_READING,
  OUTSIDE_LOOKUP_RETRY, OUTSIDE_LOOKUP_TITLE, OUTSIDE_LOOKUP_URL_HINT, OUTSIDE_LOOKUP_URL_LABEL, OUTSIDE_LOOKUP_URL_SAVE,
} from "../../constants/netaudit"
import { outsideLookupLine } from "../../utils/netaudit/outsideLookup"
import { formatRelativeTime } from "../../utils/format/time"
import { toSettings } from "../../router/routes"

// Network → Devices → Internet view: the switch for asking an outside scan database what the internet can reach, and what the
// last look did. Findings appear with the others. Simple is the switch and one status line; Advanced adds which service is asked.
const { state, error, busy, load, set, setUrl } = useOutsideLookup()
const { advanced } = useNetauditViewMode()
const url = ref("")

watch(state, (s) => { if (s) url.value = s.url === s.default_url ? "" : s.url }, { immediate: true })
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
        <ToggleField :model-value="state.enabled" :label="OUTSIDE_LOOKUP_LABEL" :hint="OUTSIDE_LOOKUP_HINT" :details="OUTSIDE_LOOKUP_DETAILS" :disabled="busy" @update:model-value="set" />

        <p v-if="state.enabled" class="m-0 text-[length:var(--text-small)] text-secondary" data-test="outside-lookup-status">{{ outsideLookupLine(state.last, state.last.outcome ? formatRelativeTime(state.last.at) : "") }}</p>

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
