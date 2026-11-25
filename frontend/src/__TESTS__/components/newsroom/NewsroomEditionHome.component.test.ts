import { describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, mount } from '@vue/test-utils'
import NewsroomEdition from '../../../components/newsroom/NewsroomEdition.vue'
import NewsroomEditionHome from '../../../components/newsroom/NewsroomEditionHome.vue'
import { toNetwork, toRadio } from '../../../router/routes'
import type { BriefHomeLine, BriefIssue } from '../../../types/newsroom'

vi.mock('../../../services/modelStatsService', () => ({ ModelStatsService: { get: vi.fn().mockResolvedValue({ days: [], today: [] }) } }))

const issue = (home?: BriefHomeLine[]): BriefIssue => ({ id: 1, edition_id: 2, name: 'Morning', made_at: '2026-11-22T06:40:00Z', sections: [], model_calls: 0, home })
const mountIt = (home?: BriefHomeLine[]) => mount(NewsroomEdition, { props: { issue: issue(home) }, global: { stubs: { RouterLink: RouterLinkStub } } })
const mountHome = (lines: BriefHomeLine[]) => mount(NewsroomEditionHome, { props: { lines }, global: { stubs: { RouterLink: RouterLinkStub } } })

const ONLINE: BriefHomeLine = { id: 'network_online', count: 16, words: 'Devices the network sees right now.', level: 'info' }
const PMF: BriefHomeLine = { id: 'radio_pmf_off', count: 1, words: 'Confirmed WPA2 access points of yours that do not protect their management frames.', level: 'check' }
const OPEN: BriefHomeLine = { id: 'radio_open_or_weak_ap', count: 1, words: 'Confirmed access points of yours that are open or use an outdated protection.', level: 'critical' }
const UNMARKED: BriefHomeLine = { id: 'network_not_marked_mine', count: 2, words: 'Devices on your network that you have not marked as yours.', level: 'check' }

describe('the Home block of an edition', () => {
  it('shows each count with the core\'s sentence and says it holds counts only', () => {
    const w = mountIt([ONLINE, { id: 'radio_off', words: 'Radio is off, so nothing is said about the air.', level: 'info' }])
    const block = w.get('[data-test="home-block"]')
    expect(block.text()).toContain('Counts only')
    // A quiet block is folded to one line, and opens on a press.
    expect(block.find('[data-test="home-lines"]').exists()).toBe(false)
    return block.get('[data-test="home-toggle"]').trigger('click').then(() => {
      expect(block.text()).toContain('16')
      expect(block.text()).toContain('Devices the network sees right now.')
      expect(block.text()).toContain('Radio is off')
    })
  })

  it('shows its text as text and is absent when the edition did not ask', () => {
    const w = mountIt([{ id: 'x', words: '<img src=x onerror=alert(1)>', level: 'check' }])
    expect(w.find('img').exists()).toBe(false)
    expect(w.text()).toContain('<img src=x onerror=alert(1)>')
    expect(mountIt(undefined).find('[data-test="home-block"]').exists()).toBe(false)
  })
})

describe('Home by level', () => {
  it('says all quiet and folds when nothing needs a look', () => {
    const w = mountHome([ONLINE])
    expect(w.get('[data-test="home-state"]').text()).toBe('All quiet')
    expect(w.get('[data-test="home-block"]').attributes('data-state')).toBe('quiet')
    expect(w.get('[data-test="home-toggle"]').attributes('aria-expanded')).toBe('false')
    expect(w.find('[data-test="home-lines"]').exists()).toBe(false)
  })

  it('opens with the lines to check, each one a link to the page where it is acted on, and the routine facts apart', () => {
    const w = mountHome([PMF, UNMARKED, ONLINE])
    expect(w.get('[data-test="home-state"]').text()).toBe('2 to check')
    expect(w.get('[data-test="home-toggle"]').attributes('aria-expanded')).toBe('true')
    const rows = w.findAll('[data-test="home-attention"]')
    expect(rows.map((r) => r.text())).toEqual([expect.stringContaining('Confirmed WPA2'), expect.stringContaining('not marked as yours')])
    const links = w.findAllComponents(RouterLinkStub).filter((l) => l.attributes('data-test') === 'home-attention')
    expect(links.map((l) => l.props('to'))).toEqual([toRadio(), toNetwork()])
    expect(w.get('[data-test="home-routine"]').text()).toContain('Devices the network sees right now.')
    expect(w.get('[data-test="home-routine"]').find('a').exists()).toBe(false)
  })

  it('puts a critical line first, in the error colour, and says so in the header', () => {
    const w = mountHome([PMF, OPEN, ONLINE])
    expect(w.get('[data-test="home-state"]').text()).toBe('1 critical · 1 to check')
    expect(w.get('[data-test="home-block"]').attributes('data-state')).toBe('critical')
    const rows = w.findAll('[data-test="home-attention"]')
    expect(rows[0]!.attributes('data-level')).toBe('critical')
    expect(rows[0]!.text()).toContain('open or use an outdated protection')
  })

  it('can be folded and opened by hand, and keeps the links to Network and Radio', async () => {
    const w = mountHome([PMF])
    await w.get('[data-test="home-toggle"]').trigger('click')
    expect(w.find('[data-test="home-lines"]').exists()).toBe(false)
    await w.get('[data-test="home-toggle"]').trigger('click')
    expect(w.find('[data-test="home-lines"]').exists()).toBe(true)
    const to = w.findAllComponents(RouterLinkStub).filter((l) => l.attributes('data-test') !== 'home-attention').map((l) => l.props('to'))
    expect(to).toEqual([toNetwork(), toRadio()])
  })

  it('reads a line with no level (an edition written before levels) as routine', () => {
    const w = mountHome([{ id: 'network_online', count: 3, words: 'Devices the network sees right now.' }])
    expect(w.get('[data-test="home-state"]').text()).toBe('All quiet')
  })
})
