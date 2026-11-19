<script setup lang="ts">
import { computed, ref, useId } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import BaseToggle from "../common/buttons/BaseToggle.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import StatusTag from "../common/display/StatusTag.vue"
import {
  MCP_EDIT, MCP_NO_TOOLS, MCP_PROBLEM_ADVICE, MCP_REMOVE, MCP_STATUS_LABELS, MCP_STATUS_STATE, MCP_TEST_LABEL, MCP_TEST_RESULT_OK, MCP_TOOL_OFF_HINT, MCP_TOOLS_HIDE,
  MCP_TOOLS_SHOW, MCP_TRUST_HINTS, MCP_TRUST_LABEL, MCP_TRUST_LABELS, mcpToolCount,
} from "../../constants/mcp"
import { MCP_TRUSTS } from "../../types/mcp"
import type { McpProblem, McpServer, McpTrust } from "../../types/mcp"
import type { ChoiceOption } from "../../types/ui"

// One MCP server: where it is, how its connection is doing in words with the next step, how its tools are trusted, and the tools it offers with a switch
// each. Every change is made on the spot (no Save), through the page; the card only reports what the person did.
const props = defineProps<{ server: McpServer; busy: boolean; testing: boolean; test?: McpServer }>()
const emit = defineEmits<{
  (e: "edit"): void
  (e: "remove"): void
  (e: "test"): void
  (e: "enable", on: boolean): void
  (e: "trust", trust: McpTrust): void
  (e: "tool", name: string, off: boolean): void
}>()

const TRUST_OPTIONS: ChoiceOption[] = MCP_TRUSTS.map((value) => ({ value, label: MCP_TRUST_LABELS[value] }))
const toolsId = useId()
const showTools = ref(false)

const TRANSPORT_WORDS = { streamable_http: "streamable HTTP", sse: "the older SSE transport" } as const
const state = computed(() => props.server.status)
const advice = computed(() => (props.server.problem_code ? MCP_PROBLEM_ADVICE[props.server.problem_code as McpProblem] : undefined))
const offered = computed(() => props.server.tools.filter((t) => !t.disabled).length)
const detail = computed(() => {
  const s = props.server
  const parts: string[] = []
  if (s.transport) parts.push(`over ${TRANSPORT_WORDS[s.transport]}`)
  parts.push(mcpToolCount(offered.value) + (offered.value === s.tools.length ? "" : ` of ${s.tools.length}`))
  if (s.server_name) parts.push(`${s.server_name}${s.server_version ? " " + s.server_version : ""}`)
  return parts.join(" · ")
})
const testProblem = computed(() => (props.test?.problem_code ? MCP_PROBLEM_ADVICE[props.test.problem_code as McpProblem] : undefined))
</script>

<template>
  <article class="flex flex-col gap-3" :data-test="'mcp-server-' + server.name" :aria-label="server.name">
    <header class="flex flex-wrap items-center gap-x-3 gap-y-2">
      <h3 class="m-0 font-mono text-[length:var(--text-body)] font-semibold text-primary">{{ server.name }}</h3>
      <StatusTag :state="MCP_STATUS_STATE[state]" :label="MCP_STATUS_LABELS[state]" data-test="mcp-status" />
      <span class="ml-auto flex flex-wrap items-center gap-2">
        <BaseToggle :model-value="server.enabled" :label="'Switch ' + server.name + ' on or off'" hide-label class="max-sm:min-h-11" :disabled="busy" data-test="mcp-enable" @update:model-value="emit('enable', $event)" />
        <BaseButton variant="ghost" size="sm" icon="edit" icon-only class-name="max-sm:!h-11 max-sm:!w-11" :label="MCP_EDIT + ' ' + server.name" :disabled="busy" data-test="mcp-edit" @click="emit('edit')" />
        <BaseButton variant="ghost" size="sm" icon="trash" icon-only class-name="max-sm:!h-11 max-sm:!w-11" :label="MCP_REMOVE + ' ' + server.name" :disabled="busy" data-test="mcp-remove" @click="emit('remove')" />
      </span>
    </header>

    <p class="m-0 break-all font-mono text-[length:var(--text-small)] text-muted">{{ server.url }}</p>

    <div class="flex flex-col gap-1" data-test="mcp-state">
      <p v-if="state === 'ready'" class="m-0 text-[length:var(--text-small)] text-secondary">{{ detail }}</p>
      <p v-else-if="state === 'connecting'" class="m-0 text-[length:var(--text-small)] text-secondary">Connecting to the server.</p>
      <p v-else-if="state === 'off'" class="m-0 text-[length:var(--text-small)] text-muted">Switched off: nothing is sent to this server.</p>
      <template v-else>
        <p class="m-0 text-[length:var(--text-small)] text-state-error">{{ server.problem }}</p>
        <p v-if="advice" class="m-0 text-[length:var(--text-small)] text-muted">{{ advice }}</p>
      </template>
      <p v-if="state === 'ready' && server.problem" class="m-0 text-[length:var(--text-small)] text-muted">{{ server.problem }}. {{ advice }}</p>
    </div>

    <div class="flex flex-col gap-1.5">
      <span class="text-[length:var(--text-small)] font-medium text-secondary">{{ MCP_TRUST_LABEL }}</span>
      <SegmentedControl :model-value="server.trust" :options="TRUST_OPTIONS" :label="MCP_TRUST_LABEL + ' (' + server.name + ')'" variant="compact" @update:model-value="emit('trust', $event as McpTrust)" />
      <p class="m-0 text-[length:var(--text-small)] text-faint">{{ MCP_TRUST_HINTS[server.trust] }}</p>
    </div>

    <div class="flex flex-wrap items-center gap-2">
      <BaseButton variant="secondary" size="sm" class-name="max-sm:!h-11" :loading="testing" :disabled="busy || state === 'off'" data-test="mcp-test" @click="emit('test')">{{ MCP_TEST_LABEL }}</BaseButton>
      <BaseButton v-if="server.tools.length" variant="ghost" size="sm" class-name="max-sm:!h-11" :aria-expanded="showTools" :aria-controls="toolsId" data-test="mcp-tools-toggle" @click="showTools = !showTools">
        {{ showTools ? MCP_TOOLS_HIDE : MCP_TOOLS_SHOW }} ({{ server.tools.length }})
      </BaseButton>
    </div>

    <div v-if="test" role="status" class="flex flex-col gap-1" data-test="mcp-test-result">
      <p class="m-0 flex flex-wrap items-center gap-2 text-[length:var(--text-small)]">
        <StatusTag :state="MCP_STATUS_STATE[test.status]" :label="MCP_STATUS_LABELS[test.status]" />
        <span class="text-secondary">{{ test.status === "ready" ? MCP_TEST_RESULT_OK + " " + mcpToolCount(test.tools.length) + "." : test.problem }}</span>
      </p>
      <p v-if="testProblem" class="m-0 text-[length:var(--text-small)] text-muted">{{ testProblem }}</p>
    </div>

    <div v-if="showTools" :id="toolsId" class="flex flex-col gap-1 rounded-[var(--radius-md)] bg-canvas p-2" data-test="mcp-tools">
      <p class="m-0 px-1 text-[length:var(--text-small)] text-faint">{{ MCP_TOOL_OFF_HINT }}</p>
      <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
        <li v-for="tool in server.tools" :key="tool.name" class="flex items-start gap-3 px-1 py-2" :data-test="'mcp-tool-' + tool.name">
          <BaseToggle :model-value="!tool.disabled" :label="'Offer ' + tool.name" hide-label class="max-sm:min-h-11" :disabled="busy" @update:model-value="emit('tool', tool.name, !$event)" />
          <span class="flex min-w-0 flex-col gap-0.5">
            <span class="break-all font-mono text-[length:var(--text-small)] text-primary">{{ tool.name }}</span>
            <span v-if="tool.description" class="line-clamp-2 text-[length:var(--text-small)] text-muted">{{ tool.description }}</span>
          </span>
        </li>
        <li v-if="!server.tools.length" class="px-1 py-2 text-[length:var(--text-small)] text-muted">{{ MCP_NO_TOOLS }}</li>
      </ul>
    </div>
  </article>
</template>
