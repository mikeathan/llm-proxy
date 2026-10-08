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
import TelegramSetupHelp from "./TelegramSetupHelp.vue"

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
const SECRET_MASK = "********"
const SECRET_NOT_SET = "Not set"
const SECRET_STORED = "Stored"
const SECRET_CONFIGURED = "Configured"
const SECRET_PENDING = "Pending save"

const connectors = computed({
  get: () => props.editConfig.communication?.connectors ?? {},
  set: (val: Record<string, ConnectorConfig>) => {
    emit("update:editConfig", { ...props.editConfig, communication: { ...props.editConfig.communication, connectors: val } })
  },
})
const connectorNames = computed(() => Object.keys(connectors.value))
const withInbound = computed(() => connectorNames.value.filter((name) => connectors.value[name]?.settings?.workspace_id))

const showForm = ref(false)
const showTelegramHelp = ref(false)
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
const editedConnector = computed(() => editingName.value ? connectors.value[editingName.value] : undefined)
const editedToken = computed(() => editingName.value ? tokenMgr.tokens.value[editingName.value] : undefined)
const botTokenPlaceholder = computed(() => editedToken.value?.dirty ? SECRET_MASK : editedToken.value?.masked || SECRET_NOT_SET)
const botTokenStatus = computed(() => form.value.token || editedToken.value?.dirty ? SECRET_PENDING : editedToken.value?.masked ? SECRET_STORED : SECRET_NOT_SET)
const webhookSecretPlaceholder = computed(() => editedConnector.value?.settings?.webhook_token ? SECRET_MASK : SECRET_NOT_SET)
const webhookSecretStatus = computed(() => form.value.webhook_token ? SECRET_PENDING : editedConnector.value?.settings?.webhook_token ? SECRET_CONFIGURED : SECRET_NOT_SET)
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

  const existing = connectors.value[name]
  const settings: Record<string, string> = {}
  if (form.value.chat_id) settings.chat_id = form.value.chat_id
  if (form.value.workspace_id) settings.workspace_id = form.value.workspace_id
  if (form.value.webhook_token) settings.webhook_token = form.value.webhook_token
  else if (isEditing.value && existing?.settings?.webhook_token) settings.webhook_token = existing.settings.webhook_token

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
  const cfg = connectors.value[name]
  if (!cfg) return
  const ok = await confirm({
    title: `Remove ${name}?`,
    message: "Its saved webhook is unregistered before its stored bot token is deleted. These actions happen now; the connector itself is removed when you save.",
    type: "warning",
    confirmText: "Remove",
  })
  if (!ok) return
  saveError.value = ""
  try {
    if (cfg.webhook_url) {
      await AdminApiService.deleteConnectorWebhook(name)
      connectors.value = { ...connectors.value, [name]: { ...cfg, webhook_url: undefined } }
      clearWebhookState(name)
    }
    await AdminApiService.deleteToolSecret("connector", name)
    const updated = { ...connectors.value }
    delete updated[name]
    connectors.value = updated
    delete tokenMgr.tokens.value[name]
    clearWebhookState(name)
  } catch (err) {
    // Keep the connector and its token available when cleanup fails.
    saveError.value = `Failed to remove ${name}: ${err instanceof Error ? err.message : String(err)}`
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
        <BaseButton variant="ghost" size="sm" @click="showTelegramHelp = true">Telegram setup help</BaseButton>
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
        <p class="m-0 text-[length:var(--text-small)] text-muted">
          For outgoing notifications, enter a bot token and a destination Chat ID. To receive messages too, add a workspace and register an inbound webhook after saving.
        </p>
        <div class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,240px),1fr))] gap-4">
          <FormField
            label="Connector name"
            :hint="isEditing ? 'Your label for this connector. The name cannot change and is used in its webhook URL.' : 'A label you choose, e.g. my-telegram. The app uses it in the webhook URL; it does not need to match your Telegram bot name.'"
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
          <FormField label="Chat ID" hint="The destination chat, e.g. 987654321 or -1001234567890 for a group. This is separate from the bot token and its numeric prefix.">
            <template #default="{ id, describedBy }">
              <input :id="id" v-model="form.chat_id" :aria-describedby="describedBy" type="text" class="form-control font-mono" autocomplete="off" />
            </template>
          </FormField>
          <FormField label="Workspace for inbound messages" hint="Optional. Enter an existing workspace ID where incoming messages should run. Leave blank for outgoing notifications only.">
            <template #default="{ id, describedBy }">
              <input :id="id" v-model="form.workspace_id" :aria-describedby="describedBy" type="text" class="form-control font-mono" autocomplete="off" />
            </template>
          </FormField>
          <FormField label="Bot token" :hint="isEditing ? 'Paste the full BotFather token, including the colon, to replace it. Leave empty to keep the stored token.' : 'Paste the full token from @BotFather, including the colon: 123456789:example-token. Stored encrypted when you save.'">
            <template #tag><span class="font-mono text-[length:var(--text-micro)] text-faint">{{ botTokenStatus }}</span></template>
            <template #default="{ id, describedBy }">
              <input :id="id" v-model="form.token" :placeholder="botTokenPlaceholder" :aria-describedby="describedBy" type="password" class="form-control font-mono" autocomplete="new-password" data-1p-ignore data-lpignore="true" />
            </template>
          </FormField>
          <FormField label="Webhook secret token" :hint="isEditing ? 'Optional. Leave empty to keep the configured secret, or enter a new one to replace it. Save, then register the webhook again to apply a replacement at Telegram.' : 'Optional. A separate secret you choose to verify incoming webhook requests. Use letters, numbers, underscores or hyphens (1–256 characters).'">
            <template #tag><span class="font-mono text-[length:var(--text-micro)] text-faint">{{ webhookSecretStatus }}</span></template>
            <template #default="{ id, describedBy }">
              <input :id="id" v-model="form.webhook_token" :placeholder="webhookSecretPlaceholder" :aria-describedby="describedBy" type="password" class="form-control font-mono" autocomplete="new-password" data-1p-ignore data-lpignore="true" />
            </template>
          </FormField>
        </div>
        <details class="text-[length:var(--text-small)] text-muted">
          <summary class="cursor-pointer text-secondary focus-visible:outline-none focus-visible:ring-2">How to find your Telegram values</summary>
          <ol class="m-0 mt-3 list-decimal space-y-2 pl-5">
            <li>
              <strong class="text-secondary">Bot token:</strong> Open @BotFather in Telegram and use /mybots to select your bot, then API Token. Copy the entire token with its numeric prefix, colon and remaining characters.
            </li>
            <li>
              <strong class="text-secondary">Chat ID:</strong> Open a chat with your bot, press Start and send a message. Before registering a webhook, call Telegram's getUpdates API using your bot token and copy message.chat.id from that message. For a group, add the bot and send it a command in that group, then copy that message's chat ID, including the minus sign.
              Use <code class="break-all font-mono">https://api.telegram.org/bot&lt;YOUR_BOT_TOKEN&gt;/getUpdates</code>, replacing &lt;YOUR_BOT_TOKEN&gt; with the full token.
              <a href="https://core.telegram.org/bots/api#getupdates" target="_blank" rel="noopener noreferrer" class="text-accent-brand underline">Telegram getUpdates instructions</a>.
              getUpdates is unavailable while a webhook is registered; reuse your saved chat ID or obtain it from the incoming webhook payload.
            </li>
            <li>
              <strong class="text-secondary">Workspace ID:</strong> Open the workspace in this app. Its ID is the part of the address immediately after /workspaces/.
            </li>
          </ol>
        </details>
        <p class="m-0 text-[length:var(--text-small)] text-muted">
          {{ isEditing ? 'Click Apply changes' : 'Click Add connector' }}, then Save communication settings. If you entered a workspace, use the Inbound webhook panel's Public host and Register button to register with Telegram.
        </p>
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
    <TelegramSetupHelp v-model:open="showTelegramHelp" />
  </div>
</template>
