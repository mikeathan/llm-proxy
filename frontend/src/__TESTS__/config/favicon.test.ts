import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { PRESETS } from '../../theme/presets'

// The tab icon is the brand's split disc (the sidebar's BrandMark), in the
// theme's own colours: cream + persimmon on a dark browser, ink + rust on a
// light one. These tests tie it to the brand mark and to tokens.css so the
// icon cannot drift from the app.

const ROOT = resolve(__dirname, '../../..')
const read = (path: string) => readFileSync(resolve(ROOT, path), 'utf8')

const hex = (channels: string) => '#' + channels.split(' ').map((n) => Number(n).toString(16).padStart(2, '0')).join('')
const dark = PRESETS['retro-dark'].tokens
const light = PRESETS['retro-paper'].tokens

const pathData = (source: string) => [...source.matchAll(/\sd="([^"]+)"/g)].map((m) => m[1])

describe('favicon', () => {
  const html = read('index.html')
  const iconLinks = [...html.matchAll(/<link[^>]*rel="icon"[^>]*>/g)].map((m) => m[0])

  it('is one SVG icon link that points at a real file in public/', () => {
    expect(iconLinks).toHaveLength(1)
    expect(iconLinks[0]).toContain('type="image/svg+xml"')
    const href = /href="([^"]+)"/.exec(iconLinks[0] as string)?.[1] as string
    expect(href).toBe('favicon.svg')
    expect(existsSync(resolve(ROOT, 'public', href))).toBe(true)
  })

  const svg = read('public/favicon.svg')

  it('draws exactly the two half-discs of the sidebar brand mark', () => {
    const brandMark = read('src/components/layout/BrandMark.vue')
    expect(pathData(svg)).toEqual(pathData(brandMark))
    expect(svg).toContain('viewBox="0 0 16 16"')
  })

  it('uses the dark theme colours by default and the paper colours on a light browser', () => {
    const [base, lightMedia] = svg.split('prefers-color-scheme: light')
    expect(base).toContain(`.l{fill:${hex(dark['text-primary'])}}`)
    expect(base).toContain(`.r{fill:${hex(dark['accent-brand'])}}`)
    expect(lightMedia).toContain(`.l{fill:${hex(light['text-primary'])}}`)
    expect(lightMedia).toContain(`.r{fill:${hex(light['accent-brand'])}}`)
  })

  it('has no raster image, script or external reference', () => {
    expect(svg).not.toMatch(/<image|<script|href=|url\(/)
  })
})
