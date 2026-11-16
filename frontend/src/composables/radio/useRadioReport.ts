import { onBeforeUnmount, ref } from 'vue'
import { RadioService } from '../../services/radio/radioService'
import { DispatcherService } from '../../services/automation/dispatcherService'
import { TemplateService } from '../../services/template/templateService'
import { useWorkspaceFileWriter } from '../editor/useWorkspaceFileWriter'
import { errorMessage } from '../../utils/errors'
import { RADIO_REPORT_FAILED, RADIO_REPORT_ID, RADIO_REPORT_NO_WORKSPACE } from '../../constants/radio'
import type { Automation } from '../../types/dispatcher'

const POLL_MS = 3000
/** Idle looks allowed after a press before giving up on seeing its run: the run can start a moment after the answer to the press. */
const MAX_IDLE_LOOKS = 10
const DAILY_FALLBACK = '0 7 * * *'

/** When the report runs: only on a press, or on the playbook's own schedule. */
type ReportSchedule = 'manual' | 'daily'

/**
 * Radio · Report. The report is an ordinary automation made from the radio-watch-report playbook, so it keeps the scheduler, the run queue, the
 * history and the bell that every automation has. Setting it up writes the playbook as the task file, creates the automation (manual, or with the
 * playbook's own schedule) and runs it once; later presses only run it. It never starts listening: that takes the owner's passphrase.
 */
export function useRadioReport() {
  const { writeFile } = useWorkspaceFileWriter()
  const automation = ref<Automation | null>(null)
  const workspaces = ref<string[]>([])
  // Whether a watch is running: one look, not a second poller (the Listening panel owns the polling).
  const listening = ref<boolean | null>(null)
  const error = ref('')
  const busy = ref(false)
  // Set from a press until its run has been seen to start and end (or a new result has appeared): a look straight after the press can still show
  // the automation idle with the old result, and that must not read as the run being over.
  const waiting = ref(false)
  let before = ''
  let sawRun = false
  let idleLooks = 0
  let timer: ReturnType<typeof setInterval> | null = null

  async function load(): Promise<void> {
    try {
      const [all, spaces, state] = await Promise.all([DispatcherService.listAutomations(), DispatcherService.listWorkspaces(), RadioService.listening().then((st) => st.listening, () => null)]) // a stack that is off answers with a refusal: that is "unknown", not a failed report
      listening.value = state
      automation.value = all.find((a) => a.name === RADIO_REPORT_ID) ?? null
      settle()
      workspaces.value = spaces.map((w) => w.id)
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, RADIO_REPORT_FAILED)
    }
    sync()
  }

  /** What tells one result from the next: the number of runs kept and the last output and error. */
  const marker = (a: Automation | null): string => `${a?.history?.length ?? 0}|${a?.last_output ?? ''}|${a?.last_error ?? ''}`

  /** Ends the wait once the pressed run has been seen going and gone, a new result is there, or too many looks found it idle. */
  function settle(): void {
    if (!waiting.value) return
    const a = automation.value
    if (a?.is_running || a?.queued) { sawRun = true; return }
    if (sawRun || marker(a) !== before || ++idleLooks > MAX_IDLE_LOOKS) waiting.value = false
  }

  /** Polls while the report is running or waiting, and not otherwise. */
  function sync(): void {
    const want = waiting.value || !!(automation.value?.is_running || automation.value?.queued)
    if (want && !timer) timer = setInterval(() => { void load() }, POLL_MS)
    if (!want && timer) { clearInterval(timer); timer = null }
  }

  async function run(workspace: string): Promise<void> {
    before = marker(automation.value)
    sawRun = false
    idleLooks = 0
    await DispatcherService.triggerAutomation(workspace, RADIO_REPORT_ID)
    waiting.value = true
    await load()
  }

  /** Makes the automation from the playbook and runs it once; nothing is created when replacing an existing task file is declined. */
  async function setUp(workspace: string, schedule: ReportSchedule): Promise<void> {
    if (!workspace) { error.value = RADIO_REPORT_NO_WORKSPACE; return }
    busy.value = true
    error.value = ''
    try {
      const template = await TemplateService.getTemplate(RADIO_REPORT_ID)
      const file = `${RADIO_REPORT_ID}.md`
      if ((await writeFile(workspace, file, template.content)) === 'cancelled') return
      const daily = schedule === 'daily'
      await DispatcherService.createAutomation(workspace, {
        name: RADIO_REPORT_ID,
        trigger: daily ? { type: 'cron', value: template.defaults?.schedule || DAILY_FALLBACK } : { type: 'manual', value: '' },
        task_file: file, strategy: '', model: '', loop_strategy: '', network_grant: '', memory_mode: '',
        notify: null, skip_if_busy: daily, journal: !!template.defaults?.journal, profile: template.defaults?.profile ?? '',
      })
      await run(workspace)
    } catch (err) {
      error.value = errorMessage(err, RADIO_REPORT_FAILED)
    } finally {
      busy.value = false
    }
  }

  async function analyse(): Promise<void> {
    const found = automation.value
    if (!found) return
    busy.value = true
    error.value = ''
    try {
      await run(found.workspace)
    } catch (err) {
      error.value = errorMessage(err, RADIO_REPORT_FAILED)
    } finally {
      busy.value = false
    }
  }

  onBeforeUnmount(() => { if (timer) clearInterval(timer) })
  return { automation, workspaces, listening, error, busy, waiting, load, setUp, analyse }
}
