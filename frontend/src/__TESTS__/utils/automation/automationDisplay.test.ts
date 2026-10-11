import { describe, it, expect } from 'vitest'
import { automationStatus, nextRunLabel, triggerLabel } from '../../../utils/automation/automationDisplay'
import type { Automation } from '../../../types/dispatcher'

const auto = (over: Partial<Automation>): Automation => ({ id: 'ws/a', workspace: 'ws', name: 'a', task_file: 't.md', strategy: 'persistent', trigger: 'manual', ...over })

describe('triggerLabel', () => {
  it.each([
    [{ trigger: 'cron', trigger_value: '0 7 * * *' }, 'Every day at 07:00'],
    [{ trigger: 'cron', trigger_value: '*/5 * * * *' }, 'Every 5 minutes'],
    [{ trigger: 'cron', trigger_value: 'not a cron' }, 'Invalid schedule: not a cron'],
    [{ trigger: 'interval', trigger_value: '1h' }, 'Every hour'],
    // The backend stores Go durations: drop the zero units it adds.
    [{ trigger: 'interval', trigger_value: '2h0m0s' }, 'Every 2 hours'],
    [{ trigger: 'interval', trigger_value: '1h30m0s' }, 'Every 90 minutes'],
    [{ trigger: 'interval', trigger_value: '1h30m15s' }, 'Every 1h30m15s'],
    [{ trigger: 'interval', trigger_value: '0s' }, 'Every 0s'],
    [{ trigger: 'manual' }, 'Manual only'],
    [{ trigger: 'webhook' }, 'webhook'],
  ])('%o → %s', (over, label) => {
    expect(triggerLabel(auto(over as Partial<Automation>))).toBe(label)
  })
})

describe('automationStatus', () => {
  it('prefers running over queued, then a failed last run, then idle', () => {
    expect(automationStatus(auto({ is_running: true, queued: true }))).toEqual({ state: 'running', label: 'Running' })
    expect(automationStatus(auto({ queued: true, queue_position: 3 }))).toEqual({ state: 'queued', label: 'Queued #3' })
    expect(automationStatus(auto({ queued: true }))).toEqual({ state: 'queued', label: 'Queued #1' })
    expect(automationStatus(auto({ last_error: 'boom' }))).toEqual({ state: 'error', label: 'Last run failed' })
    expect(automationStatus(auto({}))).toEqual({ state: 'neutral', label: 'Idle' })
  })
})

describe('nextRunLabel', () => {
  // Read on the wall clock the server's timestamp carries, whatever zone the browser is in.
  it.each([
    [{ next_run_at: '2026-10-12T07:30:00+01:00' }, 'Mon 12 Oct, 07:30'],
    [{ next_run_at: '2026-10-12T07:30:00.5Z' }, 'Mon 12 Oct, 07:30'],
    [{}, ''],
    [{ next_run_at: '' }, ''],
  ])('%o → "%s"', (over, label) => {
    expect(nextRunLabel(auto(over as Partial<Automation>))).toBe(label)
  })
})
