import { API_ENDPOINTS } from '../../constants/api'
import { del, get, put } from '../httpClient'
import type { ProfileDeleted, ProfileDetail, ProfileField, ProfileInput, PromptProfile } from '../../types/profile'

const profileUrl = (id: string) => `${API_ENDPOINTS.profiles}/${encodeURIComponent(id)}`

export class ProfileService {
  static list(): Promise<PromptProfile[]> {
    return get<PromptProfile[]>(API_ENDPOINTS.profiles)
  }

  static get(id: string): Promise<ProfileDetail> {
    return get<ProfileDetail>(profileUrl(id))
  }

  static schema(): Promise<ProfileField[]> {
    return get<ProfileField[]>(API_ENDPOINTS.profileSchema)
  }

  static save(id: string, input: ProfileInput): Promise<ProfileDetail> {
    return put<ProfileDetail>(profileUrl(id), input)
  }

  static remove(id: string): Promise<ProfileDeleted> {
    return del<ProfileDeleted>(profileUrl(id))
  }
}
