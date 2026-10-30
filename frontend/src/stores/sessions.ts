import { computed, ref } from 'vue'
import { acceptHMRUpdate, defineStore } from 'pinia'

import {
  CancelInteraction,
  Connect,
  Disconnect,
  FillNativeLoginHotkey,
  OpenNativeClient,
  RefreshStatus,
  SetNativeFillProfile,
  SubmitOTP,
} from '@bindings/go/main/App'
import type { domain } from '@bindings/go/models'
import { EventsOn } from '@bindings/runtime/runtime'
import { errorMessage, isActive, coerceSession } from '@/lib/state'

/** Emitted by Go when authentication pauses for a one-time code. */
export interface OtpRequest {
  requestId: string
  profileId: string
  prompt: string
  message: string
}

/**
 * Emitted by Go when an SSH server this machine has never seen answers.
 *
 * Accepting is a security decision rather than a step in a wizard, so it is a
 * distinct request from the OTP one and is never answered on the user's behalf.
 */
export interface HostKeyRequest {
  requestId: string
  profileId: string
  host: string
  algorithm: string
  fingerprint: string
  knownHosts: string
}

interface ConnectionFailed {
  sessionId: string
  profileId: string
  reason: string
}

/**
 * Tracks live VPN sessions.
 *
 * State is driven by `session:changed` events from Go, so the UI follows the
 * backend state machine rather than keeping a second copy of the truth.
 */
export const useSessionsStore = defineStore('sessions', () => {
  const sessions = ref<Record<string, domain.SessionView>>({})
  const busy = ref<Record<string, boolean>>({})
  const errors = ref<Record<string, string>>({})

  const otpRequest = ref<OtpRequest | null>(null)
  const otpSubmitting = ref(false)
  const otpError = ref('')

  const hostKeyRequest = ref<HostKeyRequest | null>(null)

  const now = ref(Date.now())
  let ticker: number | undefined
  let listening = false

  const anyConnected = computed(() =>
    Object.values(sessions.value).some((s) => s.state === 'connected'),
  )

  function sessionFor(profileId: string): domain.SessionView | undefined {
    return sessions.value[profileId]
  }

  function stateFor(profileId: string): string {
    return sessions.value[profileId]?.state ?? 'disconnected'
  }

  function isBusy(profileId: string): boolean {
    return busy.value[profileId] === true
  }

  function errorFor(profileId: string): string {
    return errors.value[profileId] ?? ''
  }

  function clearError(profileId: string) {
    delete errors.value[profileId]
  }

  function durationFor(profileId: string): number {
    const session = sessions.value[profileId]
    if (!session?.connectedAt || session.state !== 'connected') {
      return session?.durationSeconds ?? 0
    }
    const started = new Date(session.connectedAt).getTime()
    return Math.max(0, Math.floor((now.value - started) / 1000))
  }

  function init() {
    if (listening) return
    listening = true

    EventsOn('session:changed', (raw: unknown) => {
      const session = coerceSession(raw)
      if (!session) return
      sessions.value = { ...sessions.value, [session.profileId]: session as domain.SessionView }
      if (session.state !== 'failed') {
        delete errors.value[session.profileId]
      }
      if (!isActive(session.state) && otpRequest.value?.profileId === session.profileId) {
        otpRequest.value = null
      }
      if (!isActive(session.state) && hostKeyRequest.value?.profileId === session.profileId) {
        hostKeyRequest.value = null
      }
    })

    EventsOn('auth:otp_required', (request: OtpRequest) => {
      otpError.value = ''
      otpSubmitting.value = false
      otpRequest.value = request
    })

    EventsOn('auth:host_key_review', (request: HostKeyRequest) => {
      hostKeyRequest.value = request
    })

    EventsOn('connection:failed', (failure: ConnectionFailed) => {
      errors.value = { ...errors.value, [failure.profileId]: failure.reason }
    })

    ticker = window.setInterval(() => {
      now.value = Date.now()
    }, 1000)
  }

  function dispose() {
    if (ticker !== undefined) window.clearInterval(ticker)
    ticker = undefined
  }

  async function connect(profileId: string, credentialId: string) {
    busy.value = { ...busy.value, [profileId]: true }
    delete errors.value[profileId]
    try {
      await Connect(profileId, credentialId)
      return true
    } catch (err) {
      errors.value = { ...errors.value, [profileId]: errorMessage(err, 'Could not connect') }
      return false
    } finally {
      busy.value = { ...busy.value, [profileId]: false }
    }
  }

  async function disconnect(profileId: string) {
    busy.value = { ...busy.value, [profileId]: true }
    try {
      await Disconnect(profileId)
      return true
    } catch (err) {
      errors.value = { ...errors.value, [profileId]: errorMessage(err, 'Could not disconnect') }
      return false
    } finally {
      busy.value = { ...busy.value, [profileId]: false }
    }
  }

  async function openNativeClient(profileId: string, credentialId = '') {
    try {
      await OpenNativeClient(profileId, credentialId)
      await SetNativeFillProfile(profileId)
      delete errors.value[profileId]
      await refresh()
      return true
    } catch (err) {
      errors.value = {
        ...errors.value,
        [profileId]: errorMessage(err, 'Could not open the vendor client'),
      }
      return false
    }
  }

  async function fillNativeLoginHotkey(profileId = '') {
    const key = profileId || '_hotkey'
    busy.value = { ...busy.value, [key]: true }
    if (profileId) delete errors.value[profileId]
    try {
      await FillNativeLoginHotkey(profileId)
      if (profileId) delete errors.value[profileId]
      return true
    } catch (err) {
      const message = errorMessage(err, 'Could not fill the login form')
      if (profileId) {
        errors.value = { ...errors.value, [profileId]: message }
      } else {
        errors.value = { ...errors.value, _hotkey: message }
      }
      return false
    } finally {
      busy.value = { ...busy.value, [key]: false }
    }
  }

  async function rememberNativeFillProfile(profileId: string) {
    if (!profileId) return
    try {
      await SetNativeFillProfile(profileId)
    } catch {
      // non-fatal
    }
  }

  async function submitOtp(code: string) {
    if (!otpRequest.value) return
    if (code.trim() === '') {
      otpError.value = 'Enter the code from your authenticator.'
      return
    }
    otpSubmitting.value = true
    try {
      await SubmitOTP(otpRequest.value.requestId, code.trim())
      otpRequest.value = null
      otpError.value = ''
    } catch (err) {
      otpError.value = errorMessage(err, 'Could not submit the code')
    } finally {
      otpSubmitting.value = false
    }
  }

  async function cancelOtp() {
    if (!otpRequest.value) return
    const { requestId } = otpRequest.value
    otpRequest.value = null
    otpError.value = ''
    try {
      await CancelInteraction(requestId)
    } catch {
      // The attempt already ended; nothing left to cancel.
    }
  }

  async function answerHostKey(accept: boolean) {
    if (!hostKeyRequest.value) return
    const { requestId } = hostKeyRequest.value
    hostKeyRequest.value = null
    try {
      if (accept) {
        await SubmitOTP(requestId, 'accept')
      } else {
        await CancelInteraction(requestId)
      }
    } catch {
      // The attempt already ended; there is nothing left waiting.
    }
  }

  async function refresh() {
    try {
      const latest = await RefreshStatus()
      const next: Record<string, domain.SessionView> = { ...sessions.value }
      for (const raw of latest ?? []) {
        const session = coerceSession(raw)
        if (!session) continue
        next[session.profileId] = session as domain.SessionView
      }
      sessions.value = next
    } catch (err) {
      console.warn('status refresh failed', errorMessage(err))
    }
  }

  return {
    sessions,
    otpRequest,
    otpSubmitting,
    otpError,
    hostKeyRequest,
    anyConnected,
    init,
    dispose,
    sessionFor,
    stateFor,
    isBusy,
    errorFor,
    clearError,
    durationFor,
    connect,
    disconnect,
    openNativeClient,
    fillNativeLoginHotkey,
    rememberNativeFillProfile,
    submitOtp,
    cancelOtp,
    answerHostKey,
    refresh,
  }
})

if (import.meta.hot) {
  import.meta.hot.accept(acceptHMRUpdate(useSessionsStore, import.meta.hot))
}
