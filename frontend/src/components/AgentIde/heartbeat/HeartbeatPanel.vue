<script setup lang="ts">
import { computed, toRef } from 'vue'
import { useHeartbeat } from '../../../composables/automation/useHeartbeat'
import { useConnectorOptions } from '../../../composables/automation/useConnectorOptions'
import { useModels } from '../../../composables/models/useModels'
import { useUnsavedChangesGuard } from '../../../composables/ui/useUnsavedChangesGuard'
import { HEARTBEAT_TASK_FILE, heartbeatEveryOptions, heartbeatStatusText } from '../../../utils/automation/heartbeat'
import { asUtc, formatAbsoluteTime, formatRelativeTime } from '../../../utils/format/time'
import { toSettings, toWorkspaceFile } from '../../../router/routes'
import Panel from '../../common/layout/Panel.vue'
import FormField from '../../common/forms/FormField.vue'
import ToggleField from '../../common/forms/ToggleField.vue'
import UnsavedTag from '../../common/display/UnsavedTag.vue'
import BaseButton from '../../common/buttons/BaseButton.vue'
import LoadingState from '../../common/feedback/LoadingState.vue'
import ErrorState from '../../common/feedback/ErrorState.vue'

// The workspace Heartbeat section: a frequent, cheap check that only speaks up when something needs
// attention. The checks themselves live in heartbeat.md; this panel owns when, with what, and where to.
const props = defineProps<{ workspaceId: string }>()

const { state, draft, loading, loadError, saving, saveError, dirty, load, save, discard } = useHeartbeat(
  toRef(props, 'workspaceId'),
)
const { state: admin } = useModels()
const { connectorOptions, noConnectors } = useConnectorOptions(computed(() => draft.value?.connector ?? ''))
useUnsavedChangesGuard(dirty)

const models = computed(() => admin.value?.models ?? [])
const localModels = computed(() => models.value.filter((m) => m.provider === 'local'))
const cloudModels = computed(() => models.value.filter((m) => m.provider !== 'local'))
const everyOptions = computed(() => heartbeatEveryOptions(draft.value?.every ?? ''))

// A chosen model says for itself whether it is local; with none chosen the server has resolved the default.
const wakesLocalModel = computed(() => {
  if (!draft.value?.enabled || !state.value) return false
  if (draft.value.model) return localModels.value.some((m) => m.name === draft.value!.model)
  return !state.value.config.model && state.value.wakes_local_model
})
const showNoChecks = computed(() => !!draft.value?.enabled && state.value?.has_checks === false)
const status = computed(() => state.value?.status)
</script>

<template>
  <Panel title="Heartbeat">
    <LoadingState v-if="loading && !state" label="Loading the heartbeat" :rows="3" />
    <ErrorState
      v-else-if="loadError"
      title="Could not load the heartbeat"
      :cause="loadError"
      next="Check that the backend is running, then retry."
    >
      <template #action><BaseButton variant="secondary" icon="refresh" @click="load">Retry</BaseButton></template>
    </ErrorState>
    <form v-else-if="draft && state" class="flex flex-col gap-4" @submit.prevent="save">
      <p class="m-0 max-w-[64ch] text-[length:var(--text-small)] text-muted">
        A frequent, low-cost check that only speaks up when something needs attention. It reads the checks you write in
        <RouterLink :to="toWorkspaceFile(workspaceId, HEARTBEAT_TASK_FILE)" class="font-mono text-accent-info-text hover:underline">{{ HEARTBEAT_TASK_FILE }}</RouterLink>.
      </p>

      <ToggleField
        v-model="draft.enabled"
        label="Heartbeat"
        hint="While on, each check sends the checks in heartbeat.md to the model."
      />

      <div class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,220px),1fr))] gap-4">
        <FormField label="Check every">
          <template #default="{ id }">
            <select :id="id" v-model="draft.every" :disabled="!draft.enabled" class="form-control">
              <option v-for="opt in everyOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
            </select>
          </template>
        </FormField>
        <FormField label="Model" hint="Which model runs the checks. A cloud model keeps the local one asleep.">
          <template #default="{ id, describedBy }">
            <select :id="id" v-model="draft.model" :aria-describedby="describedBy" :disabled="!draft.enabled" class="form-control">
              <option value="">Workspace default</option>
              <optgroup v-if="localModels.length" label="Local models">
                <option v-for="m in localModels" :key="m.name" :value="m.name">{{ m.name }}</option>
              </optgroup>
              <optgroup v-if="cloudModels.length" label="Cloud models">
                <option v-for="m in cloudModels" :key="m.name" :value="m.name">{{ m.name }} · {{ m.provider }}</option>
              </optgroup>
            </select>
          </template>
        </FormField>
        <FormField label="Send alerts to" hint="Quiet checks are never sent.">
          <template #default="{ id, describedBy }">
            <select :id="id" v-model="draft.connector" :aria-describedby="describedBy" :disabled="!draft.enabled" class="form-control">
              <option value="">Don't send alerts</option>
              <option v-for="opt in connectorOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
            </select>
          </template>
        </FormField>
      </div>
      <p v-if="noConnectors && draft.enabled" role="note" class="m-0 text-[length:var(--text-small)] text-muted">
        No connectors yet.
        <RouterLink :to="toSettings('communication')" class="text-accent-info-text hover:underline">Add one in Settings → Communication</RouterLink>
        to receive alerts in Telegram.
      </p>

      <p v-if="wakesLocalModel" role="note" class="m-0 text-[length:var(--text-small)] text-state-running">
        Every check starts the local model. Pick a cloud model to keep it asleep.
      </p>
      <p v-if="showNoChecks" role="note" class="m-0 text-[length:var(--text-small)] text-secondary">
        No checks yet. Add what to watch in
        <RouterLink :to="toWorkspaceFile(workspaceId, HEARTBEAT_TASK_FILE)" class="font-mono text-accent-info-text hover:underline">{{ HEARTBEAT_TASK_FILE }}</RouterLink>.
        Until then every check is skipped at no cost.
      </p>

      <p data-test="heartbeat-status" class="m-0 text-[length:var(--text-small)] text-muted">
        <template v-if="status">
          Last check
          <time :datetime="asUtc(status.at)" :title="formatAbsoluteTime(asUtc(status.at))">{{ formatRelativeTime(asUtc(status.at)) }}</time>:
          {{ heartbeatStatusText(status.result) }}.
        </template>
        <template v-else>No checks have run yet.</template>
      </p>

      <p v-if="saveError" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error">{{ saveError }}</p>
      <div class="flex flex-wrap items-center justify-end gap-2">
        <UnsavedTag v-if="dirty" class="mr-auto" />
        <BaseButton variant="ghost" :disabled="!dirty || saving" @click="discard">Discard</BaseButton>
        <BaseButton type="submit" :disabled="!dirty || saving">{{ saving ? 'Saving…' : 'Save' }}</BaseButton>
      </div>
    </form>
  </Panel>
</template>
