/** One part of an audit report's code-written "Observed" section, cut at its heading. */
export interface AuditReportSection {
  id: string
  title: string
  /** The section's markdown, without its suggested fixes (those are in `fixes`). */
  body: string
  /** The suggested fixes written under the section's table, or "" when it has none. */
  fixes: string
  /** What the folded section says on its closed line (a count, or the first sentence). */
  summary: string
  /** True for a part the operator acts on: it is shown, never folded away. */
  open: boolean
}

/** An audit report cut into the parts the viewer shows: the assessment first, then the code-written sections. */
export interface AuditReportView {
  /** Text before the report itself (a saved run's header lines), or "". */
  preface: string
  /** The opening lines of the Observed section: what was scanned and how it compares with the scan before. */
  lead: string
  sections: AuditReportSection[]
  /** The "Note:" lines the code added (what could not be read, and the like), without their marker. */
  notes: string[]
  /** The model's assessment, without its heading, or "". */
  assessment: string
}
