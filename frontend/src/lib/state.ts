import type { domain, main } from '@bindings/go/models'

/** Presentation metadata for the Go connection state machine. */
export type ConnectionState =
  | 'disconnected'
  | 'connecting'
  | 'authenticating'
  | 'waiting_for_otp'
  | 'connected'
  | 'disconnecting'
  | 'failed'

interface StateStyle {
  label: string
  dot: string
  text: string
  busy: boolean
}

const STATES: Record<ConnectionState, StateStyle> = {
  disconnected: { label: 'Disconnected', dot: 'bg-ink-faint', text: 'text-ink-muted', busy: false },
  connecting: { label: 'Connecting', dot: 'bg-info animate-pulse', text: 'text-info', busy: true },
  authenticating: { label: 'Authenticating', dot: 'bg-info animate-pulse', text: 'text-info', busy: true },
  waiting_for_otp: { label: 'Waiting for code', dot: 'bg-warn animate-pulse', text: 'text-warn', busy: true },
  connected: { label: 'Connected', dot: 'bg-ok', text: 'text-ok', busy: false },
  disconnecting: { label: 'Disconnecting', dot: 'bg-warn animate-pulse', text: 'text-warn', busy: true },
  failed: { label: 'Failed', dot: 'bg-bad', text: 'text-bad', busy: false },
}

const FALLBACK: StateStyle = { label: 'Unknown', dot: 'bg-ink-faint', text: 'text-ink-muted', busy: false }

export function stateStyle(state: string | undefined): StateStyle {
  return STATES[state as ConnectionState] ?? FALLBACK
}

/** True while a state occupies the profile, so the UI offers Disconnect. */
export function isActive(state: string | undefined): boolean {
  return (
    state === 'connecting' ||
    state === 'authenticating' ||
    state === 'waiting_for_otp' ||
    state === 'connected' ||
    state === 'disconnecting'
  )
}

/**
 * Whether Connect can be offered for a connection as it stands.
 *
 * An adapter that cannot drive its client is never offered one. Past that the
 * only question is the account: most connections need one saved here, while an
 * SSH tunnel is happy with a key the agent already holds.
 */
export function canConnect(connection: main.ConnectionView): boolean {
  const capabilities = connection.capabilities
  if (!capabilities.canConnect) return false
  if (!capabilities.supportsPassword || capabilities.accountOptional) return true
  return connection.credentials.length > 0
}

/**
 * Which account a Connect click should use, or null when the user has to pick.
 *
 * An empty string is a real answer, not a missing one: it tells Go to connect
 * without an account, which is how a tunnel reaches the ssh-agent.
 */
export function accountToConnectWith(connection: main.ConnectionView): string | null {
  if (!connection.capabilities.supportsPassword) return connection.credentials[0]?.id ?? ''
  if (connection.credentials.length === 1) return connection.credentials[0].id
  if (connection.credentials.length === 0 && connection.capabilities.accountOptional) return ''
  return null
}

/** Formats a duration in seconds as HH:MM:SS. */
export function formatDuration(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds))
  const hours = Math.floor(s / 3600)
  const minutes = Math.floor((s % 3600) / 60)
  const seconds = s % 60
  return [hours, minutes, seconds].map((n) => String(n).padStart(2, '0')).join(':')
}

/** Wails rejects promises with a plain string or an Error; normalise both. */
export function errorMessage(error: unknown, fallback = 'Something went wrong'): string {
  if (typeof error === 'string' && error.trim() !== '') return error
  if (error instanceof Error && error.message) return error.message
  return fallback
}

type LooseSession = {
  profileId?: string
  ProfileID?: string
  state?: string
  State?: string
  assignedIp?: string
  AssignedIP?: string
  interface?: string
  Interface?: string
  identity?: string
  Identity?: string
  credentialId?: string
  CredentialID?: string
  connectedAt?: string
  durationSeconds?: number
  forwards?: domain.PortForward[]
  Forwards?: domain.PortForward[]
  error?: string
  Error?: string
}

/** Wails events sometimes arrive with Go field names instead of json tags. */
export function coerceSession(raw: unknown): {
  profileId: string
  state: string
  assignedIp: string
  interface: string
  identity: string
  credentialId: string
  connectedAt?: string
  durationSeconds: number
  forwards: domain.PortForward[]
  error: string
} | null {
  let value: unknown = raw
  if (typeof value === 'string') {
    try {
      value = JSON.parse(value)
    } catch {
      return null
    }
  }
  if (Array.isArray(value)) value = value[0]
  if (!value || typeof value !== 'object') return null

  const session = value as LooseSession
  const profileId = session.profileId || session.ProfileID || ''
  if (!profileId) return null

  return {
    profileId,
    state: session.state || session.State || 'disconnected',
    assignedIp: session.assignedIp || session.AssignedIP || '',
    interface: session.interface || session.Interface || '',
    identity: session.identity || session.Identity || '',
    credentialId: session.credentialId || session.CredentialID || '',
    connectedAt: session.connectedAt,
    durationSeconds: session.durationSeconds ?? 0,
    forwards: session.forwards ?? session.Forwards ?? [],
    error: session.error || session.Error || '',
  }
}
