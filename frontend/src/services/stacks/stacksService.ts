import { API_ENDPOINTS } from '../../constants/api'
import { get, post, put } from '../httpClient'
import type { StackId, StackSummary, StackView, SwitchedStack } from '../../types/stacks'

/** The stacks and their modules. A switch that needs the passphrase is retried by the HTTP client once the person gives it. */
export class StacksService {
  static async list(): Promise<StackSummary[]> {
    return (await get<{ stacks: StackSummary[] }>(API_ENDPOINTS.stacks)).stacks ?? []
  }

  static stack(id: StackId): Promise<StackView> {
    return get<StackView>(API_ENDPOINTS.stack(id))
  }

  /** Switches every module of the stack on, or off; a stack has no switch of its own. Asks for the passphrase only when a guarded module would change. */
  static async switchAll(stack: StackId, enabled: boolean): Promise<SwitchedStack> {
    const answer = await post<SwitchedStack>(API_ENDPOINTS.stackAll(stack, enabled), {})
    return { ...answer, blocked: answer.blocked ?? [] }
  }

  static async setModule(stack: StackId, module: string, enabled: boolean): Promise<SwitchedStack> {
    const answer = await put<SwitchedStack>(API_ENDPOINTS.stackModule(stack, module), { enabled })
    return { ...answer, blocked: answer.blocked ?? [] }
  }
}
