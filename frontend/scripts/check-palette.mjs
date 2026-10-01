#!/usr/bin/env node
/*
 * Palette ratchet (plan D21, V10 amendment). Raw Tailwind palette classes
 * (`bg-gray-800`, `hover:text-blue-400/50`, …) are being migrated to semantic
 * token classes. ESLint's vue/no-restricted-class sees template classes only,
 * but 84% of the debt sits in `@apply`, so this dependency-free scan covers
 * templates, <style> @apply and TS string literals alike.
 *
 * The allowlist (scripts/palette-allowlist.json) records how many palette
 * classes each file may still contain. The check fails when a file exceeds its
 * count, a file not listed gains any, or a count is higher than needed — so the
 * number only ever goes down. After migrating a file, lower its count with:
 *
 *   node scripts/check-palette.mjs --update
 *
 * `--update` refuses to raise any count. `--seed` records the starting
 * baseline and works only when no allowlist exists. Since the legacy palette
 * bridge was deleted (final Phase 5 PR) the allowlist is empty (`{}`): no file
 * may contain a palette class.
 */
import { existsSync, readdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = fileURLToPath(new URL('..', import.meta.url))
const SRC = join(ROOT, 'src')
const ALLOWLIST = join(ROOT, 'scripts', 'palette-allowlist.json')
const EXTENSIONS = ['.vue', '.ts', '.css']
const SKIP_DIRS = new Set(['__TESTS__'])
const FAMILIES =
  'gray|slate|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose|white|black'
const UTILITIES =
  'bg|text|border(?:-[trblxy])?|ring|ring-offset|divide(?:-[xy])?|placeholder|from|to|via|outline|fill|stroke|shadow|accent|caret|decoration'
const PALETTE_CLASS = new RegExp(
  String.raw`(?<![\w-])(?:[a-z0-9-]+:)*(?:${UTILITIES})-(?:${FAMILIES})(?:-\d{2,3})?(?:\/\d+)?(?![\w-])`,
  'g',
)

function* sourceFiles(dir) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (entry.isDirectory()) {
      if (!SKIP_DIRS.has(entry.name)) yield* sourceFiles(join(dir, entry.name))
    } else if (EXTENSIONS.some((ext) => entry.name.endsWith(ext))) {
      yield join(dir, entry.name)
    }
  }
}

function countUsage() {
  const counts = {}
  for (const file of sourceFiles(SRC)) {
    const n = readFileSync(file, 'utf8').match(PALETTE_CLASS)?.length ?? 0
    if (n > 0) counts[relative(ROOT, file).split('\\').join('/')] = n
  }
  return Object.fromEntries(Object.entries(counts).sort(([a], [b]) => a.localeCompare(b)))
}

function readAllowlist() {
  try {
    return JSON.parse(readFileSync(ALLOWLIST, 'utf8'))
  } catch {
    return {}
  }
}

function writeAllowlist(counts, verb) {
  writeFileSync(ALLOWLIST, `${JSON.stringify(counts, null, 2)}\n`)
  console.log(`check-palette: allowlist ${verb} (${sum(counts)} palette classes in ${Object.keys(counts).length} files).`)
}

function main() {
  const actual = countUsage()
  if (process.argv.includes('--seed')) {
    if (existsSync(ALLOWLIST)) {
      console.error('check-palette: --seed only creates a missing allowlist; use --update to lower counts.')
      process.exit(1)
    }
    writeAllowlist(actual, 'seeded')
    return
  }
  const allowed = readAllowlist()
  const raised = Object.entries(actual).filter(([file, n]) => n > (allowed[file] ?? 0))

  if (process.argv.includes('--update')) {
    if (raised.length) {
      for (const [file, n] of raised) console.error(`  ${file}: ${allowed[file] ?? 0} → ${n}`)
      console.error('check-palette: --update never raises a count; use semantic token classes instead.')
      process.exit(1)
    }
    writeAllowlist(actual, 'updated')
    return
  }

  const stale = Object.entries(allowed).filter(([file, n]) => (actual[file] ?? 0) < n)
  for (const [file, n] of raised) {
    console.error(`  ${file}: ${n} palette classes (allowed ${allowed[file] ?? 0}) — use semantic token classes`)
  }
  for (const [file, n] of stale) {
    console.error(`  ${file}: allowlist says ${n}, file has ${actual[file] ?? 0} — run: node scripts/check-palette.mjs --update`)
  }
  if (raised.length || stale.length) process.exit(1)
  console.log(`check-palette: ok (${sum(actual)} legacy palette classes left in ${Object.keys(actual).length} files).`)
}

function sum(counts) {
  return Object.values(counts).reduce((a, b) => a + b, 0)
}

main()
