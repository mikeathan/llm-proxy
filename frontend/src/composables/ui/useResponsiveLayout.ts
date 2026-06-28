import { ref, onMounted, onUnmounted } from "vue"

export function useResponsiveLayout(breakpoint = 1024) {
  const isMobile = ref(window.innerWidth < breakpoint)

  function updateLayout() {
    isMobile.value = window.innerWidth < breakpoint
  }

  onMounted(() => {
    updateLayout()
    window.addEventListener("resize", updateLayout)
  })

  onUnmounted(() => {
    window.removeEventListener("resize", updateLayout)
  })

  return { isMobile }
}
