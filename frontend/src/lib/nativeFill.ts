import type { main } from '@/lib/bindings'
import { connectionCredentials } from '@/lib/state'

/** True when the user can press Fill after the vendor login form is on screen. */
export function canFillNativeLogin(connection: main.ConnectionView): boolean {
  if (!connection.capabilities?.canFillNativeLogin) return false
  return connectionCredentials(connection).some((c) => Boolean(c.username) || c.hasPassword)
}
