<script setup lang="ts">
import { onMounted } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import { useWebTemplates } from "../../composables/netaudit/useWebTemplates"
import { WEB_TEMPLATES_NOTE, WEB_TEMPLATES_TITLE, webTemplatesLine } from "../../constants/netaudit"

// Network · Settings · Web check templates: how many of the reviewed templates are on this computer, why the web checks cannot
// run yet (nuclei missing, templates not downloaded), and the one button that fetches them. Nothing else ever downloads them.
const { state, error, busy, load, download } = useWebTemplates()
onMounted(load)
</script>

<template>
  <Panel :title="WEB_TEMPLATES_TITLE">
    <div class="flex flex-col gap-3" data-test="web-templates">
      <ErrorState v-if="error && !state" title="The web check templates could not be read" :cause="error" next="Check that the server is running, then retry.">
        <template #action><BaseButton variant="secondary" icon="refresh" class-name="max-sm:!h-11" @click="load">Retry</BaseButton></template>
      </ErrorState>
      <LoadingState v-else-if="!state" label="Reading the web check templates" />
      <template v-else>
        <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <p class="m-0 min-w-0 text-primary" data-test="web-templates-line">{{ webTemplatesLine(state.present, state.total, state.tag) }}</p>
          <BaseButton variant="primary" class-name="max-sm:!h-11" :loading="busy" data-test="web-templates-download" @click="download">{{ state.present > 0 ? "Download checks again" : "Download checks" }}</BaseButton>
        </div>
        <p v-if="state.reason" class="m-0 text-[length:var(--text-small)] text-state-running" data-test="web-templates-reason">{{ state.reason }}</p>
        <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ error }}</p>
        <p class="m-0 text-[length:var(--text-small)] text-muted">{{ WEB_TEMPLATES_NOTE }}</p>
      </template>
    </div>
  </Panel>
</template>
