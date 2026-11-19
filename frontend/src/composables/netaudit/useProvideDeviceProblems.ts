import { provide, watch } from 'vue'
import { useNetworkAudit } from './useNetworkAudit'
import { DEVICE_PROBLEMS_INJECTION, useDeviceProblems } from './useDeviceProblems'

/**
 * Gives the cards below the open problems by device (the device list and the traffic detail link a device to its problems), read again when the
 * audit is known. A card that shows devices calls this once; the lookup is the card's own, so no page has to provide it.
 */
export function useProvideDeviceProblems(): void {
  const { audit } = useNetworkAudit()
  const problems = useDeviceProblems(() => audit.value)
  provide(DEVICE_PROBLEMS_INJECTION, problems)
  watch(() => audit.value?.automation, () => void problems.load(), { immediate: true })
}
