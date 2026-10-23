<script setup lang="ts">
import { computed, watch } from "vue"
import { useAgentsEditor } from "../../composables/settings/useAgentsEditor"
import AgentForm from "./AgentForm.vue"
import AgentList from "./AgentList.vue"
import EmptyState from "../common/feedback/EmptyState.vue"
import ErrorState from "../common/feedback/ErrorState.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import BaseButton from "../common/buttons/BaseButton.vue"

// Settings · Agents: the list beside the open agent on a wide screen; on a phone one at a time, with a way back.
// Saves go straight to the server (each agent is its own file), so this section has no shared Save; its unsaved
// edits are reported to the page's guard.
const EMPTY_TITLE = "Pick an agent to read it"
const EMPTY_BODY = "Built-in agents are read-only; duplicate one to change it, or start a new one."
const emit = defineEmits<{ (e: "dirty-change", dirty: boolean): void }>()

const editor = useAgentsEditor()
const { profiles, schema, detail, creating, draft, newId, readOnly, dirty, saving, loading, fieldErrors, formError } = editor

watch(dirty, (value) => emit("dirty-change", value), { immediate: true })

const helperOptions = computed(() =>
  profiles.value.filter((p) => p.id !== editor.currentId.value).map((p) => ({ value: p.id, label: p.name })),
)
const selectedId = computed(() => detail.value?.id ?? "")
</script>

<template>
  <ErrorState v-if="editor.listError.value" title="The agents could not be read" :cause="editor.listError.value" next="Check that the server is running, then retry.">
    <template #action><BaseButton variant="secondary" icon="refresh" @click="editor.reload">Retry</BaseButton></template>
  </ErrorState>
  <div v-else class="grid min-w-0 gap-6 lg:grid-cols-[minmax(15rem,20rem)_minmax(0,1fr)]">
    <div :class="editor.open.value ? 'hidden lg:block' : ''">
      <AgentList :profiles="profiles" :selected-id="selectedId" :creating="creating" @select="editor.select" @new="editor.startNew" />
    </div>
    <div :class="editor.open.value ? '' : 'hidden lg:block'" class="min-w-0">
      <LoadingState v-if="loading" label="Loading the agent" />
      <ErrorState v-else-if="editor.loadError.value" title="The agent could not be read" :cause="editor.loadError.value" next="Pick it again from the list." />
      <ErrorState v-else-if="editor.schemaError.value" title="The agent settings could not be read" :cause="editor.schemaError.value" next="Reload the page, then try again." />
      <AgentForm
        v-else-if="editor.open.value"
        :schema="schema"
        :draft="draft"
        :detail="detail"
        :creating="creating"
        :new-id="newId"
        :read-only="readOnly"
        :dirty="dirty"
        :saving="saving"
        :field-errors="fieldErrors"
        :form-error="formError"
        :helper-options="helperOptions"
        @update="editor.setField"
        @update:new-id="editor.setId"
        @save="editor.save"
        @discard="editor.discard"
        @remove="editor.remove"
        @duplicate="editor.duplicate"
        @back="editor.close"
      />
      <EmptyState v-else :title="EMPTY_TITLE" :body="EMPTY_BODY" />
    </div>
  </div>
</template>
