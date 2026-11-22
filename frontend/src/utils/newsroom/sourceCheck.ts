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

function mastodonProblem(source: NewsroomSource): string {
  const target = source.target.trim()
  const instance = (source.instance ?? '').trim()
  if (target === '' && instance === '') return ''
  if (instance === '') return SOURCE_PROBLEMS.instanceMissing
  if (!HOST.test(hostName(instance))) return SOURCE_PROBLEMS.instance
  return TAG.test(target.replace(/^#/, '')) ? '' : SOURCE_PROBLEMS.tag
}

const REGION = /^[a-z]{2}-[A-Z]{2}$/

/** The problem with a news search's region, or '' when it is fine or empty (empty means the default). */
export function regionProblem(source: NewsroomSource): string {
  const region = (source.instance ?? '').trim()
  return region === '' || REGION.test(region) ? '' : SOURCE_PROBLEMS.region
}

const addressProblem = (t: string): string => (isHttps(t) ? '' : SOURCE_PROBLEMS.address)
const queryProblem = (t: string): string => ([...t].length <= HN_QUERY_MAX ? '' : SOURCE_PROBLEMS.query)
const channelProblem = (t: string): string => (CHANNEL.test(channelName(t)) ? '' : SOURCE_PROBLEMS.channel)

/** The problem with a source's target (or server), in words, or '' when it is fine or still empty. */
export function sourceProblem(source: NewsroomSource): string {
  if (source.kind === 'mastodon') return mastodonProblem(source)
  const target = source.target.trim()
  if (target === '') return ''
  if (source.kind === 'hackernews') return queryProblem(target)
  if (source.kind === 'newssearch') return queryProblem(target) || regionProblem(source)
  if (source.kind === 'telegram') return channelProblem(target)
  return addressProblem(target)
}
