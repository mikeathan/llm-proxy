import { API_ENDPOINTS } from '../../constants/api'
import { get } from '../httpClient'
import type { RadioView } from '../../types/radio'

/** The Radio stack's sensors page. Reading it sends nothing and listens to nothing. */
export class RadioService {
  static sensors(): Promise<RadioView> {
    return get<RadioView>(API_ENDPOINTS.radioSensors)
  }
}
