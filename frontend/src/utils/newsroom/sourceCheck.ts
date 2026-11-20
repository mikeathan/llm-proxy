import { HN_QUERY_MAX, SOURCE_PROBLEMS } from '../../constants/newsroom'
import type { NewsroomSource } from '../../types/newsroom'

// The same rules the server applies to a source (stacks/newsroom/sources/validate.go), so a mistake shows while typing. The server still decides.
const CHANNEL = /^[A-Za-z][A-Za-z0-9_]{3,31}$/
const TAG = /^[\p{L}\p{N}_]{1,100}$/u
const HOST = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$/

/** A pasted Telegram link or @name reduced to the bare channel name. */
export function channelName(s: string): string {
  let v = s.trim().replace(/^https?:\/\//, '').replace(/^(telegram\.me|t\.me)\//, '').replace(/^s\//, '')
  v = v.split(/[/?#]/)[0] ?? ''
  return v.replace(/^@/, '')
}

function hostName(s: string): string {
  return (s.trim().toLowerCase().replace(/^https?:\/\//, '').split(/[/?#]/)[0]) ?? ''
}

function isHttps(addr: string): boolean {
  try {
    const u = new URL(addr)
    return u.protocol === 'https:' && u.hostname !== '' && u.username === ''
  } catch {
    return false
  }
}

/** The problem with a source's target (or server), in words, or '' when it is fine or still empty. */
export function sourceProblem(source: NewsroomSource): string {
  const target = source.target.trim()
  if (target === '' && source.kind !== 'mastodon') return ''
  switch (source.kind) {
    case 'feed':
    case 'page':
      return isHttps(target) ? '' : SOURCE_PROBLEMS.address
    case 'hackernews':
      return [...target].length <= HN_QUERY_MAX ? '' : SOURCE_PROBLEMS.query
    case 'telegram':
      return CHANNEL.test(channelName(target)) ? '' : SOURCE_PROBLEMS.channel
    case 'mastodon': {
      if (target === '' && !(source.instance ?? '').trim()) return ''
      if (!(source.instance ?? '').trim()) return SOURCE_PROBLEMS.instanceMissing
      if (!HOST.test(hostName(source.instance ?? ''))) return SOURCE_PROBLEMS.instance
      return TAG.test(target.replace(/^#/, '')) ? '' : SOURCE_PROBLEMS.tag
    }
  }
}
