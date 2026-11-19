import { API_ENDPOINTS } from '../../constants/api'
import { del, get, put } from '../httpClient'
import type { DashboardLayout } from '../../types/dashboard'

/** The dashboard layout the server keeps for the owner: which boards, which cards, how large. An empty list of boards means "use the shipped defaults". */
export class DashboardService {
  static layout(): Promise<DashboardLayout> {
    return get<DashboardLayout>(API_ENDPOINTS.dashboard)
  }

  static save(layout: DashboardLayout): Promise<DashboardLayout> {
    return put<DashboardLayout>(API_ENDPOINTS.dashboard, layout)
  }

  /** Forgets the layout, so the shipped boards show again. */
  static reset(): Promise<DashboardLayout> {
    return del<DashboardLayout>(API_ENDPOINTS.dashboard)
  }
}
