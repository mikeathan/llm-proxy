import { describe, expect, it } from 'vitest'
import { assessmentOf } from '../../../utils/netaudit/assessment'

const OBSERVED = '## Observed (generated from the scan data, not written by the model)\n\nScanned 254 addresses.'
const ASSESSMENT = '## Assessment (written by the model)\n\nSamba 4.6.2 is old.'

describe('assessmentOf', () => {
  it('keeps only the model assessment of an audit report, since the scan data is shown as the report', () => {
    expect(assessmentOf(`### Final Report\n\n${OBSERVED}\n\n---\n\n${ASSESSMENT}`)).toBe(ASSESSMENT)
  })

  it('returns the whole text when it has no assessment part', () => {
    expect(assessmentOf('The run failed to start.')).toBe('The run failed to start.')
  })
})
