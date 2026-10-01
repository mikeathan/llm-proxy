import { describe, expect, it } from 'vitest'
import { channelsToHex, contrastRatio, formatChannels, parseColour } from '../../theme/colour'

describe('parseColour', () => {
  it.each([
    ['#fff', [255, 255, 255]],
    ['#FF6A3D', [255, 106, 61]],
    ['255 106 61', [255, 106, 61]],
    ['rgb(255, 106, 61)', [255, 106, 61]],
    ['rgb(255 106 61)', [255, 106, 61]],
    ['hsl(0, 100%, 50%)', [255, 0, 0]],
    ['hsl(120 100% 25%)', [0, 128, 0]],
    ['  #0c0b0a  ', [12, 11, 10]],
  ])('accepts %s', (input, expected) => {
    expect(parseColour(input)).toEqual(expected)
  })

  it.each([
    '#ffff', // alpha shorthand
    '#ff6a3d80', // alpha
    'rgba(255, 106, 61, 0.5)',
    'rgb(255 106 61 / 50%)',
    'hsla(0, 100%, 50%, 1)',
    'rgb(256, 0, 0)',
    '255 106',
    'red',
    'url(x)',
    'var(--canvas)',
    '',
  ])('rejects %s', (input) => {
    expect(parseColour(input)).toBeNull()
  })
})

describe('channel formatting', () => {
  it('formats the canonical stored form and hex for export', () => {
    expect(formatChannels([12, 11, 10])).toBe('12 11 10')
    expect(channelsToHex([255, 106, 61])).toBe('#ff6a3d')
  })
})

describe('contrastRatio', () => {
  it('matches known WCAG values', () => {
    expect(contrastRatio([0, 0, 0], [255, 255, 255])).toBeCloseTo(21, 5)
    expect(contrastRatio([255, 255, 255], [255, 255, 255])).toBeCloseTo(1, 5)
    // Design-language table: persimmon on the warm-ink canvas.
    expect(contrastRatio([255, 106, 61], [12, 11, 10])).toBeCloseTo(6.91, 2)
  })
})
