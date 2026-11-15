import { API_ENDPOINTS } from '../../constants/api'
import { del, get, post, put } from '../httpClient'
import type { HelperSession, RadioAir, RadioAirwaves, RadioClass, RadioDigest, RadioLive, RadioScan, RadioScanPlan, RadioTrack, RadioKeeping, RadioPresence, RadioListenState, RadioNetworks, RadioOwnerDevice, RadioPlanView, RadioView } from '../../types/radio'

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

  /** One device's signal over this run, from memory; 404 when the run ended or the device was silent for a whole track. */
  static track(id: string): Promise<RadioTrack> {
    return get<RadioTrack>(`${API_ENDPOINTS.radioTrack}/${encodeURIComponent(id)}`)
  }

  static keeping(): Promise<RadioKeeping> {
    return get<RadioKeeping>(API_ENDPOINTS.radioKeeping)
  }

  /** 0 turns the track off and deletes what is stored; 24 or 48 keeps it that many hours. */
  static setKeeping(trackHours: number): Promise<RadioKeeping> {
    return put<RadioKeeping>(API_ENDPOINTS.radioKeeping, { track_hours: trackHours })
  }

  /** Deletes the stored tracks and leaves the choice as it is. */
  static dropTracks(): Promise<RadioKeeping> {
    return del<RadioKeeping>(API_ENDPOINTS.radioKeeping)
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
