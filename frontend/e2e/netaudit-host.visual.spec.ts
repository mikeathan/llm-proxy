import { expect, test, type Page } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// Findings about the computer the program runs on (open ports it did not have, startup files that changed) sit in the same
// report as the devices'. They are labelled "This computer", offer no rescan, expected ports or deep check (there is no
// address to scan), and nothing may scroll sideways at either width.
const finding = (over: Record<string, unknown>) => ({
  key: 'k', device: '192.0.2.10', port: 22, proto: 'tcp', service: 'ssh', check: 'open_port', severity: 'info', new: false, changed: false, accepted: false, ...over,
})
const guide = {
  steps: ['List what is listening, with the program behind each port, and find the port named in the finding.', 'If you started it, accept the finding with a note.'],
  commands: [{ platform: 'Linux', command: 'sudo ss -ltnup' }],
  risk: 'Stopping a program stops what it serves.',
  undo: 'Start the program again.',
}
const report = {
  scanned: true, scanned_at: '2026-11-01T02:10:00Z', engine: 'nmap', devices_up: 3, unscanned: [], resolved: [],
  findings: [
    finding({ key: 'p22', device: '192.0.2.10' }),
    finding({ key: 'p80', device: '192.0.2.20', port: 80, service: 'http' }),
    finding({
      key: 'this-computer|tcp|8080|new_listener', device: 'this-computer', port: 8080, check: 'new_listener', check_title: 'New open port on this computer', severity: 'medium', new: true,
      evidence: 'Something on this computer is waiting for connections on this port, and it was not when the audit first looked. The audit does not say which program it is',
      fix: 'Find out what is listening.', guide,
    }),
    finding({
      key: 'this-computer||0|host_changed_sudoers', device: 'this-computer', port: 0, proto: '', service: '', check: 'host_changed_sudoers', check_title: 'Administrator rights changed on this computer', severity: 'high', new: true,
      evidence: 'A place that decides what runs on this computer after a reboot changed since the audit first recorded it (edited, added to or removed). The audit does not show what changed',
      fix: 'Check sudoers.', fix_state: 'still_present', fix_marked_at: '2026-10-31T09:00:00Z', guide,
    }),
    finding({
      key: 'this-computer||0|host_updates_pending', device: 'this-computer', port: 0, proto: '', service: '', check: 'host_updates_pending', check_title: 'Updates are waiting on this computer', severity: 'low', new: true,
      evidence: '12 updates are waiting to be installed on this computer. 29 more security updates are offered only with Ubuntu Pro',
      fix: 'Install the waiting updates.', guide,
    }),
    finding({
      key: 'this-computer||0|host_reboot_pending', device: 'this-computer', port: 0, proto: '', service: '', check: 'host_reboot_pending', check_title: 'This computer is waiting for a restart', severity: 'medium', new: true,
      evidence: 'An update that needs a restart was installed on this computer, and the restart has been waiting for 15 days. Until then the old version of that software keeps running',
      fix: 'Restart this computer.', guide,
    }),
    finding({
      key: 'this-computer||0|host_file_modified', device: 'this-computer', port: 0, proto: '', service: '', check: 'host_file_modified', check_title: 'A program file on this computer no longer matches its package', severity: 'high', new: true,
      evidence: '12 program files that packages installed on this computer no longer match the checksum the package shipped: /usr/sbin/sshd, /usr/bin/sudo, /usr/lib/x86_64-linux-gnu/some-library-with-a-rather-long-name-1.2.3.so, and 9 more. A change by hand, by an attack or by a failing disk looks the same; the audit cannot tell which',
      fix: 'Find which package owns each file and reinstall it.', guide,
    }),
  ],
}
const automation = {
  id: 'ws/network-audit', workspace: 'ws', name: 'network-audit', task_file: 'network-audit.md', strategy: 'persistent', trigger: 'cron',
  trigger_value: '0 2 * * *', profile: 'security-auditor', network_grant: 'lan', scan: { depth: 'thorough', checks: true }, history: [{ id: 'r1', timestamp: '2026-11-01T02:00:00Z', error: '', output: '', duration_ms: 1000, model: '' }],
}

async function open(page: Page, view: 'simple' | 'advanced') {
  await setNetauditView(page, view)
  await mockAdminApi(page, { answers: { '/netaudit/findings': report, '/netaudit/expectations': [], '/automations': [automation], '/workspaces': [{ id: 'ws' }] } })
  await page.goto('automations/ws%2Fnetwork-audit')
  await expect(page.getByRole('heading', { name: 'Latest network scan' })).toBeVisible()
}

for (const view of ['simple', 'advanced'] as const) {
  test(`findings about this computer (${view}) fit the screen and offer no scan of it`, async ({ page }, testInfo) => {
    await open(page, view)
    await expect(page.getByText('This computer').first()).toBeVisible()
    await expect(page.getByText('this-computer')).toHaveCount(0)
    if (view === 'advanced') {
      const host = page.locator('[data-test="device"]').filter({ hasText: 'This computer' })
      await expect(host.getByRole('button', { name: 'Rescan this host' })).toHaveCount(0)
      await expect(host.getByRole('button', { name: 'More' })).toHaveCount(0)
    }
    await page.getByRole('button', { name: 'How to fix it' }).first().click()
    await expect(page.getByRole('button', { name: 'Scan this device again' })).toHaveCount(0)
    await expectNoSidewaysScroll(page)
    await lookAt(page, testInfo, `host-${view}.png`, true)
  })
}
