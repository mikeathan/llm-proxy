import { readFileSync } from 'node:fs'
import assert from 'node:assert/strict'

const THRESHOLD = 30
const TOP_COUNT = 10

function summarize(report, language) {
  const source = language === 'Go' ? report.functions : report.methods
  if (!Array.isArray(source)) throw new Error(`Missing ${language} function report`)
  const rows = source.map((item) => ({
    file: item.file,
    line: language === 'Go' ? item.line : item.startLine,
    name: language === 'Go' ? item.func : item.name,
    complexity: item.complexity,
    coverage: language === 'Go' ? item.coverage : item.coveragePercent,
    score: item.crap,
  }))
  for (const row of rows) {
    if (typeof row.file !== 'string' || typeof row.name !== 'string' ||
        !Number.isFinite(row.line) || !Number.isFinite(row.complexity) ||
        !Number.isFinite(row.score) || (row.coverage !== null && !Number.isFinite(row.coverage))) {
      throw new Error(`Invalid ${language} function report`)
    }
  }
  const above = rows.filter((row) => row.score > THRESHOLD)
    .sort((a, b) => b.score - a.score || a.file.localeCompare(b.file) || a.line - b.line)
  const maximum = rows.reduce((max, row) => Math.max(max, row.score), 0)
  const lines = [
    `#### ${language}`,
    '',
    `${rows.length} functions scored; **${above.length} above ${THRESHOLD}**; highest score **${maximum.toFixed(2)}**.`,
    '',
  ]
  if (above.length === 0) return [...lines, `No functions exceed ${THRESHOLD}.`, ''].join('\n')
  lines.push(`Highest ${Math.min(TOP_COUNT, above.length)} flagged functions:`, '',
    '| Function | Location | Complexity | Coverage | CRAP |',
    '| --- | --- | ---: | ---: | ---: |')
  for (const row of above.slice(0, TOP_COUNT)) {
    const coverage = row.coverage === null ? 'unknown' : `${row.coverage.toFixed(1)}%`
    lines.push(`| ${escapeCell(row.name)} | ${escapeCell(`${row.file}:${row.line}`)} | ${row.complexity} | ${coverage} | ${row.score.toFixed(2)} |`)
  }
  lines.push('', 'Missing coverage is scored as 0%. Full reports are in the `crap-reports` artifact.', '')
  return lines.join('\n')
}

function escapeCell(value) {
  return value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/\|/g, '&#124;').replace(/`/g, '&#96;').replace(/\[/g, '&#91;')
    .replace(/\]/g, '&#93;').replace(/\*/g, '&#42;').replace(/_/g, '&#95;')
    .replace(/[\r\n]/g, ' ')
}

function selfTest() {
  const go = { functions: [
    { file: 'llm-proxy/a.go', line: 4, func: 'low', complexity: 5, coverage: 0, crap: 30 },
    { file: 'llm-proxy/b.go', line: 8, func: 'high', complexity: 10, coverage: 0, crap: 110 },
  ] }
  const output = summarize(go, 'Go')
  assert.match(output, /2 functions scored; \*\*1 above 30\*\*/)
  assert.match(output, /110\.00/)
  assert.ok(!output.includes('low'))
  const ts = { methods: [
    { file: 'src/a.ts', startLine: 1, name: '<script>|risk', complexity: 6, coveragePercent: null, crap: 42 },
  ] }
  const tsOutput = summarize(ts, 'TypeScript')
  assert.ok(tsOutput.includes('&lt;script&gt;&#124;risk'))
  assert.ok(tsOutput.includes('unknown'))
  assert.ok(!tsOutput.includes('<script>'))
  assert.match(summarize({ methods: [] }, 'TypeScript'), /No functions exceed 30/)
  assert.throws(() => summarize({}, 'Go'))
  const unordered = { functions: Array.from({ length: 12 }, (_, index) => ({
    file: 'a.go', line: index + 1, func: `risk${index}`, complexity: 10, coverage: 0, crap: 100 + index,
  })) }
  const ranked = summarize(unordered, 'Go')
  assert.ok(ranked.indexOf('risk11') < ranked.indexOf('risk10'))
  assert.ok(!ranked.includes('| risk0 |'))
  console.log('PASS: CRAP threshold, ordering, unknown coverage, escaping and empty reports')
}

if (process.argv[2] === '--self-test') {
  selfTest()
} else {
  const [goPath, tsPath] = process.argv.slice(2)
  if (!goPath || !tsPath) throw new Error('Usage: node scripts/crap-summary.mjs GO_JSON TS_JSON')
  console.log('### CRAP scores\n\nReport only. Scores above 30 are flagged; lower is better.\n')
  console.log(summarize(JSON.parse(readFileSync(goPath, 'utf8')), 'Go'))
  console.log(summarize(JSON.parse(readFileSync(tsPath, 'utf8')), 'TypeScript'))
}
