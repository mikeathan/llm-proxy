import { API_ENDPOINTS } from '../../constants/api'
import { del, get, post, postFile, put } from '../httpClient'
import type { KnownBadState, AcceptRequest, DeepCheck, DeviceExpectation, DecoyState, RouterLogState, OutsideLookupState, OutsideLookupUpdate, DnsCheckState, DnsCheckUpdate, DeviceInventory, IPv6DiscoveryState, PresenceState, PresenceUpdate, ExpectationRequest, FixRequest, FindingsReport, NetworkActivity, NetworkOverview, Readiness, TimelineSummary, UbuntuDataStatus, ScanAck, ScanScope, ScopeUpdate, TargetProblem, VulnDataStatus, VulnImportSource, NetauditSwitch, NearbyState } from '../../types/netaudit'
import type { NetworkAlertEvents } from '../../types/notifications'

const query = (params: Record<string, string>) => `?${new URLSearchParams(params).toString()}`

export class NetauditService {
  static readiness(): Promise<Readiness> {
    return get<Readiness>(API_ENDPOINTS.netauditReadiness)
  }

  static scope(): Promise<ScanScope> {
    return get<ScanScope>(API_ENDPOINTS.netauditScope)
  }

  static updateScope(update: ScopeUpdate): Promise<ScanScope> {
    return put<ScanScope>(API_ENDPOINTS.netauditScope, update)
  }

  static findings(workspaceId: string, automation: string): Promise<FindingsReport> {
    return get<FindingsReport>(`${API_ENDPOINTS.netauditFindings}${query({ workspace_id: workspaceId, automation })}`)
  }

  /** The Network page's first screen. Without an audit it still shows the devices and the trend. */
  static async overview(audit?: { workspaceId: string; automation: string }): Promise<NetworkOverview> {
    const params = audit ? query({ workspace_id: audit.workspaceId, automation: audit.automation }) : ''
    const overview = await get<NetworkOverview>(`${API_ENDPOINTS.netauditOverview}${params}`)
    // The server encodes an empty list as null; the page reads them as lists.
    return { ...overview, series: overview.series ?? [], recent: overview.recent ?? [] }
  }

  /** The Activity timeline: what happened lately, newest first. `limit` is how many rows. */
  static async activity(audit: { workspaceId: string; automation: string } | undefined, limit: number): Promise<NetworkActivity> {
    const params = query({ limit: String(limit), ...(audit ? { workspace_id: audit.workspaceId, automation: audit.automation } : {}) })
    const activity = await get<NetworkActivity>(`${API_ENDPOINTS.netauditActivity}${params}`)
    return { rows: activity.rows ?? [], router: activity.router }
  }

  /** Asks the audit workspace's model, once, to explain the last week. It must be a model on this machine; the server says if not. */
  static explainTimeline(audit: { workspaceId: string; automation: string }): Promise<TimelineSummary> {
    return post<TimelineSummary>(`${API_ENDPOINTS.netauditActivitySummary}${query({ workspace_id: audit.workspaceId, automation: audit.automation })}`)
  }

  /** What is held of Canonical's security notices, and whether this computer can use them. */
  static knownBad(): Promise<KnownBadState> {
    return get<KnownBadState>(API_ENDPOINTS.netauditKnownBad)
  }

  /** Saves the operator's abuse.ch list address, which holds their key. It is a secret: the server asks for the passphrase and never returns it. */
  static saveKnownBadAddress(address: string): Promise<KnownBadState> {
    return put<KnownBadState>(API_ENDPOINTS.netauditKnownBadKey, { address })
  }

  static removeKnownBadAddress(): Promise<KnownBadState> {
    return del<KnownBadState>(API_ENDPOINTS.netauditKnownBadKey)
  }

  /** Fetches the list now with the saved address. */
  static downloadKnownBad(): Promise<KnownBadState> {
    return post<KnownBadState>(API_ENDPOINTS.netauditKnownBadDownload, {})
  }

  static ubuntuData(): Promise<UbuntuDataStatus> {
    return get<UbuntuDataStatus>(API_ENDPOINTS.netauditUbuntuData)
  }

  /** Fetches this release's notices (about 1.3 MB) from Canonical's fixed address and replaces what is held. */
  static updateUbuntuData(): Promise<UbuntuDataStatus> {
    return post<UbuntuDataStatus>(API_ENDPOINTS.netauditUbuntuDataUpdate)
  }

  /** Deletes the notices held. The person is asked for the passphrase again. */
  static clearUbuntuData(): Promise<UbuntuDataStatus> {
    return del<UbuntuDataStatus>(API_ENDPOINTS.netauditUbuntuData)
  }

  static async checkTargets(targets: string[]): Promise<TargetProblem[]> {
    const { problems } = await post<{ problems: TargetProblem[] }>(API_ENDPOINTS.netauditTargetsCheck, { targets })
    return problems
  }

  static acks(workspaceId: string): Promise<ScanAck[]> {
    return get<ScanAck[]>(`${API_ENDPOINTS.netauditAcks}${query({ workspace_id: workspaceId })}`)
  }

  static accept(request: AcceptRequest): Promise<ScanAck> {
    return post<ScanAck>(API_ENDPOINTS.netauditAcks, request)
  }

  static revoke(workspaceId: string, key: string): Promise<void> {
    return del(`${API_ENDPOINTS.netauditAcks}${query({ workspace_id: workspaceId, key })}`)
  }

  static markFixed(request: FixRequest): Promise<unknown> {
    return post(API_ENDPOINTS.netauditFixes, request)
  }

  static unmarkFixed(workspaceId: string, key: string): Promise<void> {
    return del(`${API_ENDPOINTS.netauditFixes}${query({ workspace_id: workspaceId, key })}`)
  }

  static expectations(workspaceId: string): Promise<DeviceExpectation[]> {
    return get<DeviceExpectation[]>(`${API_ENDPOINTS.netauditExpectations}${query({ workspace_id: workspaceId })}`)
  }

  static setExpectation(request: ExpectationRequest): Promise<DeviceExpectation> {
    return put<DeviceExpectation>(API_ENDPOINTS.netauditExpectations, request)
  }

  static removeExpectation(workspaceId: string, device: string): Promise<void> {
    return del(`${API_ENDPOINTS.netauditExpectations}${query({ workspace_id: workspaceId, device })}`)
  }

  /** The last deep check of each device in the workspace, newest first. */
  static deepChecks(workspaceId: string): Promise<DeepCheck[]> {
    return get<DeepCheck[]>(`${API_ENDPOINTS.netauditDeep}${query({ workspace_id: workspaceId })}`)
  }

  /**
   * Starts a deep check of one device and answers when it is done, which can take a few minutes. The server asks for the
   * passphrase again first (the page's usual "confirm it's you" step); it never takes ports or scripts from the page.
   */
  static runDeepCheck(workspaceId: string, automation: string, device: string): Promise<DeepCheck> {
    return post<DeepCheck>(API_ENDPOINTS.netauditDeep, { workspace_id: workspaceId, automation, device })
  }

  static vulnData(): Promise<VulnDataStatus> {
    return get<VulnDataStatus>(API_ENDPOINTS.netauditVulnData)
  }

  /**
   * Starts the download (or resumes it, or refreshes what is held); answers at once with the new state. `restart` fetches
   * every file again instead of only the missing ones.
   */
  static startVulnDownload(restart = false): Promise<VulnDataStatus> {
    return post<VulnDataStatus>(restart ? `${API_ENDPOINTS.netauditVulnDownload}?restart=1` : API_ENDPOINTS.netauditVulnDownload, {})
  }

  /** Deletes all the downloaded vulnerability data (stopping a download that runs); answers with the empty status. */
  static clearVulnData(): Promise<VulnDataStatus> {
    return del<VulnDataStatus>(API_ENDPOINTS.netauditVulnData)
  }

  /** Hands the server a data file fetched elsewhere (NVD's yearly file needs its year; CISA's list does not). */
  static importVulnData(source: VulnImportSource, file: File, year?: number): Promise<VulnDataStatus> {
    const params: Record<string, string> = { source }
    if (year !== undefined) params.year = String(year)
    return postFile<VulnDataStatus>(`${API_ENDPOINTS.netauditVulnImport}${query(params)}`, file)
  }

  static cancelVulnDownload(): Promise<VulnDataStatus> {
    return post<VulnDataStatus>(API_ENDPOINTS.netauditVulnCancel, {})
  }

  /** Who has been seen on the network. */
  static devices(): Promise<DeviceInventory> {
    return get<DeviceInventory>(API_ENDPOINTS.netauditDevices)
  }

  /** Names a device; naming approves it. Answers with the new list. */
  static nameDevice(mac: string, name: string): Promise<DeviceInventory> {
    return put<DeviceInventory>(`${API_ENDPOINTS.netauditDevices}/${encodeURIComponent(mac)}`, { name })
  }

  /** Rejects the name a device gave when asked, so it is not offered again. Answers with the new list. */
  static dismissSuggestion(mac: string): Promise<DeviceInventory> {
    return post<DeviceInventory>(`${API_ENDPOINTS.netauditDevices}/${encodeURIComponent(mac)}/dismiss-suggestion`, {})
  }

  static approveDevice(mac: string): Promise<DeviceInventory> {
    return put<DeviceInventory>(`${API_ENDPOINTS.netauditDevices}/${encodeURIComponent(mac)}`, { approved: true })
  }

  /** Marks a device as important, or clears the mark. It approves nothing. Answers with the new list. */
  static markImportant(mac: string, important: boolean): Promise<DeviceInventory> {
    return put<DeviceInventory>(`${API_ENDPOINTS.netauditDevices}/${encodeURIComponent(mac)}`, { important })
  }

  static forgetDevice(mac: string): Promise<DeviceInventory> {
    return del<DeviceInventory>(`${API_ENDPOINTS.netauditDevices}/${encodeURIComponent(mac)}`)
  }

  /** Accepts the router's current hardware address as the right one. The server asks for the passphrase again first. */
  static confirmGateway(): Promise<DeviceInventory> {
    return post<DeviceInventory>(API_ENDPOINTS.netauditGatewayConfirm, {})
  }

  static presence(): Promise<PresenceState> {
    return get<PresenceState>(API_ENDPOINTS.netauditPresence)
  }

  static setPresence(update: PresenceUpdate): Promise<PresenceState> {
    return put<PresenceState>(API_ENDPOINTS.netauditPresence, update)
  }

  static ipv6Discovery(): Promise<IPv6DiscoveryState> {
    return get<IPv6DiscoveryState>(API_ENDPOINTS.netauditIPv6Discovery)
  }

  static deviceNames(): Promise<NetauditSwitch> {
    return get<NetauditSwitch>(API_ENDPOINTS.netauditDeviceNames)
  }

  static followUpScan(): Promise<NetauditSwitch> {
    return get<NetauditSwitch>(API_ENDPOINTS.netauditFollowUpScan)
  }

  static decoys(): Promise<DecoyState> {
    return get<DecoyState>(API_ENDPOINTS.netauditDecoys)
  }

  static routerLog(): Promise<RouterLogState> {
    return get<RouterLogState>(API_ENDPOINTS.netauditRouterLog)
  }

  static outsideLookup(): Promise<OutsideLookupState> {
    return get<OutsideLookupState>(API_ENDPOINTS.netauditOutsideLookup)
  }

  /** Changes the outside lookup's switch or service address. Switching it on sends the public address to an outside service, so the server asks for the passphrase again first. */
  static setOutsideLookup(update: OutsideLookupUpdate): Promise<OutsideLookupState> {
    return put<OutsideLookupState>(API_ENDPOINTS.netauditOutsideLookup, update)
  }

  static dnsCheck(): Promise<DnsCheckState> {
    return get<DnsCheckState>(API_ENDPOINTS.netauditDnsCheck)
  }

  /** Changes the DNS check's switch or settings. Switching it on lets an outside resolver learn the names and the public address, so the server asks for the passphrase again first. */
  static setDnsCheck(update: DnsCheckUpdate): Promise<DnsCheckState> {
    return put<DnsCheckState>(API_ENDPOINTS.netauditDnsCheck, update)
  }

  /** What devices said about themselves, the switch, and whether a look is running. */
  static nearby(): Promise<NearbyState> {
    return get<NearbyState>(API_ENDPOINTS.netauditNearby)
  }

  /** Starts a look and returns at once; the state says when it has ended. A second look too soon is refused with a plain message. */
  static nearbyLook(): Promise<NearbyState> {
    return post<NearbyState>(API_ENDPOINTS.netauditNearbyLook, {})
  }

  /** Empties the list of devices that connected to a decoy. The switch and the listeners stay as they are. */
  static clearDecoyTouches(): Promise<DecoyState> {
    return del<DecoyState>(API_ENDPOINTS.netauditDecoyTouches)
  }

  /**
   * What the Network module alerted about after the given alert id, newest first. With no id it returns no items, only the
   * newest id, as the place to start watching from.
   */
  static alerts(after?: number): Promise<NetworkAlertEvents> {
    return get<NetworkAlertEvents>(`${API_ENDPOINTS.netauditAlerts}${after === undefined ? '' : query({ after: String(after) })}`)
  }
}
