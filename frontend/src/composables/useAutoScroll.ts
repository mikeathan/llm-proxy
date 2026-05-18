import { ref } from "vue";

/**
 * Professional auto-scroll composable.
 *
 * Tracks whether the user is near the bottom BEFORE a DOM update, then
 * auto-scrolls AFTER the update only if they were.  This means:
 *   • Scroll up to read → auto-scroll pauses
 *   • Scroll back to the bottom → auto-scroll naturally resumes
 *
 * Also provides a toggle-scroll button that alternates between
 * scrolling to bottom and scrolling to top on each click.
 *
 * Usage with a single container ref:
 *   const { container, capturePosition, scrollIfNearBottom, scrollDirection, toggleScroll } = useAutoScroll()
 *   // template: ref="container"
 *   watch(source, async () => {
 *     capturePosition()
 *     await nextTick()
 *     scrollIfNearBottom()
 *   })
 *
 * Usage with a manual ref / conditional containers (e.g. Logs.vue's two panes):
 *   const scroller = useAutoScroll()
 *   watch(source, async () => {
 *     const el = isActive("app") ? appEl.value : processEl.value
 *     scroller.capturePosition(el)
 *     await nextTick()
 *     scroller.scrollIfNearBottom(el)
 *   })
 */
export function useAutoScroll(threshold = 50) {
  // When used with template ref="container"
  const container = ref<HTMLElement | null>(null);

  /** Direction the toggle button will scroll on next click. */
  const scrollDirection = ref<"down" | "up">("down");

  let wasNearBottom = true;

  function isNearBottom(el: HTMLElement | null): boolean {
    if (!el) return true;
    return el.scrollHeight - el.scrollTop - el.clientHeight < threshold;
  }

  /** Call BEFORE the DOM updates — saves whether user was near the bottom. */
  function capturePosition(el?: HTMLElement | null) {
    wasNearBottom = isNearBottom(el ?? container.value ?? null);
  }

  /** Call AFTER the DOM updates — auto-scrolls only if they were near bottom. */
  function scrollIfNearBottom(el?: HTMLElement | null, behavior: ScrollBehavior = "smooth"): boolean {
    const target = el ?? container.value ?? null;
    if (wasNearBottom && target) {
      target.scrollTo({ top: target.scrollHeight, behavior });
      return true;
    }
    return false;
  }

  /** Force scroll to bottom regardless of user position. */
  function scrollToBottom(el?: HTMLElement | null, behavior: ScrollBehavior = "smooth") {
    const target = el ?? container.value ?? null;
    target?.scrollTo({ top: target.scrollHeight, behavior });
  }

  /** Force scroll to top regardless of user position. */
  function scrollToTop(el?: HTMLElement | null, behavior: ScrollBehavior = "smooth") {
    const target = el ?? container.value ?? null;
    target?.scrollTo({ top: 0, behavior });
  }

  /**
   * Scroll in the current direction, then flip the arrow for next click.
   *   • down → scrolls to bottom, flips to up
   *   • up   → scrolls to top, flips to down
   */
  function toggleScroll(el?: HTMLElement | null) {
    const target = el ?? container.value ?? null;
    if (!target) return;
    if (scrollDirection.value === "down") {
      target.scrollTo({ top: target.scrollHeight, behavior: "smooth" });
      scrollDirection.value = "up";
    } else {
      target.scrollTo({ top: 0, behavior: "smooth" });
      scrollDirection.value = "down";
    }
  }

  return {
    container,
    capturePosition,
    scrollIfNearBottom,
    scrollToBottom,
    scrollToTop,
    toggleScroll,
    scrollDirection,
    isNearBottom,
  };
}
