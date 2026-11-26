import type { Automation, AutomationRun } from '../../src/types/dispatcher'

// Two automations shaped like vertex's on 2026-11-26 (names, schedules and run times); the outputs are made up.
const run = (id: string, timestamp: string, duration_ms: number, error = ''): AutomationRun =>
  ({ id, workspace_id: 'workspace-1', automation_name: '', timestamp, error, output: '', duration_ms, model: 'Qwen3.6 35B A3B' })

export const AUTOMATIONS: Automation[] = [
  {
    id: 'workspace-1/network-audit', workspace: 'workspace-1', name: 'network-audit', task_file: 'network-audit.md', strategy: 'isolated',
    trigger: 'cron', trigger_value: '0 2 * * *', model: 'Qwen3.6 35B A3B', profile: 'security-auditor', next_run_at: '2026-11-27T02:00:00Z',
    last_output: '## Assessment (written by the model)\n\nNothing new since yesterday.', last_error: '',
    history: [run('a1', '2026-11-25T02:04:00Z', 251_000), run('a2', '2026-11-26T02:04:07Z', 247_000)],
  },
  {
    id: 'workspace-1/radio-watch-report', workspace: 'workspace-1', name: 'radio-watch-report', task_file: 'radio-watch-report.md', strategy: 'isolated',
    trigger: 'manual', trigger_value: '', model: 'Qwen3.6 35B A3B', last_error: '',
    last_output: '[2026-11-18 06:58:21] ▶ Executing automation `radio-watch-report` in workspace `workspace-1`...\nReading task file: `radio-watch-report.md`\n\n⏱ **Duration:** 1m 9s\n\n### Final Report\n\nListening: 486 cycles in total. Two access points do not protect management frames.',
    history: [run('b1', '2026-11-16T10:02:00Z', 0, 'The local model would not start'), run('b2', '2026-11-18T06:58:21Z', 69_000)],
  },
]
