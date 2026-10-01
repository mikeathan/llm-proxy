<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from "vue"
import { useAutoScroll } from "../../../composables/ui/useAutoScroll"

// A scrollable, focusable log. It follows new lines only while the reader is
// at the bottom, so scrolling up to read is never interrupted.
const props = withDefaults(defineProps<{ text: string; label: string; emptyText?: string }>(), {
  emptyText: "Nothing logged yet.",
})

const pane = ref<HTMLElement | null>(null)
const scroller = useAutoScroll()

watch(
  () => props.text,
  async () => {
    scroller.notifyContent()
    await nextTick()
    scroller.scrollIfNearBottom(pane.value)
  },
)
onMounted(() => scroller.scrollToBottom(pane.value))
</script>

<template>
  <div
    ref="pane"
    role="region"
    :aria-label="label"
    tabindex="0"
    class="max-h-[60vh] min-h-[240px] overflow-auto border border-hairline bg-canvas p-3 focus-visible:outline-none focus-visible:ring-2"
    @scroll="scroller.updateWasNearBottom(pane)"
  >
    <pre v-if="text" class="m-0 whitespace-pre-wrap break-words font-mono text-[length:var(--text-small)] leading-[1.7] text-secondary">{{ text }}</pre>
    <p v-else class="m-0 font-mono text-[length:var(--text-small)] text-muted">{{ emptyText }}</p>
  </div>
</template>
