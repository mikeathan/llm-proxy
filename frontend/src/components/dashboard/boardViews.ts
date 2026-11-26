import { defineAsyncComponent } from 'vue'
import type { DashboardBoardView, DashboardBoardViewDef } from '../../types/dashboard'

// The boards that are one page to read instead of a grid of cards (ADR-0089). The view fills the board and links to where what it shows is set up
// itself. A view is loaded when its board is first opened.
export const BOARD_VIEWS: Readonly<Record<DashboardBoardView, DashboardBoardViewDef>> = {
  'newsroom-brief': { component: defineAsyncComponent(() => import('../newsroom/NewsroomBriefs.vue')), wide: true },
  'newsroom-feed': { component: defineAsyncComponent(() => import('../newsroom/NewsroomFeedView.vue')), wide: false },
}
