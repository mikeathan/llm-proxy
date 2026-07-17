import { ref, onUnmounted } from "vue";

/**
 * Auto-scroll composable.
 *
 * Behaviour:
 *   • At rest near the bottom → follows new content.
 *   • User scrolls up → auto-scroll pauses immediately.
 *   • While paused, if the user stops scrolling for `idleMs` AND new content
 *     arrived, auto-scroll resumes and snaps to the bottom.
 *     (A finished/static conversation stays paused.)
 *   • User scrolls back to the bottom → auto-scroll re-arms immediately.
 *
 * Usage with a single container ref:
 *   const { container, scrollIfNearBottom, notifyContent, updateWasNearBottom } = useAutoScroll()
 *   // template: ref="container" @scroll="updateWasNearBottom(container)"
 *   watch(source, async () => {
 *     notifyContent()
 *     await nextTick()
 *     scrollIfNearBottom()
 *   })
 */
export function useAutoScroll(threshold = 50, idleMs = 2000) {
  const container = ref<HTMLElement | null>(null);

  /** Direction the toggle button will scroll on next click. */
  const scrollDirection = ref<"down" | "up">("down");

  let wasNearBottom = true;
  let newContentDuringPause = false;
  let idleTimer: ReturnType<typeof setTimeout> | null = null;

  function clearIdle() {
    if (idleTimer) {
      clearTimeout(idleTimer);
      idleTimer = null;
    }
  }

  function isNearBottom(el: HTMLElement | null): boolean {
    if (!el) return true;
    return el.scrollHeight - el.scrollTop - el.clientHeight < threshold;
  }

  function scrollTo(target: HTMLElement | null, behavior: ScrollBehavior = "instant") {
    if (!target) return;
    target.scrollTo({ top: target.scrollHeight, behavior });
  }

  /** Consumers call this on every content change. */
  function notifyContent() {
    if (!wasNearBottom) {
      newContentDuringPause = true;
    }
  }

  /** Auto-scrolls only while user is near the bottom. */
  function scrollIfNearBottom(el?: HTMLElement | null, behavior: ScrollBehavior = "instant"): boolean {
    const target = el ?? container.value ?? null;
    if (wasNearBottom && target) {
      scrollTo(target, behavior);
      return true;
    }
    return false;
  }

  /** Force scroll to bottom and re-arm auto-scroll. */
  function scrollToBottom(el?: HTMLElement | null, behavior: ScrollBehavior = "instant") {
    const target = el ?? container.value ?? null;
    wasNearBottom = true;
    newContentDuringPause = false;
    clearIdle();
    scrollTo(target, behavior);
  }

  /** Force scroll to top regardless of user position. */
  function scrollToTop(el?: HTMLElement | null, behavior: ScrollBehavior = "smooth") {
    const target = el ?? container.value ?? null;
    target?.scrollTo({ top: 0, behavior });
  }

  /**
   * Bound to the container's @scroll. Tracks whether the user is near the
   * bottom.  Also drives idle-resume: when the user scrolls up from the
   * bottom we start a timer; if new content arrived during the pause it
   * resumes after `idleMs`.
   */
  function updateWasNearBottom(el?: HTMLElement | null) {
    const target = el ?? container.value ?? null;
    if (!target) return;
    const prev = wasNearBottom;
    wasNearBottom = isNearBottom(target);
    if (prev && !wasNearBottom) {
      // User just scrolled up from the bottom → arm idle-resume.
      newContentDuringPause = false;
      clearIdle();
      idleTimer = setTimeout(() => {
        idleTimer = null;
        if (newContentDuringPause) {
          wasNearBottom = true;
          newContentDuringPause = false;
          scrollTo(target, "instant");
        }
      }, idleMs);
    } else if (wasNearBottom) {
      // User is at the bottom → clear any pause state.
      newContentDuringPause = false;
      clearIdle();
    }
  }

  /**
   * Scroll in the current direction, then flip the arrow for next click.
   *   • down → scrolls to bottom (re-arms), flips to up
   *   • up   → scrolls to top, flips to down
   */
  function toggleScroll(el?: HTMLElement | null) {
    const target = el ?? container.value ?? null;
    if (!target) return;
    if (scrollDirection.value === "down") {
      scrollToBottom(target, "smooth");
      scrollDirection.value = "up";
    } else {
      scrollToTop(target, "smooth");
      scrollDirection.value = "down";
    }
  }

  onUnmounted(clearIdle);

  return {
    container,
    scrollIfNearBottom,
    scrollToBottom,
    scrollToTop,
    toggleScroll,
    scrollDirection,
    isNearBottom,
    updateWasNearBottom,
    notifyContent,
  };
}
