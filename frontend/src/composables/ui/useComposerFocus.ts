import { ref } from 'vue'

// Whether the chat's message box has focus. On a phone the shell hides the tab bar while it does (the keyboard is up and
// the bar would only take room from the conversation). Module state: there is one composer on screen at a time.
const focused = ref(false)

export function useComposerFocus() {
  return { focused }
}
