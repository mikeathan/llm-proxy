import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const service = vi.hoisted(() => ({ ownerDevices: vi.fn(), addOwnerDevice: vi.fn(), removeOwnerDevice: vi.fn() }))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))
const confirm = vi.fn()
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm }) }))

import RadioManageDevices from '../../../components/radio/RadioManageDevices.vue'

const A = 'aaaaaaaa'.padEnd(64, '0')
const device = (id: string, label: string) => ({ id, label, visits: 3, first_seen: '2026-11-10', last_seen: '2026-11-13', class: 'personal', class_by: 'rules' })

async function show() {
  const w = mount(RadioManageDevices)
  await flushPromises()
  return w
}

beforeEach(() => vi.clearAllMocks())

describe('RadioManageDevices (the control side of My devices)', () => {
  it('lists the devices marked as yours by name, and removes one only after you confirm', async () => {
    service.ownerDevices.mockResolvedValue([device(A, 'Laptop')])
    service.removeOwnerDevice.mockResolvedValue(undefined)
    const w = await show()
    expect(w.findAll('[data-test="radio-manage-name"]').map((n) => n.text())).toEqual(['Laptop'])
    confirm.mockResolvedValueOnce(false)
    await w.get('[data-test="radio-my-device-remove"]').trigger('click')
    await flushPromises()
    expect(service.removeOwnerDevice).not.toHaveBeenCalled()
    confirm.mockResolvedValueOnce(true)
    await w.get('[data-test="radio-my-device-remove"]').trigger('click')
    await flushPromises()
    expect(confirm).toHaveBeenLastCalledWith(expect.objectContaining({ message: expect.stringContaining('“Laptop” stops being marked as yours') }))
    expect(service.removeOwnerDevice).toHaveBeenCalledWith(A)
  })

  it('keeps adding by address folded away, and shows the server’s plain refusal with what was typed', async () => {
    service.ownerDevices.mockResolvedValue([])
    const { HttpError } = await import('../../../services/httpClient')
    service.addOwnerDevice.mockRejectedValue(new HttpError('an address is six pairs of digits', undefined, 400))
    const w = await show()
    expect(w.get('[data-test="radio-devices-empty"]').text()).toContain('Find my device')
    expect(w.find('[data-test="radio-add-form"]').exists()).toBe(false)
    await w.get('[data-test="radio-add-open"]').trigger('click')
    await w.get('[data-test="radio-device-label"]').setValue('tv')
    await w.get('[data-test="radio-device-address"]').setValue('nope')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(service.addOwnerDevice).toHaveBeenCalledWith('nope', 'tv')
    expect(w.get('[data-test="radio-devices-error"]').text()).toBe('an address is six pairs of digits')
    expect((w.get('[data-test="radio-device-address"]').element as HTMLInputElement).value).toBe('nope')
  })
})
