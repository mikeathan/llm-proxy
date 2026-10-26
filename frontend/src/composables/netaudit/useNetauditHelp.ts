import { ref } from 'vue'
import type { NetauditHelpTopic } from '../../types/netaudit'

// The Help panel is one per page: any screen's Help button, or a warning's "Learn more", opens it on a topic.
const open = ref(false)
const topic = ref<NetauditHelpTopic>('basics')

export function useNetauditHelp() {
  return {
    open,
    topic,
    show: (on: NetauditHelpTopic = 'basics') => {
      topic.value = on
      open.value = true
    },
    hide: () => (open.value = false),
  }
}
