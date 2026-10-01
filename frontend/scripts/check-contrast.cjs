#!/usr/bin/env node
/*
 * Contrast gate for the theme presets (plan D2 "Accessibility gate"). Reads
 * every preset from src/styles/tokens.css, resolves it over the default as the
 * cascade does, and checks the WCAG 2.x pairs below. Exits non-zero on any
 * failure. Run by `npm run lint`; src/__TESTS__/theme/presets.test.ts proves
 * the same pairs from TypeScript (theme/contrastPairs.ts) — keep the two lists
 * in step.
 *
 *   node scripts/check-contrast.cjs          # summary + failures
 *   node scripts/check-contrast.cjs --all    # every pair
 *   node scripts/check-contrast.cjs --json   # every pair as JSON (for the drift test)
 *
 * Tokens are opaque "R G B" channel triples (plan D15), so no alpha compositing.
 */
'use strict'

const fs = require('node:fs')
const path = require('node:path')

const BODY_TEXT_MIN = 4.5
const NON_TEXT_MIN = 3

// Each entry: [foreground token, background token, minimum ratio, role].
// Text tiers on every surface tier, the control border on every surface, and
// every state accent on the surface it is drawn on (mirrors theme/contrastPairs.ts).
const TEXT_TOKENS = ['text-primary', 'text-secondary', 'text-muted', 'text-faint']
const SURFACE_TOKENS = ['canvas', 'surface', 'surface-raised', 'surface-hover']
const STATE_TEXT_TOKENS = [
  'accent-brand',
  'state-success',
  'state-running',
  'state-queued',
  'state-error',
  'accent-info-text',
]
// Solid fills that carry text-on-accent labels (primary / danger buttons).
const ACCENT_FILLS = ['accent-brand', 'accent-info', 'state-success', 'state-running', 'state-error']

function buildPairs() {
  const pairs = []
  for (const bg of SURFACE_TOKENS) {
    for (const fg of TEXT_TOKENS) pairs.push([fg, bg, BODY_TEXT_MIN, 'text'])
    for (const fg of STATE_TEXT_TOKENS) pairs.push([fg, bg, BODY_TEXT_MIN, 'state text'])
    pairs.push(['border-control', bg, NON_TEXT_MIN, 'control boundary'])
    pairs.push(['focus-ring', bg, NON_TEXT_MIN, 'focus indicator'])
    pairs.push(['accent-info', bg, NON_TEXT_MIN, 'fill / large text'])
  }
  pairs.push(['text-inverse', 'text-primary', BODY_TEXT_MIN, 'primary button'])
  for (const bg of ACCENT_FILLS) pairs.push(['text-on-accent', bg, BODY_TEXT_MIN, 'text on accent'])
  return pairs
}

function parseChannels(value) {
  const parts = String(value).trim().split(/\s+/).map(Number)
  if (parts.length !== 3 || parts.some((n) => !Number.isInteger(n) || n < 0 || n > 255)) {
    throw new Error(`not an "R G B" channel triple: "${value}"`)
  }
  return parts
}

function linearise(channel) {
  const c = channel / 255
  return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4)
}

function relativeLuminance([r, g, b]) {
  return 0.2126 * linearise(r) + 0.7152 * linearise(g) + 0.0722 * linearise(b)
}

function contrastRatio(a, b) {
  const la = relativeLuminance(a)
  const lb = relativeLuminance(b)
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05)
}

// evaluate: readToken resolves a token name ("canvas") to its channel string.
function evaluate(readToken) {
  return buildPairs().map(([fg, bg, min, role]) => {
    const ratio = contrastRatio(parseChannels(readToken(fg)), parseChannels(readToken(bg)))
    return { fg, bg, min, role, ratio, pass: ratio >= min }
  })
}

const TOKENS_PATH = path.resolve(__dirname, '../src/styles/tokens.css')
const BASE_THEME = 'retro-dark'
const THEMES = ['retro-dark', 'retro-dark-soft', 'retro-dark-lifted', 'retro-paper']
const BLOCK_PATTERN = /([^{}]+)\{([^{}]*)\}/g
const DECLARATION_PATTERN = /--([a-z0-9-]+)\s*:\s*([^;]+);/g

function stripComments(css) {
  return css.replace(/\/\*[\s\S]*?\*\//g, '')
}

function declarations(body) {
  const tokens = {}
  for (const [, name, value] of body.matchAll(DECLARATION_PATTERN)) tokens[name] = value.trim()
  return tokens
}

// themeBlocks maps each theme id to the custom properties declared for it,
// merged in source order as the cascade does (a later block wins).
function themeBlocks(css) {
  const blocks = {}
  for (const [, selector, body] of stripComments(css).matchAll(BLOCK_PATTERN)) {
    for (const theme of THEMES) {
      if (selector.includes(`[data-theme='${theme}']`)) blocks[theme] = { ...blocks[theme], ...declarations(body) }
    }
  }
  return blocks
}

// resolveTheme layers a preset over the default, mirroring the cascade:
// :root carries retro-dark, so a preset that omits a token inherits it.
function resolveTheme(blocks, theme) {
  return theme === BASE_THEME ? blocks[BASE_THEME] : { ...blocks[BASE_THEME], ...blocks[theme] }
}

function report(theme, results, showAll) {
  const failures = results.filter((r) => !r.pass)
  const weakest = results.reduce((a, b) => (b.ratio / b.min < a.ratio / a.min ? b : a))
  console.log(`\n${theme}: ${results.length - failures.length}/${results.length} pass` +
    ` (tightest: ${weakest.fg} on ${weakest.bg} ${weakest.ratio.toFixed(2)}:1, min ${weakest.min})`)
  for (const r of showAll ? results : failures) {
    const mark = r.pass ? 'ok  ' : 'FAIL'
    console.log(`  ${mark} ${r.ratio.toFixed(2).padStart(5)}:1 ≥${r.min}  ${r.fg} on ${r.bg}  [${r.role}]`)
  }
  return failures.length
}

function main() {
  const showAll = process.argv.includes('--all')
  const asJson = process.argv.includes('--json')
  const blocks = themeBlocks(fs.readFileSync(TOKENS_PATH, 'utf8'))
  const byTheme = {}
  let failures = 0
  for (const theme of THEMES) {
    if (!blocks[theme]) throw new Error(`tokens.css has no [data-theme='${theme}'] block`)
    const tokens = resolveTheme(blocks, theme)
    const results = evaluate((name) => {
      if (!(name in tokens)) throw new Error(`${theme} does not define --${name}`)
      return tokens[name]
    })
    byTheme[theme] = results
    if (asJson) failures += results.filter((r) => !r.pass).length
    else failures += report(theme, results, showAll)
  }
  if (asJson) console.log(JSON.stringify(byTheme))
  if (failures > 0) {
    console.error(`\n${failures} contrast pair(s) fail the accessibility gate.`)
    process.exit(1)
  }
  if (!asJson) console.log('\nAll presets pass the accessibility gate.')
}

main()
