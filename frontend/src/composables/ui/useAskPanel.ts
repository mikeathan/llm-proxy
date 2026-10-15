import { ref } from "vue"

// Whether the ask panel (the assistant beside any page) is open. One module-level
// flag: the header's Ask field opens it and the panel itself closes it, and neither
// owns the other.
const open = ref(false)

export function useAskPanel() {
  return {
    open,
    show: () => (open.value = true),
    hide: () => (open.value = false),
    toggle: () => (open.value = !open.value),
  }
}
