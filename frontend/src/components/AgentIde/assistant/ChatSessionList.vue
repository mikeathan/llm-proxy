<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import type { SessionBrief } from '../../../types/assistant'
import { formatAbsoluteTime, formatRelativeTime } from '../../../utils/format/time'
import { buildSessionSections } from '../../../utils/assistant/sessionSections'
import { sourceIcon } from '../../../utils/assistant/source'
import { usePinnedSessions } from '../../../composables/assistant/usePinnedSessions'
import BaseButton from '../../common/buttons/BaseButton.vue'
import MicroLabel from '../../common/display/MicroLabel.vue'
import StatusTag from '../../common/display/StatusTag.vue'
import SearchInput from '../../common/forms/SearchInput.vue'
import Icon from '../../icons/Icon.vue'

// The workspace's conversations (plan D14): searchable by title, pinnable
// (kept per workspace in this browser), grouped Pinned · Today · Yesterday ·
// This week · Older, with webhook conversations in their own folder. Row
// actions are named after their conversation and show on hover or keyboard
// focus (always on touch-sized screens). Deleting is confirmed by the chat.
const props = defineProps<{
  sessions: SessionBrief[]
  currentSessionId: string | null
  workspaceId: string
  isMobile?: boolean
}>()

const emit = defineEmits<{
  (e: 'load', sessionId: string): void
  (e: 'delete', sessionId: string): void
  (e: 'rename', sessionId: string, title: string): void
  (e: 'cancel', sessionId: string): void
  (e: 'new-chat'): void
  (e: 'clear-all'): void
  (e: 'delete-group', ids: string[]): void
  (e: 'close'): void
}>()

const UNTITLED = 'Empty conversation'

const query = ref('')
const { pinned, toggle: togglePin } = usePinnedSessions(() => props.workspaceId)
const sections = computed(() => buildSessionSections(props.sessions, { pinned: pinned.value, query: query.value, now: Date.now() }))
const titleOf = (session: SessionBrief) => session.snippet || UNTITLED

const collapsed = ref(new Set<string>())
function toggleSection(key: string) {
  const next = new Set(collapsed.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  collapsed.value = next
}

const renaming = ref<string | null>(null)
const renameInput = ref('')
const renameField = ref<HTMLInputElement[] | null>(null)

async function startRename(session: SessionBrief) {
  renaming.value = session.id
  renameInput.value = session.snippet
  await nextTick()
  renameField.value?.[0]?.focus()
}

function confirmRename() {
  if (renaming.value && renameInput.value.trim()) emit('rename', renaming.value, renameInput.value.trim())
  renaming.value = null
}

const cancelRename = () => {
  renaming.value = null
}
</script>

<template>
  <div class="flex h-full w-full flex-col overflow-hidden border-r border-hairline bg-surface">
    <div class="flex flex-none items-center justify-between gap-2 border-b border-hairline px-3 py-2">
      <MicroLabel>Conversations</MicroLabel>
      <span class="flex items-center gap-1">
        <BaseButton v-if="sessions.length" variant="ghost" size="sm" icon="trash" icon-only label="Delete all conversations" @click="emit('clear-all')" />
        <BaseButton variant="ghost" size="sm" icon="plus" icon-only label="New chat" @click="emit('new-chat')" />
        <BaseButton v-if="isMobile" variant="ghost" size="sm" icon="close" icon-only label="Close conversations" @click="emit('close')" />
      </span>
    </div>

    <p v-if="!sessions.length" class="m-0 px-3 py-6 text-center text-[length:var(--text-small)] text-muted">
      No conversations yet. Ask the assistant something to start one.
    </p>

    <template v-else>
      <div class="flex-none px-3 py-2">
        <SearchInput v-model="query" label="Search conversations" />
      </div>
      <p v-if="!sections.length" class="m-0 px-3 py-4 text-center text-[length:var(--text-small)] text-muted">
        No conversation matches “{{ query.trim() }}”.
      </p>

      <div class="min-h-0 flex-1 overflow-y-auto pb-2">
        <section v-for="section in sections" :key="section.key" :aria-label="section.label">
          <div data-test="session-group" class="flex items-center gap-1 px-3 pb-1 pt-3">
            <button
              v-if="section.kind === 'source'"
              type="button"
              :aria-expanded="!collapsed.has(section.key)"
              class="flex min-w-0 flex-1 items-center gap-1 rounded-[var(--radius-sm)] text-left focus-visible:outline-none focus-visible:ring-2"
              @click="toggleSection(section.key)"
            >
              <Icon :name="collapsed.has(section.key) ? 'chevron-right' : 'chevron-down'" size="xs" />
              <MicroLabel>{{ section.label }}</MicroLabel>
            </button>
            <MicroLabel v-else class="flex-1">{{ section.label }}</MicroLabel>
            <BaseButton
              v-if="section.kind === 'source'"
              variant="ghost"
              size="sm"
              icon="trash"
              icon-only
              :label="`Delete all ${section.label.toLowerCase()} conversations`"
              @click="emit('delete-group', section.sessions.map((s) => s.id))"
            />
          </div>

          <ul v-show="!collapsed.has(section.key)" class="m-0 list-none p-0">
            <li
              v-for="session in section.sessions"
              :key="session.id"
              :class="[
                'session-row group relative flex min-w-0 items-center gap-1 border-l-2 px-2 py-1.5',
                currentSessionId === session.id ? 'border-accent-brand bg-surface-active' : 'border-transparent hover:bg-surface-hover',
              ]"
            >
              <input
                v-if="renaming === session.id"
                ref="renameField"
                v-model="renameInput"
                aria-label="Conversation title"
                class="form-control flex-1"
                @keydown.enter="confirmRename"
                @keydown.escape="cancelRename"
                @blur="confirmRename"
              />
              <button
                v-else
                type="button"
                :aria-current="currentSessionId === session.id ? 'true' : undefined"
                class="flex min-w-0 flex-1 flex-col gap-0.5 rounded-[var(--radius-sm)] px-1 py-0.5 text-left focus-visible:outline-none focus-visible:ring-2"
                @click="emit('load', session.id)"
              >
                <span class="flex min-w-0 items-center gap-1.5">
                  <Icon v-if="sourceIcon(session.source)" :name="sourceIcon(session.source)!" size="xs" class-name="flex-none text-muted" />
                  <span class="truncate text-[length:var(--text-small)] text-primary">{{ titleOf(session) }}</span>
                </span>
                <span class="flex items-center gap-2">
                  <StatusTag v-if="session.running" state="running" label="Running" />
                  <time
                    v-if="session.updated_at"
                    :datetime="session.updated_at"
                    :title="formatAbsoluteTime(session.updated_at)"
                    class="font-mono text-[length:var(--text-micro)] text-faint"
                  >{{ formatRelativeTime(session.updated_at) }}</time>
                </span>
              </button>

              <span
                v-if="renaming !== session.id"
                :class="['session-actions flex-none items-center', isMobile ? 'flex' : 'hidden group-hover:flex group-focus-within:flex']"
              >
                <BaseButton
                  variant="ghost"
                  size="sm"
                  :icon="pinned.has(session.id) ? 'arrow-down' : 'arrow-up'"
                  icon-only
                  :label="`${pinned.has(session.id) ? 'Unpin' : 'Pin'} ${titleOf(session)}`"
                  @click="togglePin(session.id)"
                />
                <BaseButton v-if="session.running" variant="ghost" size="sm" icon="stop" icon-only :label="`Stop ${titleOf(session)}`" @click="emit('cancel', session.id)" />
                <BaseButton variant="ghost" size="sm" icon="edit" icon-only :label="`Rename ${titleOf(session)}`" @click="startRename(session)" />
                <BaseButton variant="ghost" size="sm" icon="trash" icon-only :label="`Delete ${titleOf(session)}`" @click="emit('delete', session.id)" />
              </span>
            </li>
          </ul>
        </section>
      </div>
    </template>
  </div>
</template>
