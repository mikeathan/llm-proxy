import { API_ENDPOINTS } from '../../constants/api'
import { del, get, post, postFile, put } from '../httpClient'
import type { AcceptRequest, DeepCheck, DeviceExpectation, ExpectationRequest, FixRequest, FindingsReport, Readiness, ScanAck, ScanScope, ScopeUpdate, TargetProblem, VulnDataStatus, VulnImportSource } from '../../types/netaudit'

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
}
