import type { main } from '@/lib/bindings'

/** True when the user can press Fill after the vendor login form is on screen. */
export function canFillNativeLogin(connection: main.ConnectionView): boolean {
  if (!connection.capabilities?.canFillNativeLogin) return false
  return connection.credentials.some((c) => Boolean(c.username) || c.hasPassword)
}
