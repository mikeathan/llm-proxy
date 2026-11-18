import { beforeEach, describe, expect, it, vi } from 'vitest'

const http = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn(), del: vi.fn() }))
vi.mock('../../../services/httpClient', () => http)

import { RadioService } from '../../../services/radio/radioService'
import { API_ENDPOINTS } from '../../../constants/api'

beforeEach(() => vi.clearAllMocks())

describe('RadioService', () => {
  it('starts a watch saying its session was sized for the focus windows too', async () => {
    http.post.mockResolvedValue({ listening: true })
    await RadioService.start('wlx00', 168)
    expect(http.post).toHaveBeenCalledWith(API_ENDPOINTS.radioListening, { adapter: 'wlx00', hours: 168, focus: true })
  })

  it('renames a device you marked with a PUT on the same path that marks it', async () => {
    http.put.mockResolvedValue({ label: 'Mac' })
    await RadioService.renameMine('2a6e3b0d', 'Mac')
    expect(http.put).toHaveBeenCalledWith(`${API_ENDPOINTS.radioMarkMine}/2a6e3b0d`, { label: 'Mac' })
  })
})
