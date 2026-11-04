import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'

const confirmMock = vi.fn()
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm: confirmMock }) }))

import UbuntuDataCard from '../../../components/netaudit/UbuntuDataCard.vue'
import { NetauditService } from '../../../services/netaudit/netauditService'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'
import type { UbuntuDataStatus } from '../../../types/netaudit'

const HELD: UbuntuDataStatus = { supported: true, host_release: 'noble', running: false, release: 'noble', notices: 981, packages: 2218, generated_at: '2026-11-04T00:46:40Z', imported_at: new Date().toISOString() }
const EMPTY: UbuntuDataStatus = { supported: true, host_release: 'noble', running: false, notices: 0, packages: 0 }

// Each card is unmounted after its test: a mounted one would react to the next test's change of view.
const mounted: VueWrapper[] = []
function mountCard() {
  const w = mount(UbuntuDataCard)
  mounted.push(w)
  return w
}

describe('UbuntuDataCard', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    confirmMock.mockReset()
    useNetauditViewMode().set('simple')
  })
  afterEach(() => mounted.splice(0).forEach((w) => w.unmount()))

  it('says it is not downloaded yet, and offers one button with the size and where from', async () => {
    vi.spyOn(NetauditService, 'ubuntuData').mockResolvedValue(EMPTY)
    const w = mountCard()
    await flushPromises()
    expect(w.get('[data-test="ubuntu-line"]').text()).toContain('Not downloaded yet')
    expect(w.get('[data-test="ubuntu-update"]').text()).toBe('Download notices')
    expect(w.find('[data-test="ubuntu-delete"]').exists()).toBe(false)
    expect(w.text()).toContain('about 1.3 MB from security-metadata.canonical.com')
    expect(w.text()).toContain('Canonical Ltd.')
  })

  it('says what is held and when it was downloaded, and offers Update and Delete', async () => {
    vi.spyOn(NetauditService, 'ubuntuData').mockResolvedValue(HELD)
    const w = mountCard()
    await flushPromises()
    expect(w.get('[data-test="ubuntu-line"]').text()).toMatch(/981 security notices for Ubuntu noble, downloaded/)
    expect(w.get('[data-test="ubuntu-update"]').text()).toBe('Update notices')
    expect(w.find('[data-test="ubuntu-delete"]').exists()).toBe(true)
    expect(w.text()).toContain('Ubuntu Pro')
  })

  it('says a computer that is not Ubuntu has nothing to fetch, and offers no button', async () => {
    vi.spyOn(NetauditService, 'ubuntuData').mockResolvedValue({ supported: false, running: false, notices: 0, packages: 0 })
    const w = mountCard()
    await flushPromises()
    expect(w.get('[data-test="ubuntu-unsupported"]').text()).toContain('not running Ubuntu')
    expect(w.find('[data-test="ubuntu-update"]').exists()).toBe(false)
  })

  it('updates once per click and shows the new state', async () => {
    vi.spyOn(NetauditService, 'ubuntuData').mockResolvedValue(EMPTY)
    let finish!: (s: UbuntuDataStatus) => void
    const update = vi.spyOn(NetauditService, 'updateUbuntuData').mockImplementation(() => new Promise((resolve) => { finish = resolve }))
    const w = mountCard()
    await flushPromises()
    await w.get('[data-test="ubuntu-update"]').trigger('click')
    await w.get('[data-test="ubuntu-update"]').trigger('click')
    expect(update).toHaveBeenCalledTimes(1)
    finish(HELD)
    await flushPromises()
    expect(w.get('[data-test="ubuntu-line"]').text()).toContain('981 security notices')
  })

  it('shows the server\'s words when an update fails, and keeps what was held', async () => {
    vi.spyOn(NetauditService, 'ubuntuData').mockResolvedValue(HELD)
    vi.spyOn(NetauditService, 'updateUbuntuData').mockRejectedValue(new Error('The notices could not be downloaded. Check the connection and the Guardrails internet switch, then try again.'))
    const w = mountCard()
    await flushPromises()
    await w.get('[data-test="ubuntu-update"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="ubuntu-error"]').text()).toContain('Guardrails internet switch')
    expect(w.get('[data-test="ubuntu-line"]').text()).toContain('981 security notices')
  })

  it('asks before deleting, and deletes only when confirmed', async () => {
    vi.spyOn(NetauditService, 'ubuntuData').mockResolvedValue(HELD)
    const clear = vi.spyOn(NetauditService, 'clearUbuntuData').mockResolvedValue(EMPTY)
    const w = mountCard()
    await flushPromises()
    confirmMock.mockResolvedValueOnce(false)
    await w.get('[data-test="ubuntu-delete"]').trigger('click')
    await flushPromises()
    expect(clear).not.toHaveBeenCalled()
    confirmMock.mockResolvedValueOnce(true)
    await w.get('[data-test="ubuntu-delete"]').trigger('click')
    await flushPromises()
    expect(clear).toHaveBeenCalledTimes(1)
    expect(w.get('[data-test="ubuntu-line"]').text()).toContain('Not downloaded yet')
  })

  it('shows the counts and the build date only in the Advanced view', async () => {
    vi.spyOn(NetauditService, 'ubuntuData').mockResolvedValue(HELD)
    const w = mountCard()
    await flushPromises()
    expect(w.find('[data-test="ubuntu-details"]').exists()).toBe(false)
    useNetauditViewMode().set('advanced')
    await flushPromises()
    const details = w.get('[data-test="ubuntu-details"]').text()
    expect(details).toContain('2,218')
    expect(details).toContain('2026-11-04')
  })

  it('says old data is old', async () => {
    vi.spyOn(NetauditService, 'ubuntuData').mockResolvedValue({ ...HELD, imported_at: new Date(Date.now() - 20 * 86_400_000).toISOString() })
    const w = mountCard()
    await flushPromises()
    expect(w.get('[data-test="ubuntu-old"]').text()).toContain('more than two weeks old')
  })

  it('says it could not read, and offers a retry', async () => {
    const spy = vi.spyOn(NetauditService, 'ubuntuData').mockRejectedValueOnce(new Error('down'))
    const w = mountCard()
    await flushPromises()
    expect(w.text()).toContain('could not be checked')
    spy.mockResolvedValueOnce(HELD)
    await w.get('button').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="ubuntu-line"]').text()).toContain('981 security notices')
  })
})
