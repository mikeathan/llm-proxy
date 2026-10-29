// An audit's saved report has two parts: what the code read from the scan, and the model's assessment. The first is the
// Results tab itself, so "Last result" shows only the second; text with no assessment part is returned whole.
const ASSESSMENT_HEADING = "## Assessment (written by the model)"

export function assessmentOf(output: string): string {
  const at = output.indexOf(ASSESSMENT_HEADING)
  return at < 0 ? output : output.slice(at)
}
