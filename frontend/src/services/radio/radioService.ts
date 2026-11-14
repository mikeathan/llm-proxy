import { API_ENDPOINTS } from '../../constants/api'
import { del, get, post, put } from '../httpClient'
import type { HelperSession, RadioAir, RadioAirDevices, RadioAirSort, RadioAirwaves, RadioClass, RadioDigest, RadioLive, RadioScan, RadioScanPlan, RadioPresence, RadioListenState, RadioNetworks, RadioOwnerDevice, RadioPlanView, RadioView } from '../../types/radio'

/** The Radio stack: the sensors page, the listening run, the owner's devices. Reading the sensors sends nothing and listens to nothing. */
export class RadioService {
  static sensors(): Promise<RadioView> {
    return get<RadioView>(API_ENDPOINTS.radioSensors)
  }

  static listening(): Promise<RadioListenState> {
    return get<RadioListenState>(API_ENDPOINTS.radioListening)
  }

  /** The exact requests a session must approve for this adapter: the run asks the same ones. */
  static plan(adapter: string): Promise<RadioPlanView> {
    return get<RadioPlanView>(`${API_ENDPOINTS.radioListeningPlan}?adapter=${encodeURIComponent(adapter)}`)
  }

  /** Opens a bounded session for the approved list (a step-up route: the passphrase is asked for). */
  static beginSession(body: { stack: string; label: string; hours: number; max_uses: number; requests: unknown[] }): Promise<HelperSession> {
    return post<HelperSession>(API_ENDPOINTS.helperSessions, body)
  }

  static async openSessions(): Promise<HelperSession[]> {
    const r = await get<{ open: HelperSession[] }>(API_ENDPOINTS.helperSessions)
    return r.open
  }

  static endSession(id: string): Promise<void> {
    return del(`${API_ENDPOINTS.helperSessions}/${encodeURIComponent(id)}`)
  }

  static start(adapter: string, hours: number): Promise<RadioListenState> {
    return post<RadioListenState>(API_ENDPOINTS.radioListening, { adapter, hours })
  }

  static stop(): Promise<RadioListenState> {
    return del<RadioListenState>(API_ENDPOINTS.radioListening)
  }

  static async ownerDevices(): Promise<RadioOwnerDevice[]> {
    const r = await get<{ devices: RadioOwnerDevice[] }>(API_ENDPOINTS.radioOwnerDevices)
    return r.devices
  }

  static addOwnerDevice(address: string, label: string, cls?: RadioClass): Promise<{ label: string }> {
    return post<{ label: string }>(API_ENDPOINTS.radioOwnerDevices, cls ? { address, label, class: cls } : { address, label })
  }

  static air(): Promise<RadioAir> {
    return get<RadioAir>(API_ENDPOINTS.radioAir)
  }

  /** Other people's devices, one row each by a short id; `cls` narrows to one kind. */
  static airDevices(cls?: RadioClass, sort?: RadioAirSort): Promise<RadioAirDevices> {
    const q = new URLSearchParams()
    if (cls) q.set('class', cls)
    if (sort) q.set('sort', sort)
    const query = q.toString()
    return get<RadioAirDevices>(query ? `${API_ENDPOINTS.radioAirDevices}?${query}` : API_ENDPOINTS.radioAirDevices)
  }

  static airwaves(): Promise<RadioAirwaves> {
    return get<RadioAirwaves>(API_ENDPOINTS.radioAirwaves)
  }

  static scan(): Promise<RadioScan> {
    return get<RadioScan>(API_ENDPOINTS.radioScan)
  }

  /** Empty channels and zero seconds put the default back. */
  static saveScan(plan: RadioScanPlan): Promise<RadioScan> {
    return put<RadioScan>(API_ENDPOINTS.radioScan, plan)
  }

  static live(): Promise<RadioLive> {
    return get<RadioLive>(API_ENDPOINTS.radioLive)
  }

  /** Takes one of the owner's devices as standing one metre from the computer, so every distance is measured from it. */
  static calibrate(id: string): Promise<{ calibrated: boolean }> {
    return post<{ calibrated: boolean }>(`${API_ENDPOINTS.radioCalibrate}/${encodeURIComponent(id)}`, {})
  }

  static digest(): Promise<RadioDigest> {
    return get<RadioDigest>(API_ENDPOINTS.radioDigest)
  }

  static presence(): Promise<RadioPresence> {
    return get<RadioPresence>(API_ENDPOINTS.radioPresence)
  }

  static networks(): Promise<RadioNetworks> {
    return get<RadioNetworks>(API_ENDPOINTS.radioNetworks)
  }

  static setNetworkNames(names: string[]): Promise<{ names: string[] }> {
    return put<{ names: string[] }>(API_ENDPOINTS.radioNetworks, { names })
  }

  static confirmNetwork(id: string): Promise<{ confirmed: boolean }> {
    return post<{ confirmed: boolean }>(`${API_ENDPOINTS.radioNetworks}/${encodeURIComponent(id)}/confirm`)
  }

  static dropNetwork(id: string): Promise<void> {
    return del(`${API_ENDPOINTS.radioNetworks}/${encodeURIComponent(id)}`)
  }

  static removeOwnerDevice(id: string): Promise<void> {
    return del(`${API_ENDPOINTS.radioOwnerDevices}/${encodeURIComponent(id)}`)
  }

  static forgetNeighbours(): Promise<{ forgotten: boolean }> {
    return post<{ forgotten: boolean }>(API_ENDPOINTS.radioForget)
  }
}
