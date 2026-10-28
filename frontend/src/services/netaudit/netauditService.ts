import { API_ENDPOINTS } from '../../constants/api'
import { del, get, post, put } from '../httpClient'
import type { AcceptRequest, FixRequest, FindingsReport, Readiness, ScanAck, ScanScope, ScopeUpdate, TargetProblem, VulnDataStatus } from '../../types/netaudit'

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

  static cancelVulnDownload(): Promise<VulnDataStatus> {
    return post<VulnDataStatus>(API_ENDPOINTS.netauditVulnCancel, {})
  }
}
