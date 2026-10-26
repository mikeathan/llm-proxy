import { API_ENDPOINTS } from '../../constants/api'
import { del, get, post, put } from '../httpClient'
import type { AcceptRequest, FindingsReport, Readiness, ScanAck, ScanScope, ScopeUpdate } from '../../types/netaudit'

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

  static acks(workspaceId: string): Promise<ScanAck[]> {
    return get<ScanAck[]>(`${API_ENDPOINTS.netauditAcks}${query({ workspace_id: workspaceId })}`)
  }

  static accept(request: AcceptRequest): Promise<ScanAck> {
    return post<ScanAck>(API_ENDPOINTS.netauditAcks, request)
  }

  static revoke(workspaceId: string, key: string): Promise<void> {
    return del(`${API_ENDPOINTS.netauditAcks}${query({ workspace_id: workspaceId, key })}`)
  }
}
