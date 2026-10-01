import { execFileSync } from 'node:child_process'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { CONTRAST_PAIRS, evaluateContrast } from '../../theme/contrastPairs'
import { PRESETS, PRESET_IDS } from '../../theme/presets'

// scripts/check-contrast.cjs (run by `npm run lint`) keeps its own copy of the
// pair list so it can run without the app bundle. This proves the copy has not
// drifted from theme/contrastPairs.ts: same pairs, same minimums, same results.

interface ScriptResult {
  fg: string
  bg: string
  min: number
  role: string
  ratio: number
  pass: boolean
}

const SCRIPT = resolve(__dirname, '../../../scripts/check-contrast.cjs')
const scriptResults = JSON.parse(execFileSync(process.execPath, [SCRIPT, '--json'], { encoding: 'utf8' })) as Record<
  string,
  ScriptResult[]
>

const resultsFor = (id: string): ScriptResult[] => {
  const results = scriptResults[id]
  if (!results) throw new Error(`check-contrast.cjs reported nothing for ${id}`)
  return results
}

const key = (r: { fg: string; bg: string; min: number; role: string }) => `${r.fg}|${r.bg}|${r.min}|${r.role}`

describe('scripts/check-contrast.cjs', () => {
  it('checks exactly the pairs in theme/contrastPairs.ts', () => {
    const expected = CONTRAST_PAIRS.map(key).sort()
    for (const id of PRESET_IDS) expect(resultsFor(id).map(key).sort()).toEqual(expected)
  })

  it.each(PRESET_IDS)('%s: matches the TypeScript ratios', (id) => {
    const ts = new Map(evaluateContrast(PRESETS[id].tokens).map((r) => [key(r), r.ratio]))
    for (const r of resultsFor(id)) expect(r.ratio).toBeCloseTo(ts.get(key(r)) ?? Number.NaN, 6)
  })
})
