import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import type { main } from '@bindings/go/models'

import { connectionMap, isDashboardVisible } from './visibility.ts'

function conn(
  id: string,
  opts: { hidden?: boolean; requires?: string } = {},
): main.ConnectionView {
  return {
    hidden: opts.hidden ?? false,
    profile: {
      id,
      requires: opts.requires ?? '',
      name: id,
      kind: id.includes('tunnel') ? 'ssh' : 'vpn',
      group: '',
      host: '',
      provider: id.includes('tunnel') ? 'ssh' : 'mock',
    },
  } as main.ConnectionView
}

describe('dashboard visibility with hidden groups', () => {
  it('shows everything when hidden groups are revealed', () => {
    const vpn = conn('vpn', { hidden: true })
    const tunnel = conn('tunnel', { requires: 'vpn' })
    const byId = connectionMap([vpn, tunnel])
    assert.equal(isDashboardVisible(vpn, byId, true), true)
    assert.equal(isDashboardVisible(tunnel, byId, true), true)
  })

  it('hides a connection in a hidden group', () => {
    const vpn = conn('vpn', { hidden: true })
    const byId = connectionMap([vpn])
    assert.equal(isDashboardVisible(vpn, byId, false), false)
  })

  it('hides an SSH tunnel when its VPN parent is in a hidden group', () => {
    const vpn = conn('vpn', { hidden: true })
    const tunnel = conn('tunnel', { requires: 'vpn' })
    const byId = connectionMap([vpn, tunnel])
    assert.equal(isDashboardVisible(tunnel, byId, false), false)
  })

  it('shows an SSH tunnel when its VPN parent is visible', () => {
    const vpn = conn('vpn', { hidden: false })
    const tunnel = conn('tunnel', { requires: 'vpn' })
    const byId = connectionMap([vpn, tunnel])
    assert.equal(isDashboardVisible(tunnel, byId, false), true)
  })

  it('hides a deep chain when any ancestor is in a hidden group', () => {
    const vpn = conn('vpn', { hidden: true })
    const jump = conn('jump', { requires: 'vpn' })
    const inner = conn('inner', { requires: 'jump' })
    const byId = connectionMap([vpn, jump, inner])
    assert.equal(isDashboardVisible(jump, byId, false), false)
    assert.equal(isDashboardVisible(inner, byId, false), false)
  })

  it('keeps an orphan visible when the parent id is not in the catalogue', () => {
    const tunnel = conn('tunnel', { requires: 'gone' })
    const byId = connectionMap([tunnel])
    assert.equal(isDashboardVisible(tunnel, byId, false), true)
  })
})
