import type { AuditReportSection, AuditReportView } from "../../types/auditReport"
import { ASSESSMENT_HEADING } from "./assessment"

// An audit's saved report is the code-written "Observed" section (a heading per part) and the model's assessment. This cuts it at those
// headings so a viewer can show the assessment first, keep what the operator acts on open, and fold the reference parts behind a one-line
// summary. Text that is not such a report gives null and is shown as plain markdown, exactly as before.
const OBSERVED_HEADING = "## Observed (generated from the scan data, not written by the model)"
const SECTION_MARK = /^### (.+)$/
const NOTE_MARK = "> Note:"
const FIXES_MARK = "\nSuggested fixes:"
const RULE = /\n---\s*$/
const TABLE_HEADER_ROWS = 2
const FIRST_SENTENCE = /^(.+?[.!?])(\s|$)/

/** Parts that carry what the operator acts on. A part this file does not know is shown too: nothing is hidden by surprise. */
const OPEN_TITLES = new Set(["New", "New checks", "Changed", "Still open", "Checks", "Ways in that matter"])
const FOLDED_TITLES = new Set(["Resolved", "Resolved checks", "Exposed services worth reviewing", "Open ports"])
const FOLDED_PREFIX = "Last "
const ROW_NOUNS: Record<string, readonly [string, string]> = {
  Resolved: ["port", "ports"],
  "Resolved checks": ["problem", "problems"],
  "Exposed services worth reviewing": ["service", "services"],
  "Open ports": ["device", "devices"],
}
const FIXES_NOUN: readonly [string, string] = ["fix", "fixes"]
const NOTES_NOUN: readonly [string, string] = ["note", "notes"]

const plural = (n: number, [one, many]: readonly [string, string]) => `${n} ${n === 1 ? one : many}`
const tableRows = (text: string) => Math.max(0, text.split("\n").filter((l) => l.startsWith("|")).length - TABLE_HEADER_ROWS)
const listItems = (text: string) => text.split("\n").filter((l) => /^(- |\d+\. )/.test(l)).length

/** The noun and count a folded part is summed up by, or "" when it has none. */
function summaryOf(title: string, body: string): string {
  if (title in ROW_NOUNS) return plural(tableRows(body), ROW_NOUNS[title]!)
  if (title.startsWith(FOLDED_PREFIX)) return FIRST_SENTENCE.exec(body.trim())?.[1] ?? ""
  return ""
}

const isOpen = (title: string) => OPEN_TITLES.has(title) || !(FOLDED_TITLES.has(title) || title.startsWith(FOLDED_PREFIX))

function splitFixes(body: string): [string, string] {
  const at = body.indexOf(FIXES_MARK)
  return at < 0 ? [body.trim(), ""] : [body.slice(0, at).trim(), body.slice(at + FIXES_MARK.length).trim()]
}

/** Lifts the "Note:" paragraphs out of a piece of text; the rest stays where it was. */
function liftNotes(text: string): { rest: string; notes: string[] } {
  const notes: string[] = []
  const kept = text.split(/\n{2,}/).filter((paragraph) => {
    const trimmed = paragraph.trim()
    if (!trimmed.startsWith(NOTE_MARK)) return true
    notes.push(trimmed.replace(/^> /gm, "").trim())
    return false
  })
  return { rest: kept.join("\n\n").trim(), notes }
}

function sectionsOf(observed: string): { lead: string; parts: { title: string; text: string }[] } {
  const lead: string[] = []
  const parts: { title: string; text: string[] }[] = []
  for (const line of observed.split("\n")) {
    const title = SECTION_MARK.exec(line)?.[1]
    if (title) parts.push({ title: title.trim(), text: [] })
    else (parts.length ? parts[parts.length - 1]!.text : lead).push(line)
  }
  return { lead: lead.join("\n"), parts: parts.map((p) => ({ title: p.title, text: p.text.join("\n") })) }
}

export function parseAuditReport(text: string): AuditReportView | null {
  const at = text.indexOf(OBSERVED_HEADING)
  if (at < 0) return null
  const assessmentAt = text.indexOf(ASSESSMENT_HEADING)
  const observedEnd = assessmentAt < 0 ? text.length : assessmentAt
  const observed = text.slice(at + OBSERVED_HEADING.length, observedEnd).replace(RULE, "")
  const assessment = assessmentAt < 0 ? "" : text.slice(assessmentAt + ASSESSMENT_HEADING.length).trim()

  const cut = sectionsOf(observed)
  const notes: string[] = []
  const take = (piece: string) => {
    const lifted = liftNotes(piece)
    notes.push(...lifted.notes)
    return lifted.rest
  }
  const lead = take(cut.lead)
  const sections: AuditReportSection[] = cut.parts.map((part, index) => {
    const [body, fixes] = splitFixes(take(part.text))
    return { id: `${index}-${part.title}`, title: part.title, body, fixes, summary: summaryOf(part.title, body), open: isOpen(part.title) }
  })
  return { preface: text.slice(0, at).trim(), lead, sections, notes, assessment }
}

/** The closed line of a section's fixes, and of the notes. */
export const fixesSummary = (fixes: string) => plural(listItems(fixes), FIXES_NOUN)
export const notesSummary = (notes: readonly string[]) => plural(notes.length, NOTES_NOUN)
