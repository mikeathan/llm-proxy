import { API_ENDPOINTS } from '../../constants/api'
import { get } from '../httpClient'
import type { PromptProfile } from '../../types/profile'

export class ProfileService {
  static list(): Promise<PromptProfile[]> {
    return get<PromptProfile[]>(API_ENDPOINTS.profiles)
  }
}
