<script setup lang="ts">
import { computed } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import FormField from "../common/forms/FormField.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import StatusTag from "../common/display/StatusTag.vue"
import {
  MCP_ADVANCED_LABEL, MCP_AUTH_LABEL, MCP_AUTH_LABELS, MCP_CANCEL, MCP_FIELD_CA, MCP_FIELD_CA_HINT, MCP_FIELD_HEADER, MCP_FIELD_NAME, MCP_FIELD_NAME_HINT,
  MCP_FIELD_SECRET_BEARER, MCP_FIELD_SECRET_HEADER, MCP_FIELD_URL, MCP_FIELD_URL_HINT, MCP_HOST_CHANGED_HINT, MCP_PROBLEM_ADVICE, MCP_SAVE_ADD, MCP_SAVE_EDIT,
  MCP_SECRET_KEEP_HINT, MCP_SECRET_NEW_HINT, MCP_SECRET_PLACEHOLDER_STORED, MCP_STATUS_LABELS, MCP_STATUS_STATE, MCP_TEST_LABEL, MCP_TEST_RESULT_OK,
  MCP_TRUST_HINTS, MCP_TRUST_LABEL, MCP_TRUST_LABELS, mcpToolCount,
} from "../../constants/mcp"
import { MCP_AUTH_KINDS, MCP_TRUSTS } from "../../types/mcp"
import type { McpAuthKind, McpProblem, McpServer, McpServerForm, McpTrust } from "../../types/mcp"
import type { ChoiceOption } from "../../types/ui"

// The add and edit form of one MCP server. The sign-in value is typed here and sent once: it is sealed by the server and never comes back, so on an
// edit an empty box keeps the stored one. A stored value goes only to the address it was given for, so a changed address asks for it again.
const props = defineProps<{
  modelValue: McpServerForm
  /** The server being edited; absent when adding. */
  current?: McpServer
  saving: boolean
  testing: boolean
  /** The last test of this entry, shown under the buttons. */
  test?: McpServer
  error: string
}>()
const emit = defineEmits<{
  (e: "update:modelValue", form: McpServerForm): void
  (e: "save"): void
  (e: "cancel"): void
  (e: "test"): void
}>()

const editing = computed(() => !!props.current)
const set = <K extends keyof McpServerForm>(key: K, value: McpServerForm[K]) => emit("update:modelValue", { ...props.modelValue, [key]: value })
const text = (key: "name" | "url" | "auth_header" | "secret" | "ca_cert") => computed({ get: () => props.modelValue[key], set: (v: string) => set(key, v) })
const name = text("name"), url = text("url"), header = text("auth_header"), secret = text("secret"), ca = text("ca_cert")

const AUTH_OPTIONS: ChoiceOption[] = MCP_AUTH_KINDS.map((value) => ({ value, label: MCP_AUTH_LABELS[value] }))
const TRUST_OPTIONS: ChoiceOption[] = MCP_TRUSTS.map((value) => ({ value, label: MCP_TRUST_LABELS[value] }))

const needsSecret = computed(() => props.modelValue.auth_kind !== "none")
const hostOf = (address: string): string => { try { return new URL(address).host.toLowerCase() } catch { return "" } }
const hostChanged = computed(() => editing.value && hostOf(props.modelValue.url) !== hostOf(props.current?.url ?? ""))
const kindChanged = computed(() => editing.value && props.modelValue.auth_kind !== props.current?.auth_kind)
// The value must be typed when there is none stored, or the stored one would go to another address or be used another way.
const secretRequired = computed(() => needsSecret.value && (!editing.value || !props.current?.has_secret || hostChanged.value || kindChanged.value))
const secretHint = computed(() => {
  if (hostChanged.value && needsSecret.value) return MCP_HOST_CHANGED_HINT
  return editing.value && props.current?.has_secret && !kindChanged.value ? MCP_SECRET_KEEP_HINT : MCP_SECRET_NEW_HINT
})
const secretLabel = computed(() => (props.modelValue.auth_kind === "header" ? MCP_FIELD_SECRET_HEADER : MCP_FIELD_SECRET_BEARER))
const canSave = computed(() => {
  const f = props.modelValue
  const complete = f.name.trim() !== "" && f.url.trim() !== ""
  const headerOk = f.auth_kind !== "header" || f.auth_header.trim() !== ""
  const secretOk = !secretRequired.value || f.secret.trim() !== ""
  return complete && headerOk && secretOk && !props.saving
})
// A test of an entry whose value is stored but not retyped cannot send it; the saved server has its own test button on its card.
const canTest = computed(() => !props.testing && props.modelValue.url.trim() !== "" && (!secretRequired.value || props.modelValue.secret.trim() !== ""))

const testProblem = computed(() => (props.test?.problem_code ? MCP_PROBLEM_ADVICE[props.test.problem_code as McpProblem] : undefined))
</script>

<template>
  <form class="flex flex-col gap-4" data-test="mcp-form" @submit.prevent="canSave && emit('save')">
    <div class="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
      <FormField :label="MCP_FIELD_NAME" :hint="MCP_FIELD_NAME_HINT">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model="name" type="text" class="form-control font-mono" placeholder="files" autocomplete="off" spellcheck="false" :disabled="editing" :aria-describedby="describedBy" data-test="mcp-name" />
        </template>
      </FormField>
      <FormField :label="MCP_FIELD_URL" :hint="MCP_FIELD_URL_HINT">
        <template #default="{ id, describedBy }">
          <input :id="id" v-model="url" type="url" class="form-control font-mono" placeholder="https://mcp.example.com/mcp" autocomplete="off" spellcheck="false" :aria-describedby="describedBy" data-test="mcp-url" />
        </template>
      </FormField>
    </div>

    <div class="flex flex-col gap-3">
      <div class="flex flex-col gap-1.5">
        <span class="text-[length:var(--text-small)] font-medium text-secondary">{{ MCP_AUTH_LABEL }}</span>
        <SegmentedControl :model-value="modelValue.auth_kind" :options="AUTH_OPTIONS" :label="MCP_AUTH_LABEL" variant="compact" @update:model-value="set('auth_kind', $event as McpAuthKind)" />
      </div>
      <div v-if="needsSecret" class="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
        <FormField v-if="modelValue.auth_kind === 'header'" :label="MCP_FIELD_HEADER">
          <template #default="{ id }">
            <input :id="id" v-model="header" type="text" class="form-control font-mono" placeholder="X-Api-Key" autocomplete="off" spellcheck="false" data-test="mcp-header" />
          </template>
        </FormField>
        <FormField :label="secretLabel" :hint="secretHint" :class="modelValue.auth_kind === 'header' ? '' : 'md:col-span-2'">
          <template #default="{ id, describedBy }">
            <input
              :id="id" v-model="secret" type="password" class="form-control font-mono" autocomplete="new-password" spellcheck="false"
              :placeholder="editing && current?.has_secret && !hostChanged && !kindChanged ? MCP_SECRET_PLACEHOLDER_STORED : ''" :aria-describedby="describedBy" data-test="mcp-secret"
            />
          </template>
        </FormField>
      </div>
    </div>

    <div class="flex flex-col gap-1.5">
      <span class="text-[length:var(--text-small)] font-medium text-secondary">{{ MCP_TRUST_LABEL }}</span>
      <SegmentedControl :model-value="modelValue.trust" :options="TRUST_OPTIONS" :label="MCP_TRUST_LABEL" variant="compact" @update:model-value="set('trust', $event as McpTrust)" />
      <p class="m-0 text-[length:var(--text-small)] text-faint" data-test="mcp-trust-hint">{{ MCP_TRUST_HINTS[modelValue.trust] }}</p>
    </div>

    <details class="group rounded-[var(--radius-md)] bg-canvas px-3 py-2" :open="!!modelValue.ca_cert">
      <summary class="flex min-h-9 cursor-pointer list-none items-center text-[length:var(--text-small)] font-medium text-secondary focus-visible:outline focus-visible:outline-2 focus-visible:outline-focus-ring">{{ MCP_ADVANCED_LABEL }}</summary>
      <div class="pb-2 pt-1">
        <FormField :label="MCP_FIELD_CA" :hint="MCP_FIELD_CA_HINT">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model="ca" type="text" class="form-control font-mono" placeholder="/etc/ssl/my-authority.pem" autocomplete="off" spellcheck="false" :aria-describedby="describedBy" data-test="mcp-ca" />
          </template>
        </FormField>
      </div>
    </details>

    <p v-if="error" role="alert" class="m-0 text-[length:var(--text-small)] text-state-error" data-test="mcp-form-error">{{ error }}</p>

    <div v-if="test" role="status" class="flex flex-col gap-1" data-test="mcp-form-test">
      <p class="m-0 flex flex-wrap items-center gap-2 text-[length:var(--text-small)]">
        <StatusTag :state="MCP_STATUS_STATE[test.status]" :label="MCP_STATUS_LABELS[test.status]" />
        <span v-if="test.status === 'ready'" class="text-secondary">{{ MCP_TEST_RESULT_OK }} {{ mcpToolCount(test.tools.length) }}<template v-if="test.server_name"> from {{ test.server_name }}</template>.</span>
        <span v-else class="text-secondary">{{ test.problem }}</span>
      </p>
      <p v-if="testProblem" class="m-0 text-[length:var(--text-small)] text-muted">{{ testProblem }}</p>
    </div>

    <div class="flex flex-wrap items-center gap-2">
      <BaseButton type="submit" variant="primary" class-name="max-sm:!h-11" :loading="saving" :disabled="!canSave" data-test="mcp-save">{{ editing ? MCP_SAVE_EDIT : MCP_SAVE_ADD }}</BaseButton>
      <BaseButton variant="secondary" class-name="max-sm:!h-11" :loading="testing" :disabled="!canTest" data-test="mcp-test-form" @click="emit('test')">{{ MCP_TEST_LABEL }}</BaseButton>
      <BaseButton variant="ghost" class-name="max-sm:!h-11" data-test="mcp-cancel" @click="emit('cancel')">{{ MCP_CANCEL }}</BaseButton>
    </div>
  </form>
</template>
