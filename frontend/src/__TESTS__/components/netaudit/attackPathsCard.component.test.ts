import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AttackPathsCard from '../../../components/netaudit/AttackPathsCard.vue'
import type { AttackPath } from '../../../types/netaudit'

const path = (over: Partial<AttackPath> = {}): AttackPath => ({
  id: 'network|this-computer',
  entry: { kind: 'network', text: 'Any device on your network' },
  steps: [{ key: 'k', check: 'host_ssh_password_login', title: 'SSH accepts password login', severity: 'low' }],
  prize: { kind: 'computer', where: 'this-computer' },
  fix: { key: 'k', text: 'Turn password login off.' },
  breaks: 1,
  ...over,
})

describe('AttackPathsCard', () => {
  it('reads as a way in, the weaknesses on it, what it ends at and the one fix', () => {
    const w = mount(AttackPathsCard, { props: { paths: [path()] } })
    expect(w.text()).toContain('Any device on your network')
    expect(w.get('[data-test="path-step"]').text()).toContain('SSH accepts password login')
    expect(w.get('[data-test="path-prize"]').text()).toContain('This computer')
    expect(w.get('[data-test="path-fix"]').text()).toContain('Turn password login off.')
    expect(w.get('[data-test="path-fix"]').text()).not.toContain('breaks')
  })

  it('says how many paths one fix breaks when it breaks more than its own', () => {
    expect(mount(AttackPathsCard, { props: { paths: [path({ breaks: 2 })] } }).get('[data-test="path-fix"]').text()).toContain('This one fix breaks 2 of these paths.')
  })

  it('keeps a path of accepted findings, dimmed and marked with a word', () => {
    const w = mount(AttackPathsCard, { props: { paths: [path({ all_accepted: true, steps: [{ key: 'k', check: 'c', title: 'Telnet is open', severity: 'high', accepted: true }] })] } })
    expect(w.get('[data-test="path-step"]').text()).toContain('accepted')
    expect(w.get('li[data-test^="path-"]').classes()).toContain('opacity-70')
  })

  it('says plainly when nothing leads anywhere, and how to get a device its own paths', () => {
    const w = mount(AttackPathsCard, { props: { paths: [] } })
    expect(w.get('[data-test="attack-paths-none"]').text()).toContain('No way in found')
    expect(w.text()).toContain('Mark one on the Devices tab')
  })

  it('shows a device name as text and never as markup', () => {
    const w = mount(AttackPathsCard, { props: { paths: [path({ prize: { kind: 'important', where: '192.168.1.10', name: '<img src=x onerror=alert(1)>' } })] } })
    expect(w.find('img').exists()).toBe(false)
    expect(w.get('[data-test="path-prize"]').text()).toContain('<img src=x')
  })
})

describe('AttackPathsCard links', () => {
  it('links each step to its finding on the Findings tab', async () => {
    const { createMemoryHistory } = await import('vue-router')
    const { createAppRouter } = await import('../../../router')
    const router = createAppRouter(createMemoryHistory())
    const key = '192.168.50.7|tcp|22|ssh_password_login'
    const w = mount(AttackPathsCard, { props: { paths: [path({ steps: [{ key, check: 'ssh_password_login', title: 'SSH accepts password login', severity: 'low' }] })] }, global: { plugins: [router] } })
    const link = w.get('[data-test="path-step-link"]')
    expect(link.attributes('href')).toBe(`/network/findings?finding=${key}`)
    expect(link.attributes('aria-label')).toBe('Show SSH accepts password login in the findings')
  })
})
