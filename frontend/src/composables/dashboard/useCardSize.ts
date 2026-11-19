import { inject, ref, type Ref } from 'vue'
import { DASHBOARD_CARD_SIZE } from '../../constants/dashboard'
import type { DashboardCardSize } from '../../types/dashboard'

/** The size the dashboard card around this component has, from its own width. Outside a card it is large, so the component shows everything. */
export function useCardSize(): Ref<DashboardCardSize> {
  return inject(DASHBOARD_CARD_SIZE, () => ref<DashboardCardSize>('lg'), true)
}
