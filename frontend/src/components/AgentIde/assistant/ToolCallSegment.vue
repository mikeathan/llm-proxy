<script setup lang="ts">
import { ref, computed } from 'vue'
import Icon from '../../icons/Icon.vue'
import { describeToolStep, formatToolArguments, formatToolResult, TOOL_KIND_ICON } from '../../../utils/assistant/toolSteps'

// One tool call in the assistant's step timeline (ChatBubble): an icon for its
// kind, a plain verb, the target it acted on and its outcome in words. Opened,
// it shows the arguments and the result, decoded for reading (search hits as
// links, a terminal's output as lines). Results are untrusted text: rendered
// as text only, links limited to http(s).

interface ToolCallSeg {
  kind: 'tool_call'
  name: string
  args: string
  status: string
  result?: string
  error?: string
}

const props = defineProps<{
  segment: ToolCallSeg
  turnIdx: number
  segIdx: number
  expanded: boolean
  compact?: boolean
}>()

const emit = defineEmits<{
  toggle: [turnIdx: number, segIdx: number]
}>()

const STATUS_TEXT: Record<string, string> = { running: 'Running', success: 'Done', error: 'Failed' }
const STATUS_ICON: Record<string, string> = { running: 'spinner', success: 'check', error: 'close' }

// In compact mode the row is collapsed by default; clicking toggles a local
// inline expansion. In non-compact mode the `expanded` prop drives detail.
const compactExpanded = ref(false)
const showAll = ref(false)

const showDetail = computed(() => {
  if (props.compact) return compactExpanded.value
  return props.expanded
})

const step = computed(() => describeToolStep(props.segment.name, props.segment.args || ''))
const icon = computed(() => TOOL_KIND_ICON[step.value.kind])
const status = computed(() => (props.segment.status in STATUS_TEXT ? props.segment.status : 'error'))
// Parsed only once opened: results can be large.
const args = computed(() => (showDetail.value ? formatToolArguments(props.segment.args || '') : []))
const result = computed(() =>
  showDetail.value && props.segment.result ? formatToolResult(props.segment.result, { full: showAll.value }) : null,
)

function hostOf(url: string): string {
  try {
    return new URL(url).hostname
  } catch {
    return ''
  }
}

function onClick() {
  if (props.compact) {
    compactExpanded.value = !compactExpanded.value
  } else {
    emit('toggle', props.turnIdx, props.segIdx)
  }
}
</script>

<template>
  <li class="step" :class="`step--${status}`">
    <span class="step-marker" aria-hidden="true"><Icon :name="icon" size="xs" /></span>
    <div class="step-body">
      <button type="button" :aria-expanded="showDetail" class="step-header" @click="onClick">
        <span class="step-verb">{{ step.verb }}</span>
        <span v-if="step.target" class="step-target" :title="step.target">{{ step.target }}</span>
        <span class="step-status">
          <Icon :name="STATUS_ICON[status]!" size="xs" />
          <span class="sr-only">{{ STATUS_TEXT[status] }}</span>
        </span>
        <span class="step-chevron" :class="{ 'is-open': showDetail }" aria-hidden="true"><Icon name="chevron-right" size="xs" /></span>
      </button>

      <div v-if="showDetail" class="step-detail">
        <dl v-if="args.length" class="step-args">
          <div v-for="arg in args" :key="arg.key" class="step-arg">
            <dt>{{ arg.key }}</dt>
            <dd>{{ arg.value }}</dd>
          </div>
        </dl>

        <template v-if="result">
          <ul v-if="result.kind === 'search'" class="step-hits">
            <li v-for="(hit, i) in result.hits" :key="i" class="step-hit">
              <a v-if="hit.url" :href="hit.url" target="_blank" rel="noopener noreferrer" class="step-hit-title">{{ hit.title }}</a>
              <span v-else class="step-hit-title">{{ hit.title }}</span>
              <span v-if="hit.url" class="step-hit-host">{{ hostOf(hit.url) }}</span>
              <p v-if="hit.snippet" class="step-hit-snippet">{{ hit.snippet }}</p>
            </li>
          </ul>
          <template v-else>
            <pre class="step-output" :class="{ 'step-output--terminal': step.kind === 'terminal' }">{{ result.text }}</pre>
            <button v-if="result.truncated" type="button" class="step-more" @click="showAll = true">Show all</button>
          </template>
        </template>

        <pre v-if="segment.error" class="step-output step-output--error">{{ segment.error }}</pre>
      </div>
    </div>
  </li>
</template>

<style scoped>
.step { @apply relative flex gap-2.5; }
.step-marker {
  @apply relative z-[1] mt-[3px] flex h-5 w-5 flex-none items-center justify-center rounded-full border border-hairline bg-canvas text-muted;
}
.step--error .step-marker { @apply border-state-error/50 text-state-error; }
.step-marker :deep(svg) { @apply h-3 w-3; }
.step-body { @apply min-w-0 flex-1; }

.step-header {
  @apply flex w-full min-w-0 items-center gap-2 rounded-[var(--radius-sm)] px-1.5 py-1 text-left text-[length:var(--text-small)] transition-colors duration-fast hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2;
  border: none; background: transparent; cursor: pointer; font-family: inherit; color: inherit;
}
.step-verb { @apply flex-none text-secondary; }
.step-target { @apply min-w-0 flex-1 truncate font-mono text-[11.5px] text-muted; }
.step-status { @apply ml-auto flex flex-none items-center; }
.step-status :deep(svg) { @apply h-3 w-3; }
.step--success .step-status { @apply text-state-success; }
.step--running .step-status { @apply text-state-running; }
.step--error .step-status { @apply text-state-error; }
.step-chevron { @apply flex flex-none text-faint transition-transform duration-fast; }
.step-chevron :deep(svg) { @apply h-3 w-3; }
.step-chevron.is-open { transform: rotate(90deg); }

.step-detail { @apply mb-1 ml-1.5 mt-1 flex flex-col gap-2.5 rounded-[var(--radius-sm)] border border-hairline bg-canvas p-3; }
.step-args { @apply m-0 grid gap-x-3 gap-y-1; grid-template-columns: max-content minmax(0, 1fr); }
.step-arg { display: contents; }
.step-arg dt { @apply font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] leading-5 text-muted; }
.step-arg dd { @apply m-0 whitespace-pre-wrap break-words font-mono text-[11.5px] leading-5 text-secondary; }

.step-output {
  @apply m-0 max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-[var(--radius-sm)] border border-hairline bg-surface-raised p-2.5 font-mono text-[11.5px] leading-relaxed text-secondary;
}
.step-output--terminal { @apply text-primary; }
.step-output--error { @apply border-state-error/40 bg-state-error/10 text-state-error; }
.step-more { @apply self-start font-mono text-[length:var(--text-small)] text-accent-info-text hover:underline focus-visible:outline-none focus-visible:ring-2; border: none; background: none; cursor: pointer; padding: 0; }

.step-hits { @apply m-0 flex max-h-80 list-none flex-col gap-2.5 overflow-y-auto p-0; }
.step-hit { @apply flex flex-col gap-0.5; }
.step-hit-title { @apply text-[length:var(--text-small)] font-medium text-primary; }
a.step-hit-title { @apply text-accent-info-text underline-offset-2 hover:underline focus-visible:outline-none focus-visible:ring-2; }
.step-hit-host { @apply font-mono text-[length:var(--text-micro)] text-faint; }
.step-hit-snippet { @apply m-0 line-clamp-2 text-[length:var(--text-small)] leading-snug text-muted; }
</style>
