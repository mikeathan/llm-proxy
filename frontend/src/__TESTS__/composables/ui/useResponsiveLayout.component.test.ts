import { describe, it, expect, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { useResponsiveLayout } from '../../../composables/ui/useResponsiveLayout'
import { BREAKPOINTS } from '../../../theme/breakpoints'
import { screens } from '../../../../theme/tailwindTokens'
import { fakeMatchMedia } from '../../helpers/fakeMatchMedia'

function mountAt(width: number) {
  const media = fakeMatchMedia(width)
  vi.stubGlobal('matchMedia', media.matchMedia)
  let api!: ReturnType<typeof useResponsiveLayout>
  const Host = defineComponent({
    setup() {
      api = useResponsiveLayout()
      return () => h('div')
    },
  })
  const wrapper = mount(Host)
  return { wrapper, media, api: () => api }
}

describe('useResponsiveLayout', () => {
  afterEach(() => vi.unstubAllGlobals())

  it.each([
    [360, 'base', true],
    [640, 'sm', true],
    [800, 'md', true],
    [1024, 'lg', false],
    [1300, 'xl', false],
  ])('at %ipx reports %s (mobile: %s)', (width, breakpoint, mobile) => {
    const { api } = mountAt(width)
    expect(api().breakpoint.value).toBe(breakpoint)
    expect(api().isMobile.value).toBe(mobile)
  })

  it('follows viewport changes', () => {
    const { api, media } = mountAt(1440)
    media.resize(700)
    expect(api().breakpoint.value).toBe('sm')
    expect(api().isMobile.value).toBe(true)
  })

  it('removes every media listener on unmount', () => {
    const { wrapper, media } = mountAt(1440)
    expect(media.listenerCount()).toBeGreaterThan(0)
    wrapper.unmount()
    expect(media.listenerCount()).toBe(0)
  })

  it('reads the same breakpoints as the Tailwind screens', () => {
    const px = Object.fromEntries(Object.entries(BREAKPOINTS).map(([name, value]) => [name, `${value}px`]))
    expect(screens).toEqual(px)
  })
})
