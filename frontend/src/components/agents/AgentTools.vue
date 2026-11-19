<script setup lang="ts">
import { computed, ref } from "vue"
import Panel from "../common/layout/Panel.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Callout from "../common/feedback/Callout.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import McpServerCard from "./McpServerCard.vue"
import McpServerForm from "./McpServerForm.vue"
import { NEW_ENTRY, useMcpServers } from "../../composables/agents/useMcpServers"
import { useConfirm } from "../../composables/ui/useConfirm"
import { useToast } from "../../composables/useToast"
import {
  MCP_ADD_LABEL, MCP_EMPTY_BODY, MCP_EMPTY_TITLE, MCP_LOAD_FAILED_NEXT, MCP_LOAD_FAILED_TITLE, MCP_OFF_BODY, MCP_OFF_LINK, MCP_OFF_TITLE, MCP_REMOVE, MCP_REMOVE_MESSAGE,
  MCP_TAB_INTRO, blankMcpForm, mcpAddedToast, mcpRemovedToast, mcpRemoveTitle, mcpSavedToast,
} from "../../constants/mcp"
import { toAgents } from "../../router/routes"
import type { McpServer, McpServerForm as ServerForm } from "../../types/mcp"

// Agents · Tools: the MCP servers the agents may use (ADR-0069). The page only controls; what the agents did with a tool is in the run's steps and the
// Agent activity card. Every change here needs the passphrase (the shared client asks) and takes effect at once, with no Save for the card's own switches.
const m = useMcpServers()
const { confirm } = useConfirm()
const toast = useToast()

const ADD = "add" as const
// Which form is open: a server's name (editing it), ADD for the new-server form, or nothing.
const editing = ref<string | typeof ADD | null>(null)
const form = ref<ServerForm>(blankMcpForm())

const servers = computed(() => m.answer.value?.servers ?? [])
const empty = computed(() => m.answer.value !== null && servers.value.length === 0 && editing.value !== ADD)
const formKey = computed(() => (editing.value === ADD ? NEW_ENTRY : editing.value ?? ""))
const currentServer = computed(() => servers.value.find((s) => s.name === editing.value))

function openAdd(): void {
  m.actionError.value = ""
  m.clearTest(NEW_ENTRY)
  form.value = blankMcpForm()
  editing.value = ADD
}

function openEdit(server: McpServer): void {
  m.actionError.value = ""
  m.clearTest(server.name)
  form.value = { name: server.name, url: server.url, trust: server.trust, auth_kind: server.auth_kind, auth_header: server.auth_header ?? "", secret: "", ca_cert: server.ca_cert ?? "" }
  editing.value = server.name
}

function close(): void {
  m.clearTest(formKey.value)
  m.actionError.value = ""
  editing.value = null
}

async function submit(): Promise<void> {
  const adding = editing.value === ADD
  const saved = adding ? await m.add(form.value) : currentServer.value ? await m.save(form.value, currentServer.value) : false
  if (!saved) return
  toast.success((adding ? mcpAddedToast : mcpSavedToast)(form.value.name))
  editing.value = null
}

async function askRemove(server: McpServer): Promise<void> {
  const sure = await confirm({ title: mcpRemoveTitle(server.name), message: MCP_REMOVE_MESSAGE, type: "warning", confirmText: MCP_REMOVE })
  if (!sure) return
  if (await m.remove(server.name)) toast.success(mcpRemovedToast(server.name))
  else toast.error(m.actionError.value)
}

// A card's own change (its switch, its trust, a tool) reports a refusal in the page, where the person is looking.
async function quietly(change: Promise<boolean>): Promise<void> {
  if (!(await change)) toast.error(m.actionError.value)
}
</script>

<template>
  <div class="flex flex-col gap-4" data-test="agent-tools">
    <Callout v-if="m.answer.value && !m.answer.value.enabled" tone="warning" :title="MCP_OFF_TITLE" data-test="mcp-off">
      {{ MCP_OFF_BODY }}
      <template #actions><RouterLink :to="toAgents('modules')" class="text-[length:var(--text-small)] text-accent-info-text underline">{{ MCP_OFF_LINK }}</RouterLink></template>
    </Callout>

    <p class="m-0 max-w-[68ch] text-muted">{{ MCP_TAB_INTRO }}</p>

    <Panel v-if="editing === ADD" :title="MCP_ADD_LABEL">
      <McpServerForm
        v-model="form" :saving="m.saving.value" :testing="m.testing.value === NEW_ENTRY" :test="m.tests.value[NEW_ENTRY]" :error="m.actionError.value"
        @save="submit" @cancel="close" @test="m.testForm(form)"
      />
    </Panel>

    <LoadingState v-if="m.loading.value" label="Reading the MCP servers" :rows="2" />
    <ErrorState v-else-if="m.error.value" :title="MCP_LOAD_FAILED_TITLE" :cause="m.error.value" :next="MCP_LOAD_FAILED_NEXT" />
    <EmptyState v-else-if="empty" :title="MCP_EMPTY_TITLE" :body="MCP_EMPTY_BODY">
      <template #action><BaseButton icon="plus" data-test="mcp-add-empty" @click="openAdd">{{ MCP_ADD_LABEL }}</BaseButton></template>
    </EmptyState>
    <Panel v-else-if="servers.length" title="MCP servers">
      <template #actions>
        <BaseButton size="sm" icon="plus" :disabled="editing === ADD" data-test="mcp-add" @click="openAdd">{{ MCP_ADD_LABEL }}</BaseButton>
      </template>
      <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0" aria-label="MCP servers">
        <li v-for="server in servers" :key="server.name" class="py-4 first:pt-0 last:pb-0">
          <McpServerForm
            v-if="editing === server.name"
            v-model="form" :current="server" :saving="m.saving.value" :testing="m.testing.value === server.name" :test="m.tests.value[server.name]" :error="m.actionError.value"
            @save="submit" @cancel="close" @test="m.testForm(form, server)"
          />
          <McpServerCard
            v-else
            :server="server" :busy="m.saving.value" :testing="m.testing.value === server.name" :test="m.tests.value[server.name]"
            @edit="openEdit(server)" @remove="askRemove(server)" @test="m.testSaved(server.name)"
            @enable="quietly(m.setEnabled(server, $event))" @trust="quietly(m.setTrust(server, $event))" @tool="(name, off) => quietly(m.setToolOff(server, name, off))"
          />
        </li>
      </ul>
    </Panel>
  </div>
</template>
