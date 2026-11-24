import type { ModelStatsReport } from '../types/modelStats'
import { apiFetch } from './httpClient'
import { failureReason } from './failureReason'

const BASE = '/admin/api/model-stats'

export class ModelStatsService {
  /** The model use counts for the last `days` days (the server keeps two weeks). */
  static async get(days: number): Promise<ModelStatsReport> {
    const res = await apiFetch(`${BASE}?days=${days}`)
    if (!res.ok) throw await failureReason(res, 'read the model use')
    return (await res.json()) as ModelStatsReport
  }
}
