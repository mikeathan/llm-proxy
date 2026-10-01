import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { useConfirm } from '../../../../composables/ui/useConfirm'
import { ref } from 'vue'
import type { Automation, RecordingMeta } from '../../../../types/dispatcher'

const rec = {
  recordings: ref<RecordingMeta[]>([]),
  status: ref({ enabled: true, dir: '/data/recordings' }),
  loading: ref(false),
  fetchStatus: vi.fn(),
  fetchRecordings: vi.fn(),
  deleteRecording: vi.fn(),
  clearAutomationRecordingRef: vi.fn(),
}
vi.mock('../../../../composables/automation/useRecordings', () => ({ useRecordings: () => rec }))

import RecordingsPanel from '../../../../components/AgentIde/recordings/RecordingsPanel.vue'

const R = (id: string, minute: number): RecordingMeta =>
  ({ id, model: 'qwen', automation_name: 'nightly', timestamp: `2026-09-29T10:0${minute}:00Z`, file_path: `/r/${id}`, file_size: 2048 } as RecordingMeta)
const AUTO = { id: 'ws/nightly', workspace: 'ws', name: 'nightly', task_file: 't.md', strategy: 'persistent', trigger: 'manual' } as Automation

function mountPanel(automations: Automation[] = [AUTO]) {
  return mount(RecordingsPanel, { props: { automations, workspaces: ['ws'] } })
}
async function expand(w: ReturnType<typeof mountPanel>) {
  await w.findAll('[data-test="recording-group"]')[0]!.trigger('click')
}
const btn = (w: ReturnType<typeof mountPanel>, text: string) => w.findAll('button').find((b) => b.text().includes(text))

// Characterised before the restyle (plan D22); deleting now confirms first
// (Phase 5: every destructive action goes through ConfirmDialog).
describe('RecordingsPanel', () => {
  beforeEach(() => {
    rec.recordings.value = [R('rec-old', 1), R('rec-new', 5)]
    rec.status.value = { enabled: true, dir: '/data/recordings' }
    Object.values(rec).forEach((v) => typeof v === 'function' && v.mockReset())
  })

  it('reports the recording mode and loads recordings when it is on', async () => {
    const w = mountPanel()
    await flushPromises()
    expect(w.text()).toContain('/data/recordings')
    expect(rec.fetchRecordings).toHaveBeenCalled()
  })

  it('lists an automation\'s recordings newest first and replays one, one at a time', async () => {
    const w = mountPanel()
    await flushPromises()
    await expand(w)
    const items = w.findAll('[data-test="recording"]')
    expect(items).toHaveLength(2)
    await btn(w, 'Replay')!.trigger('click')
    expect(w.emitted('replay-recording')).toEqual([[AUTO, rec.recordings.value[1]]])
    expect(btn(w, 'Replay')!.attributes('disabled')).toBeDefined()
  })

  it('switches an automation back to the live model and deletes a recording', async () => {
    const w = mountPanel([{ ...AUTO, recording_ref: 'rec-new' }])
    await flushPromises()
    await expand(w)
    await btn(w, 'Use live model')!.trigger('click')
    await flushPromises()
    expect(rec.clearAutomationRecordingRef).toHaveBeenCalledWith('ws', 'nightly')
    await w.findAll('button[aria-label^="Delete recording"]')[0]!.trigger('click')
    await flushPromises()
    expect(rec.deleteRecording).not.toHaveBeenCalled()
    useConfirm().handleConfirm()
    await flushPromises()
    expect(rec.deleteRecording).toHaveBeenCalledWith('rec-new')
  })

  it('says when there are no recordings', async () => {
    rec.recordings.value = []
    const w = mountPanel()
    await flushPromises()
    expect(w.text()).toContain('No recordings')
  })
})
