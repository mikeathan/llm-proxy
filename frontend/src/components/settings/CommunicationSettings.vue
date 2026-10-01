<script setup lang="ts">
import { ref, computed, onMounted, provide, watch } from "vue"
import type { GlobalConfig, ConnectorConfig, ConnectorForm } from "../../types/admin"
import type { DataTableColumn } from "../../types/ui"
import { AdminApiService } from "../../services/admin/adminService"
import { useWebhook } from "../../composables/useWebhook"
import { useToolSecrets } from "../../composables/useToolSecrets"
import { useConfirm } from "../../composables/ui/useConfirm"
import BaseButton from "../common/buttons/BaseButton.vue"
import BaseToggle from "../common/buttons/BaseToggle.vue"
import Panel from "../common/layout/Panel.vue"
import DataTable from "../common/display/DataTable.vue"
import FormField from "../common/forms/FormField.vue"
import SettingsActions from "./SettingsActions.vue"
import WebhookPanel from "./WebhookPanel.vue"

// Settings · Communication: connectors the agent sends notifications and
// reports through. Connectors live in the shared configuration; each bot token
// is a stored secret, saved first by this section's Save. `configDirty` /
// `saving` / `configError` are the page's save state; typed tokens are reported
// to the page's unsaved-change guard via dirty-change.
const props = defineProps<{
  editConfig: GlobalConfig
  configDirty?: boolean
  saving?: boolean
  configError?: string
}>()

const emit = defineEmits<{
  (e: "update:editConfig", config: GlobalConfig): void
  (e: "updateConfig"): void
  (e: "discard"): void
  (e: "dirty-change", dirty: boolean): void
}>()

const CONNECTOR_TYPES = [{ value: "telegram", label: "Telegram" }]
const EMPTY_FORM: ConnectorForm = { name: "", type: "telegram", chat_id: "", workspace_id: "", token: "", webhook_token: "" }

const connectors = computed({
  get: () => props.editConfig.communication?.connectors ?? {},
  set: (val: Record<string, ConnectorConfig>) => {
    emit("update:editConfig", { ...props.editConfig, communication: { ...props.editConfig.communication, connectors: val } })
  },
})
const connectorNames = computed(() => Object.keys(connectors.value))
const withInbound = computed(() => connectorNames.value.filter((name) => connectors.value[name]?.settings?.workspace_id))

const showForm = ref(false)
const editingName = ref<string | null>(null)
const form = ref<ConnectorForm>({ ...EMPTY_FORM })

const saveError = ref("")
const tokenMgr = useToolSecrets("connector")
const { confirm } = useConfirm()

watch(tokenMgr.isDirty, (dirty) => emit("dirty-change", dirty), { immediate: true })
const dirty = computed(() => !!props.configDirty || tokenMgr.isDirty.value)

const { verifyWebhook, clearWebhookState } = useWebhook(connectors, saveError)
// WebhookPanel child uses the same singleton composable via provide/inject
// and handles create/delete itself — parent only needs verify (for onMounted
// auto-check) and clearWebhookState (for removeConnector cleanup).
provide("connectors", connectors)
provide("saveError", saveError)

const isEditing = computed(() => editingName.value !== null)
// Adding under an existing name would silently replace that connector.
const nameTaken = computed(() => !isEditing.value && form.value.name.trim() in connectors.value)
const canSubmit = computed(() => !!form.value.name.trim() && !nameTaken.value)

onMounted(async () => {
  for (const name of connectorNames.value) {
    await tokenMgr.load(name)
    if (connectors.value[name]?.settings?.workspace_id) {
      await verifyWebhook(name)
    }
  }
})

function startAdd() {
  editingName.value = null
  form.value = { ...EMPTY_FORM }
  showForm.value = true
}

function editConnector(name: string) {
  const cfg = connectors.value[name]
  if (!cfg) return
  editingName.value = name
  form.value = {
    ...EMPTY_FORM,
    name,
    type: cfg.type,
    chat_id: cfg.settings?.chat_id ?? "",
    workspace_id: cfg.settings?.workspace_id ?? "",
  }
  showForm.value = true
}

// Stages the connector in the configuration (saved by Save) and queues a typed
// token; an empty token field keeps the stored one.
function applyForm() {
  const name = form.value.name.trim()
  if (!canSubmit.value) return

  const settings: Record<string, string> = {}
  if (form.value.chat_id) settings.chat_id = form.value.chat_id
  if (form.value.workspace_id) settings.workspace_id = form.value.workspace_id
  if (form.value.webhook_token) settings.webhook_token = form.value.webhook_token

  const existing = connectors.value[name]
  const updated = { ...connectors.value }
  updated[name] = existing && isEditing.value
    ? { ...existing, settings }
    : { type: form.value.type, enabled: true, settings, secret_ref: form.value.token ? name : undefined }
  connectors.value = updated

  if (form.value.token) {
    tokenMgr.ensureTracked(name)
    tokenMgr.tokens.value[name]!.dirty = form.value.token
  }
  closeForm()
}

async function removeConnector(name: string) {
  const ok = await confirm({
    title: `Remove ${name}?`,
    message: "Its stored bot token is deleted now; the connector itself is removed when you save.",
    type: "warning",
    confirmText: "Remove",
  })
  if (!ok) return
  const updated = { ...connectors.value }
  delete updated[name]
  connectors.value = updated
  delete tokenMgr.tokens.value[name]
  clearWebhookState(name)
  try {
    await AdminApiService.deleteToolSecret("connector", name)
  } catch {
    // secret may not exist — that's fine
  }
}

function setEnabled(name: string, enabled: boolean) {
  const current = connectors.value[name]
  if (current) connectors.value = { ...connectors.value, [name]: { ...current, enabled } }
}

function closeForm() {
  showForm.value = false
  editingName.value = null
  form.value = { ...EMPTY_FORM }
}

async function save() {
  saveError.value = ""
  if (await tokenMgr.saveDirty(saveError)) {
    emit("updateConfig")
  }
}

function discard() {
  saveError.value = ""
  tokenMgr.discard()
  closeForm()
  emit("discard")
}

const COLUMNS: DataTableColumn<string>[] = [
  { key: "name", label: "Connector", value: (name) => name },
  { key: "type", label: "Type" },
  { key: "target", label: "Chat" },
  { key: "enabled", label: "Enabled" },
  { key: "actions", label: "Actions" },
]
</script>

<template>
  <div class="flex flex-col gap-4">
    <Panel title="Connectors" flush>
      <template #actions>
        <BaseButton v-if="!showForm" variant="ghost" size="sm" icon="plus" @click="startAdd">Add connector</BaseButton>
      </template>
      <DataTable
        :columns="COLUMNS"
        :rows="connectorNames"
        :row-key="(name) => name"
        caption="Connectors"
        empty-title="No connectors"
        empty-body="Add a Telegram connector so the agent can send notifications and reports."
      >
        <template #cell-name="{ row }"><span class="font-mono text-primary">{{ row }}</span></template>
        <template #cell-type="{ row }"><span class="capitalize">{{ connectors[row]?.type }}</span></template>
        <template #cell-target="{ row }">
          <span class="font-mono text-muted">{{ connectors[row]?.settings?.chat_id || "—" }}</span>
        </template>
        <template #cell-enabled="{ row }">
          <BaseToggle :model-value="!!connectors[row]?.enabled" :label="`Enable ${row}`" hide-label @update:model-value="setEnabled(row, $event)" />
        </template>
        <template #cell-actions="{ row }">
          <span class="inline-flex items-center gap-1">
            <BaseButton variant="ghost" size="sm" icon="edit" icon-only :label="`Edit ${row}`" @click="editConnector(row)" />
            <BaseButton variant="ghost" size="sm" icon="trash" icon-only :label="`Remove ${row}`" @click="removeConnector(row)" />
          </span>
        </template>
      </DataTable>
    </Panel>

    <Panel v-if="showForm" :title="isEditing ? `Edit ${editingName}` : 'New connector'" preserve-case>
      <form class="flex flex-col gap-4" @submit.prevent="applyForm">
        <div class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,240px),1fr))] gap-4">
          <FormField
            label="Connector name"
            :hint="isEditing ? 'The name identifies the connector and cannot change.' : 'Identifies the connector, e.g. my-telegram.'"
            :error="nameTaken ? `A connector named ${form.name.trim()} already exists — edit it instead.` : undefined"
          >
            <template #default="{ id, describedBy, invalid }">
              <input :id="id" v-model="form.name" :aria-describedby="describedBy" :aria-invalid="invalid" :readonly="isEditing" type="text" class="form-control font-mono read-only:text-muted" autocomplete="off" />
            </template>
          </FormField>
          <FormField label="Type">
            <template #default="{ id }">
              <select :id="id" v-model="form.type" :disabled="isEditing" class="form-control">
                <option v-for="option in CONNECTOR_TYPES" :key="option.value" :value="option.value">{{ option.label }}</option>
              </select>
            </template>
          </FormField>
          <FormField label="Chat ID" hint="Where outbound messages go.">
            <template #default="{ id, describedBy }">
              <input :id="id" v-model="form.chat_id" :aria-describedby="describedBy" type="text" class="form-control font-mono" autocomplete="off" />
            </template>
          </FormField>
          <FormField label="Workspace for inbound messages" hint="Optional. Messages sent to the bot run in this workspace.">
            <template #default="{ id, describedBy }">
              <input :id="id" v-model="form.workspace_id" :aria-describedby="describedBy" type="text" class="form-control font-mono" autocomplete="off" />
            </template>
          </FormField>
          <FormField label="Bot token" :hint="isEditing ? 'Leave empty to keep the stored token.' : 'Stored encrypted when you save.'">
            <template #default="{ id, describedBy }">
              <input :id="id" v-model="form.token" :aria-describedby="describedBy" type="password" class="form-control font-mono" autocomplete="new-password" data-1p-ignore data-lpignore="true" />
            </template>
          </FormField>
          <FormField label="Webhook secret token" hint="Optional. Telegram sends it with every inbound message.">
            <template #default="{ id, describedBy }">
              <input :id="id" v-model="form.webhook_token" :aria-describedby="describedBy" type="password" class="form-control font-mono" autocomplete="new-password" data-1p-ignore data-lpignore="true" />
            </template>
          </FormField>
        </div>
        <div class="flex flex-wrap justify-end gap-2">
          <BaseButton variant="ghost" @click="closeForm">Cancel</BaseButton>
          <BaseButton type="submit" variant="secondary" :disabled="!canSubmit">{{ isEditing ? "Apply changes" : "Add connector" }}</BaseButton>
        </div>
      </form>
    </Panel>

    <template v-if="!showForm">
      <WebhookPanel v-for="name in withInbound" :key="`url-${name}`" :name="name" :cfg="connectors[name]!" />
    </template>

    <SettingsActions :dirty="dirty" :saving="saving" :error="saveError || configError" save-label="Save communication settings" @save="save" @discard="discard" />
  </div>
</template>
