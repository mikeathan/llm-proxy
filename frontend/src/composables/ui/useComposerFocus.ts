import { ref } from 'vue'

// Whether the chat's message box has focus. On a phone the shell hides the tab bar while it does (the keyboard is up and
// the bar would only take room from the conversation). Module state: there is one composer on screen at a time.
//
// Losing focus is reported a moment late: tapping Send or + blurs the box first, and if the tab bar came back at once
// the composer would move up under the finger before the tap's click landed.
const BLUR_DELAY_MS = 300
const focused = ref(false)
let blurTimer: ReturnType<typeof setTimeout> | null = null

function onFocus() {
  if (blurTimer) clearTimeout(blurTimer)
  blurTimer = null
  focused.value = true
}

function onBlur() {
  if (blurTimer) clearTimeout(blurTimer)
  blurTimer = setTimeout(() => {
    blurTimer = null
    focused.value = false
  }, BLUR_DELAY_MS)
}

function reset() {
  if (blurTimer) clearTimeout(blurTimer)
  blurTimer = null
  focused.value = false
}

export function useComposerFocus() {
  return { focused, onFocus, onBlur, reset }
}
