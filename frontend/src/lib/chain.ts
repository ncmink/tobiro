/**
 * Presentation tree for connections that sit behind other connections.
 *
 * The domain stays flat: each connection is its own profile with an optional
 * `requires` identifier. This module only groups a list the UI already has,
 * so a VPN and the tunnels that need it can share a card without becoming
 * one object.
 */

export interface ChainNode<T> {
  item: T
  children: ChainNode<T>[]
  /** The parent id is set, but that connection is not in this list. */
  missingParent: boolean
  /** This connection is in a requires-cycle; it is shown as a root. */
  cyclic: boolean
}

/** Profile identifier, for grouping a ConnectionView-shaped object. */
export function connectionId(item: { profile: { id: string } }): string {
  return item.profile.id
}

/** Explicit parent identifier. Never inferred from names. */
export function connectionParentId(item: { profile: { requires?: string } }): string {
  return (item.profile.requires || '').trim()
}

/**
 * Groups items into a forest using an explicit parent identifier.
 *
 * Roots keep the input order. Siblings keep the input order. Cycles and
 * missing parents become roots rather than disappearing or looping.
 */
export function forest<T>(
  items: readonly T[],
  idOf: (item: T) => string,
  parentOf: (item: T) => string,
): ChainNode<T>[] {
  const list = uniqueById(items, idOf)
  const byId = new Map<string, T>()
  for (const item of list) {
    byId.set(idOf(item), item)
  }

  const cyclicIds = detectCycles(list, byId, idOf, parentOf)
  const childrenOf = new Map<string, T[]>()
  const nested = new Set<string>()

  for (const item of list) {
    const id = idOf(item)
    const parentId = parentOf(item)
    if (!parentId || !byId.has(parentId) || cyclicIds.has(id)) {
      continue
    }
    const siblings = childrenOf.get(parentId) ?? []
    siblings.push(item)
    childrenOf.set(parentId, siblings)
    nested.add(id)
  }

  const toNode = (item: T): ChainNode<T> => {
    const id = idOf(item)
    const parentId = parentOf(item)
    const kids = childrenOf.get(id) ?? []
    return {
      item,
      children: kids.map(toNode),
      missingParent: parentId !== '' && !byId.has(parentId),
      cyclic: cyclicIds.has(id),
    }
  }

  return list.filter((item) => !nested.has(idOf(item))).map(toNode)
}

/** Finds a node anywhere in the forest, or undefined. */
export function findNode<T>(
  roots: readonly ChainNode<T>[],
  id: string,
  idOf: (item: T) => string,
): ChainNode<T> | undefined {
  for (const node of roots) {
    if (idOf(node.item) === id) return node
    const nested = findNode(node.children, id, idOf)
    if (nested) return nested
  }
  return undefined
}

/** Every item under a node, not including the node itself. */
export function descendants<T>(node: ChainNode<T>): T[] {
  const out: T[] = []
  const walk = (current: ChainNode<T>) => {
    for (const child of current.children) {
      out.push(child.item)
      walk(child)
    }
  }
  walk(node)
  return out
}

function uniqueById<T>(items: readonly T[], idOf: (item: T) => string): T[] {
  const seen = new Set<string>()
  const out: T[] = []
  for (const item of items) {
    const id = idOf(item)
    if (!id || seen.has(id)) continue
    seen.add(id)
    out.push(item)
  }
  return out
}

function detectCycles<T>(
  list: readonly T[],
  byId: Map<string, T>,
  idOf: (item: T) => string,
  parentOf: (item: T) => string,
): Set<string> {
  const cyclic = new Set<string>()
  const limit = list.length + 1

  for (const item of list) {
    const seen: string[] = []
    const walking = new Set<string>()
    let step: string | undefined = idOf(item)
    for (let hops = 0; step && hops < limit; hops++) {
      if (walking.has(step)) {
        const start = seen.indexOf(step)
        for (const id of seen.slice(start < 0 ? 0 : start)) {
          cyclic.add(id)
        }
        cyclic.add(step)
        break
      }
      walking.add(step)
      seen.push(step)
      const current = byId.get(step)
      if (!current) break
      const next = parentOf(current)
      if (!next) break
      step = next
    }
  }
  return cyclic
}
