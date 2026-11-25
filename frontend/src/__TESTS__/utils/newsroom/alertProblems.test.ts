import { describe, it, expect } from 'vitest'
import { alertProblems } from '../../../utils/newsroom/alertProblems'
import type { NewsroomWatch } from '../../../types/newsroom'

const w = (over: Partial<NewsroomWatch>): NewsroomWatch => ({ id: 1, name: 'AI news', enabled: true, ...over }) as NewsroomWatch

describe('alertProblems', () => {
  it('names only the watches that need a look, with a short reason', () => {
    expect(alertProblems([
      w({ id: 1, last_status: 'nothing_new' }),
      w({ id: 2, name: 'Go', last_status: 'failing' }),
      w({ id: 3, name: 'Rust', last_status: 'in_feed', last_problems: ['a: down', 'b: down'] }),
      w({ id: 4, name: 'Chips', last_status: 'no_channel' }),
      w({ id: 5, name: 'Paused', enabled: false, last_status: 'failing' }),
    ])).toEqual(['Go: source failing', 'Rust: 2 sources not answering', 'Chips: no channel'])
  })

  it('says nothing when all is well', () => {
    expect(alertProblems([w({ last_status: 'sent' }), w({ last_status: 'analysis_failed' })]).length).toBe(1)
    expect(alertProblems([w({ last_status: 'in_feed' })])).toEqual([])
  })
})
