import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import { connectionId, connectionParentId, descendants, findNode, forest } from './chain.ts'

interface Conn {
  id: string
  name: string
  parent: string
}

function node(id: string, parent = ''): Conn {
  return { id, name: id, parent }
}

function idsOf(items: Conn[]): string[] {
  return items.map((item) => item.id)
}

function tree(
  items: Conn[],
): ReturnType<typeof forest<Conn>> {
  return forest(items, (c) => c.id, (c) => c.parent)
}

describe('connection forest', () => {
  it('a standalone VPN is one root', () => {
    const roots = tree([node('smoi')])
    assert.equal(roots.length, 1)
    assert.equal(roots[0].item.id, 'smoi')
    assert.equal(roots[0].children.length, 0)
    assert.equal(roots[0].missingParent, false)
  })

  it('a standalone SSH is one root', () => {
    const roots = tree([node('tunnel')])
    assert.equal(roots.length, 1)
    assert.equal(roots[0].item.id, 'tunnel')
    assert.equal(roots[0].children.length, 0)
  })

  it('nests an SSH tunnel under the VPN it requires', () => {
    const roots = tree([node('smoi'), node('tunnel', 'smoi')])
    assert.equal(roots.length, 1)
    assert.equal(roots[0].item.id, 'smoi')
    assert.deepEqual(idsOf(roots[0].children.map((c) => c.item)), ['tunnel'])
    assert.equal(roots[0].children[0].missingParent, false)
  })

  it('nests two SSH tunnels under one VPN', () => {
    const roots = tree([
      node('smoi'),
      node('kube', 'smoi'),
      node('db', 'smoi'),
    ])
    assert.equal(roots.length, 1)
    assert.deepEqual(idsOf(roots[0].children.map((c) => c.item)), ['kube', 'db'])
  })

  it('does not turn forwarded ports into tree nodes', () => {
    // Forwards live on the SSH session, not as sibling connections.
    const roots = tree([node('smoi'), node('tunnel', 'smoi')])
    assert.equal(descendants(roots[0]).length, 1)
    assert.equal(descendants(roots[0])[0].id, 'tunnel')
  })

  it('renders an SSH with a missing parent as a root', () => {
    const roots = tree([node('tunnel', 'gone')])
    assert.equal(roots.length, 1)
    assert.equal(roots[0].item.id, 'tunnel')
    assert.equal(roots[0].missingParent, true)
    assert.equal(roots[0].children.length, 0)
  })

  it('breaks a dependency cycle without recursing', () => {
    const roots = tree([node('a', 'b'), node('b', 'a')])
    assert.equal(roots.length, 2)
    assert.deepEqual(idsOf(roots.map((r) => r.item)).sort(), ['a', 'b'])
    assert.equal(roots[0].cyclic, true)
    assert.equal(roots[1].cyclic, true)
    assert.equal(roots[0].children.length, 0)
    assert.equal(roots[1].children.length, 0)
  })

  it('keeps unrelated connections as separate roots', () => {
    const roots = tree([
      node('smoi'),
      node('tunnel', 'smoi'),
      node('home'),
    ])
    assert.deepEqual(idsOf(roots.map((r) => r.item)), ['smoi', 'home'])
    assert.deepEqual(idsOf(roots[0].children.map((c) => c.item)), ['tunnel'])
    assert.equal(roots[1].children.length, 0)
  })

  it('keeps input order for roots and siblings', () => {
    const roots = tree([
      node('z'),
      node('a'),
      node('child-b', 'a'),
      node('child-a', 'a'),
    ])
    assert.deepEqual(idsOf(roots.map((r) => r.item)), ['z', 'a'])
    assert.deepEqual(idsOf(roots[1].children.map((c) => c.item)), ['child-b', 'child-a'])
  })

  it('nests more than one level', () => {
    const roots = tree([
      node('vpn'),
      node('jump', 'vpn'),
      node('inner', 'jump'),
    ])
    assert.equal(roots.length, 1)
    assert.equal(roots[0].item.id, 'vpn')
    assert.equal(roots[0].children[0].item.id, 'jump')
    assert.equal(roots[0].children[0].children[0].item.id, 'inner')
  })

  it('finds a nested node without flattening the tree', () => {
    const roots = tree([
      node('vpn'),
      node('jump', 'vpn'),
      node('inner', 'jump'),
    ])
    const inner = findNode(roots, 'inner', (c) => c.id)
    assert.ok(inner)
    assert.equal(inner!.item.id, 'inner')
    assert.equal(findNode(roots, 'missing', (c) => c.id), undefined)
  })

  it('does not hide a child when its parent is cyclic with someone else', () => {
    const roots = tree([
      node('a', 'b'),
      node('b', 'a'),
      node('extra', 'a'),
    ])
    const a = roots.find((r) => r.item.id === 'a')
    assert.ok(a)
    assert.deepEqual(idsOf(a!.children.map((c) => c.item)), ['extra'])
  })

  it('groups ConnectionView-shaped items by profile.requires, not by name', () => {
    const vpn = { profile: { id: 'smoi', requires: '', name: 'SMOI' } }
    const tunnel = { profile: { id: 'smoi-tunnel', requires: 'smoi', name: 'SMOI tunnel' } }
    const decoy = { profile: { id: 'other', requires: '', name: 'SMOI reports' } }
    const roots = forest([vpn, tunnel, decoy], connectionId, connectionParentId)
    assert.deepEqual(
      roots.map((r) => r.item.profile.id),
      ['smoi', 'other'],
    )
    assert.deepEqual(
      roots[0].children.map((c) => c.item.profile.id),
      ['smoi-tunnel'],
    )
    assert.equal(roots[1].children.length, 0)
  })
})
