import { formatBytes } from '../format/formatters'
import type { VulnDataStatus, VulnDataSummary, VulnSource, VulnSourceRow } from '../../types/netaudit'

// The vulnerability-data card in words: one status line and one action for Simple, a row per source for Advanced.
// The server derives the state from what it holds, so everything here is a function of one status.

const MS_PER_DAY = 86_400_000
/** Data older than this is called old (plan §13.G). */
export const OLD_AFTER_DAYS = 60
/** The server sends the zero date for a source that was never imported. */
const FIRST_REAL_YEAR = 2000
const PERCENT_SCALE = 1

const imported = (source: VulnSource): Date | null => {
  const when = new Date(source.imported_at)
  return Number.isNaN(when.getTime()) || when.getFullYear() < FIRST_REAL_YEAR ? null : when
}

const shortDate = (when: Date, nowMs: number): string => {
  if (new Date(nowMs).toDateString() === when.toDateString()) return 'today'
  return when.toLocaleDateString([], { month: 'short', day: 'numeric' })
}

/** When the data was last downloaded: the older of the two sources, so one stale source is not hidden by the other. */
function downloadedAt(s: VulnDataStatus): Date | null {
  const dates = [imported(s.nvd), imported(s.kev)].filter((d): d is Date => d !== null)
  return dates.length ? new Date(Math.min(...dates.map((d) => d.getTime()))) : null
}

export function vulnSummary(s: VulnDataStatus, nowMs: number = Date.now()): VulnDataSummary {
  if (s.state === 'running') return { line: `Downloading file ${s.files_done + 1} of ${s.files_total}`, action: 'cancel', old: false }
  if (s.files_done > 0 && s.files_done < s.files_total) return { line: `Stopped after ${s.files_done} of ${s.files_total} files`, action: 'resume', old: false }
  const when = downloadedAt(s)
  if (s.files_done === 0 || !when) return { line: 'Not downloaded', action: 'download', old: false }
  const old = nowMs - when.getTime() > OLD_AFTER_DAYS * MS_PER_DAY
  return old ? { line: `Old, downloaded ${shortDate(when, nowMs)}`, action: 'update', old } : { line: `Up to date, ${shortDate(when, nowMs)}`, action: null, old }
}

/** How far the file being fetched has come; the size is unknown until the server has told us. */
export function vulnProgress(s: VulnDataStatus): { fraction: number; text: string } {
  const done = s.bytes_done ?? 0
  const total = s.bytes_total ?? 0
  if (done <= 0) return { fraction: 0, text: 'Starting' }
  if (total <= 0) return { fraction: 0, text: `${formatBytes(done)} so far` }
  return { fraction: Math.min(done / total, PERCENT_SCALE), text: `${formatBytes(done)} of ${formatBytes(total)}` }
}

const count = (n: number) => n.toLocaleString('en-US')

/** "2022–2026" for a run of years, "2022, 2024" when there are gaps. */
function yearsText(years: number[]): string {
  const sorted = [...years].sort((a, b) => a - b)
  const run = sorted.every((y, i) => i === 0 || y === (sorted[i - 1] ?? y) + 1)
  return run && sorted.length > 1 ? `${sorted[0]}–${sorted[sorted.length - 1]}` : sorted.join(', ')
}

function skippedText(s: VulnSource): string | undefined {
  const parts = [
    s.no_config ? `${count(s.no_config)} not yet analysed` : '',
    s.needs_all ? `${count(s.needs_all)} needing two parts at once` : '',
    s.rejected ? `${count(s.rejected)} rejected` : '',
  ].filter(Boolean)
  return parts.length ? `Left out of the last file: ${parts.join(', ')}` : undefined
}

/** One row for each source that holds data. */
export function vulnSources(s: VulnDataStatus, nowMs: number = Date.now()): VulnSourceRow[] {
  const rows: VulnSourceRow[] = []
  const kev = imported(s.kev)
  if (kev) rows.push({ name: 'CISA exploited list', detail: `${shortDate(kev, nowMs)} · ${count(s.kev.cves)} CVEs` })
  const nvd = imported(s.nvd)
  if (nvd) {
    const years = s.nvd.years?.length ? ` ${yearsText(s.nvd.years)}` : ''
    rows.push({ name: `NVD${years}`, detail: `${shortDate(nvd, nowMs)} · ${count(s.nvd.cves)} CVEs`, skipped: skippedText(s.nvd) })
  }
  return rows
}
