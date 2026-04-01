export interface WorkspaceConfig {
  cron_schedule: string;
  model: string;
  temperature: number;
}

export interface AgentState {
  last_output: string;
  last_error: string;
  next_run_predicted: string;
  is_running: boolean;
}

export interface Workspace {
  id: string;
  config: WorkspaceConfig;
  state: AgentState;
  heartbeat: string;
}
