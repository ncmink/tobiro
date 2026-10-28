import type { main } from '@bindings/go/models'

/**
 * Whether a connection should appear on the dashboard for the current hidden-group
 * preference. Hiding is presentation-only: a filtered-out profile can still connect.
 *
 * A tunnel that requires a VPN in a hidden group stays off the list until
 * "Show hidden" is on, even when the tunnel's own group is visible.
 */
export function isDashboardVisible(
  connection: main.ConnectionView,
  byId: ReadonlyMap<string, main.ConnectionView>,
  showHidden: boolean,
): boolean {
  if (showHidden) return true
  if (connection.hidden) return false

  let parentId = (connection.profile.requires || '').trim()
  const seen = new Set<string>()
  while (parentId) {
    if (seen.has(parentId)) break
    seen.add(parentId)
    const parent = byId.get(parentId)
    if (!parent) break
    if (parent.hidden) return false
    parentId = (parent.profile.requires || '').trim()
  }
  return true
}

export function connectionMap(
  connections: readonly main.ConnectionView[],
): Map<string, main.ConnectionView> {
  const byId = new Map<string, main.ConnectionView>()
  for (const c of connections) {
    const id = c.profile.id
    if (id) byId.set(id, c)
  }
  return byId
}
