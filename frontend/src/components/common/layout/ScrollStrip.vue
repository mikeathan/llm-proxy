<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from "vue"
import { useScrollEdges } from "../../../composables/ui/useScrollEdges"
import { motionScroll } from "../../../utils/motion"

// A row of links that scrolls sideways when it does not fit (tabs, chips). The
// end that is cut off fades, and the current link (aria-current="page") is
// scrolled into view when `current` changes, so a deep link never opens on a tab
// that is out of sight.
const props = defineProps<{ label: string; current?: string }>()

const strip = ref<HTMLElement | null>(null)
const { moreBefore, moreAfter, update } = useScrollEdges(strip)

function revealCurrent() {
  strip.value
    ?.querySelector<HTMLElement>('[aria-current="page"]')
    ?.scrollIntoView?.({ inline: "center", block: "nearest", behavior: motionScroll("auto") })
  update()
}

onMounted(revealCurrent)
watch(() => props.current, () => void nextTick(revealCurrent))
</script>

<template>
  <nav ref="strip" :aria-label="label" :class="['scroll-strip', { 'scroll-strip--before': moreBefore, 'scroll-strip--after': moreAfter }]">
    <slot />
  </nav>
</template>

<style scoped lang="postcss">
.scroll-strip {
  @apply flex gap-1 overflow-x-auto;
  scrollbar-width: none;
}
.scroll-strip::-webkit-scrollbar {
  display: none;
}
/* The cut-off end fades; the fade length is a variable so both ends can combine. */
.scroll-strip--before,
.scroll-strip--after {
  --fade-start: 0px;
  --fade-end: 0px;
  mask-image: linear-gradient(90deg, transparent, #000 var(--fade-start), #000 calc(100% - var(--fade-end)), transparent);
}
.scroll-strip--before {
  --fade-start: 36px;
}
.scroll-strip--after {
  --fade-end: 36px;
}
</style>
