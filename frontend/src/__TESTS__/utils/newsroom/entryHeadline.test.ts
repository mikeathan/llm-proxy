import { describe, expect, it } from 'vitest'
import { entryHeadline, fallbackHeadline } from '../../../utils/newsroom/entryHeadline'

describe('entryHeadline', () => {
  it('prefers the model headline', () => {
    expect(entryHeadline({ title: 'Long original title. More.', headline: 'Short' })).toBe('Short')
  })

  it('falls back to the first sentence, without emoji', () => {
    const title = '🚀 DeepSeek-V4.1-Flash-1017 is here! Stronger benchmark performance, faster task completion, and shorter outputs than the previous version—a Pareto improvement in performance and cost. Now live on our…'
    expect(entryHeadline({ title })).toBe('DeepSeek-V4.1-Flash-1017 is here!')
  })

  it('drops links and cuts a long run-on post at a word', () => {
    const out = fallbackHeadline('🔓 Open-source release Get DeepSeek-V4.1-Flash-1017: 🤗 Hugging Face: https://huggingface.co/deepseek-ai/DeepSeek-V4.1-Flash-1017 🔗 ModelScope: https://modelscope.cn/models/x')
    expect(out).not.toContain('http')
    expect(out).toBe('Open-source release Get DeepSeek-V4.1-Flash-1017: Hugging Face: ModelScope:')
    const long = fallbackHeadline('word '.repeat(60))
    expect(long.length).toBeLessThanOrEqual(91)
    expect(long.endsWith('…')).toBe(true)
  })

  it('keeps the title when nothing readable is left', () => {
    expect(fallbackHeadline('🚀🚀')).toBe('🚀🚀')
  })
})
