export type StepStatus = 'running' | 'success' | 'error'

export interface AgentStepData {
  toolName: string
  args: string
  result: string
  error?: string
  status: StepStatus
  durationMs: number
}

export interface AgentTurnData {
  userMessage: string
  userTimestamp: string
  thinking: string
  steps: AgentStepData[]
  finalAnswer: string
  totalDurationMs: number
  state: 'streaming' | 'completed'
  createdAt: string
}
