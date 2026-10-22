import { API_ENDPOINTS } from '../../constants/api'
import { get } from '../httpClient'
import type { HelpersStatus } from '../../types/helpers'

export class HelpersService {
  static status(): Promise<HelpersStatus> {
    return get<HelpersStatus>(API_ENDPOINTS.helpers)
  }
}
