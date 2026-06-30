<script setup lang="ts">
import { ref, computed, onMounted } from "vue"
import type { GlobalConfig, ConnectorConfig } from "../../types/admin"
import { AdminApiService } from "../../services/adminService"
import BaseButton from "../common/buttons/BaseButton.vue"
import CopyButton from "../common/display/CopyButton.vue"
import Icon from "../icons/Icon.vue"

const props = defineProps<{
  editConfig: GlobalConfig
}>()

const emit = defineEmits<{
  (e: "update:editConfig", config: GlobalConfig): void
  (e: "updateConfig"): void
}>()

const connectors = computed({
  get: () => props.editConfig.communication?.connectors ?? {},
  set: (val: Record<string, ConnectorConfig>) => {
    const clone = JSON.parse(JSON.stringify(props.editConfig))
    if (!clone.communication) clone.communication = { connectors: {} }
    clone.communication.connectors = val
    emit("update:editConfig", clone)
  }
})

interface ConnectorForm { name: string; type: string; chat_id: string; workspace_id: string; token: string; webhook_token: string }
const showForm = ref(false)
const editingName = ref<string | null>(null)
const form = ref<ConnectorForm>({ name: "", type: "telegram", chat_id: "", workspace_id: "", token: "", webhook_token: "" })

const connectorTokens = ref<Record<string, { masked: string; dirty: string | null }>>({})
const saveError = ref("")
const webhookBaseUrl = ref(window.location.origin + "/api/v1/webhooks/")

const isEditing = computed(() => editingName.value !== null)
const formTitle = computed(() => isEditing.value ? `Edit: ${editingName.value}` : "Add Connector")
const submitLabel = computed(() => isEditing.value ? "Save Changes" : "Add")

onMounted(async () => {
  for (const name of Object.keys(connectors.value)) {
    await loadSecret(name)
  }
})

async function loadSecret(name: string) {
  try {
    const masked = await AdminApiService.fetchToolSecret("connector", name)
    connectorTokens.value[name] = { masked, dirty: null }
  } catch {
    connectorTokens.value[name] = { masked: "", dirty: null }
  }
}

function ensureTokenTracked(name: string) {
  if (!connectorTokens.value[name]) {
    connectorTokens.value[name] = { masked: "", dirty: null }
  }
}

function resetForm() {
  showForm.value = false
  editingName.value = null
  form.value = { name: "", type: "telegram", chat_id: "", workspace_id: "", token: "", webhook_token: "" }
}

function editConnector(name: string) {
  const cfg = connectors.value[name]
  if (!cfg) return
  editingName.value = name
  const tok = connectorTokens.value[name]
  form.value = {
    name,
    type: cfg.type,
    chat_id: cfg.settings?.chat_id ?? "",
    workspace_id: cfg.settings?.workspace_id ?? "",
    token: tok?.masked ?? "",
    webhook_token: "",
  }
  showForm.value = true
}

function saveConnector() {
  const name = form.value.name.trim()
  if (!name) return

  const settings: Record<string, string> = {}
  if (form.value.chat_id) settings.chat_id = form.value.chat_id
  if (form.value.workspace_id) settings.workspace_id = form.value.workspace_id
  if (form.value.webhook_token) settings.webhook_token = form.value.webhook_token

  const updated = isEditing.value ? { ...connectors.value } : { ...connectors.value }

  if (!isEditing.value) {
    updated[name] = {
      type: form.value.type,
      enabled: true,
      settings,
      secret_ref: form.value.token ? name : undefined,
    }
    connectors.value = updated
    ensureTokenTracked(name)
    if (form.value.token) {
      connectorTokens.value[name]!.dirty = form.value.token
    }
  } else {
    const existing = updated[name]
    if (existing) {
      updated[name] = {
        ...existing,
        settings,
      }
      const currentTok = connectorTokens.value[name]
      if (form.value.token && form.value.token !== currentTok?.masked) {
        ensureTokenTracked(name)
        connectorTokens.value[name]!.dirty = form.value.token
      }
    }
    connectors.value = updated
  }

  resetForm()
}

async function removeConnector(name: string) {
  const updated = { ...connectors.value }
  delete updated[name]
  connectors.value = updated
  delete connectorTokens.value[name]
  try {
    await AdminApiService.deleteToolSecret("connector", name)
  } catch {
    // secret may not exist — that's fine
  }
}

function toggleConnector(name: string) {
  const updated = { ...connectors.value }
  if (updated[name]) updated[name] = { ...updated[name], enabled: !updated[name].enabled }
  connectors.value = updated
}

function cancelForm() {
  resetForm()
}

async function save() {
  saveError.value = ""
  for (const [name, _cfg] of Object.entries(connectors.value)) {
    const tok = connectorTokens.value[name]
    if (tok?.dirty) {
      try {
        await AdminApiService.saveToolSecret("connector", name, tok.dirty)
        tok.masked = tok.dirty.slice(0, 4) + "..."
        tok.dirty = null
      } catch (err) {
        saveError.value = `Failed to save token for "${name}": ${err}`
        return
      }
    }
  }
  emit("updateConfig")
}
</script>

<template>
  <div class="settings-container">
    <h2 class="settings-title">Communication Connectors</h2>
    <div class="form-helper mb-4">
      Configure external platforms the agent can use to send notifications and reports.
    </div>

    <div v-if="Object.keys(connectors).length === 0" class="empty-state">
      No connectors configured. Add a Telegram, Slack, or other connector below.
    </div>

    <div v-for="(cfg, name) in connectors" :key="name" class="connector-row">
      <div class="connector-info">
        <span class="connector-name">{{ name }}</span>
        <span class="connector-type">{{ cfg.type }}</span>
      </div>
      <div class="connector-actions">
        <label class="toggle-row">
          <input type="checkbox" :checked="cfg.enabled" @change="toggleConnector(name)" />
        </label>
        <button @click="editConnector(name)" class="btn-edit" title="Edit connector">
          <Icon name="edit" size="xs" />
        </button>
        <button @click="removeConnector(name)" class="btn-remove" title="Remove connector">
          <Icon name="trash" size="xs" />
        </button>
      </div>
    </div>

    <div v-for="(cfg, name) in connectors" :key="'url-'+name" v-show="cfg.settings?.workspace_id && !showForm" class="webhook-url-row">
      <span class="webhook-label">Webhook URL:</span>
      <code class="webhook-url">{{ webhookBaseUrl }}{{ name }}</code>
      <CopyButton :text="`${webhookBaseUrl}${name}`" iconSize="sm" />
    </div>

    <button v-if="!showForm" @click="showForm = true" class="btn-add">
      + Add Connector
    </button>

    <div v-if="showForm" class="add-form">
      <h3 class="form-title">{{ formTitle }}</h3>
      <input
        v-model="form.name"
        placeholder="Connector name (e.g. my-telegram)"
        class="form-input"
        :readonly="isEditing"
        :class="{ 'form-input--readonly': isEditing }"
      />
      <select
        v-model="form.type"
        class="form-input"
        :disabled="isEditing"
      >
        <option value="telegram">Telegram</option>
      </select>
      <input v-model="form.chat_id" placeholder="Chat ID (telegram)" class="form-input" />
      <input v-model="form.workspace_id" placeholder="Workspace ID (for inbound)" class="form-input" />
      <input v-model="form.token" type="password" :placeholder="isEditing ? 'New bot token (leave empty to keep)' : 'Bot token'" class="form-input" />
      <input v-model="form.webhook_token" type="password" placeholder="Webhook secret token (for inbound)" class="form-input" />
      <div class="form-actions">
        <BaseButton variant="primary" @click="saveConnector">{{ submitLabel }}</BaseButton>
        <BaseButton variant="secondary" @click="cancelForm">Cancel</BaseButton>
      </div>
    </div>

    <div v-if="!showForm" class="setup-help">
      <h4 class="help-title">Setup Instructions</h4>
      <p class="help-text">
        After configuring a connector with a Workspace ID and Webhook Secret Token, register the webhook URL with your platform:
      </p>
      <pre class="help-code">curl -X POST "https://api.telegram.org/bot&lt;TOKEN&gt;/setWebhook?url=&lt;WEBHOOK_URL&gt;&secret_token=&lt;YOUR_SECRET&gt;"</pre>
      <p class="help-text">
        The platform will send incoming messages to your webhook URL. Messages are delivered to the workspace's active session.
      </p>
    </div>

    <div class="save-bar">
      <BaseButton variant="primary" icon="play" @click="save">Save Connector Settings</BaseButton>
    </div>
    <p v-if="saveError" class="save-error">{{ saveError }}</p>
  </div>
</template>

<style scoped lang="postcss">
.settings-container {
  @apply bg-gray-800 rounded-lg shadow-xl border border-gray-700 p-6 space-y-4;
}
.settings-title {
  @apply text-xl font-bold text-white mb-2;
}
.form-helper {
  @apply text-xs text-gray-500;
}
.empty-state {
  @apply text-sm text-gray-500 italic py-4 text-center;
}
.connector-row {
  @apply flex items-center justify-between py-2 px-3 bg-gray-900 rounded border border-gray-700;
}
.connector-info {
  @apply flex items-center gap-2;
}
.connector-name {
  @apply text-sm font-medium text-gray-200;
}
.connector-type {
  @apply text-[10px] uppercase tracking-wider text-blue-400 bg-blue-500/10 px-2 py-0.5 rounded;
}
.connector-actions {
  @apply flex items-center gap-1;
}
.toggle-row {
  @apply flex items-center;
}
.toggle-row input {
  @apply w-4 h-4;
}
.btn-edit {
  @apply p-1 hover:bg-blue-500/15 text-gray-500 hover:text-blue-400 rounded transition-colors;
}
.btn-remove {
  @apply p-1 hover:bg-red-500/15 text-gray-500 hover:text-red-400 rounded transition-colors;
}
.webhook-url-row {
  @apply flex items-center gap-2 py-1.5 px-3 bg-gray-900/60 rounded border border-gray-700/50 text-xs;
}
.webhook-label {
  @apply text-gray-400 shrink-0;
}
.webhook-url {
  @apply text-blue-300 font-mono truncate flex-1;
}
.btn-add {
  @apply text-sm text-blue-400 hover:text-blue-300 py-2;
}
.add-form {
  @apply space-y-3 p-4 bg-gray-900 rounded border border-gray-700;
}
.form-title {
  @apply text-sm font-semibold text-gray-200;
}
.form-input {
  @apply w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-sm text-white;
}
.form-input--readonly {
  @apply text-gray-500 cursor-not-allowed;
}
.form-actions {
  @apply flex gap-2;
}
.save-bar {
  @apply pt-4 border-t border-gray-700 flex justify-end items-center gap-3;
}
.save-error {
  @apply text-xs text-red-400;
}
.setup-help {
  @apply p-4 bg-gray-900/50 rounded border border-gray-700/50 space-y-2;
}
.help-title {
  @apply text-xs font-bold text-gray-300 uppercase tracking-wider;
}
.help-text {
  @apply text-xs text-gray-400 leading-relaxed;
}
.help-code {
  @apply text-xs text-blue-300 font-mono bg-gray-950 p-2 rounded overflow-x-auto whitespace-pre-wrap;
}
</style>
