<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'

import { TakeConnectRequest } from '@bindings/go/main/App'
import { EventsOn } from '@bindings/runtime/runtime'
import logo from '@/assets/images/app-logo.png'
import HostKeyDialog from '@/components/HostKeyDialog.vue'
import OtpDialog from '@/components/OtpDialog.vue'
import { coerceSession, errorMessage } from '@/lib/state'
import { useConnectionsStore } from '@/stores/connections'
import { useSessionsStore } from '@/stores/sessions'

const route = useRoute()
const router = useRouter()
const store = useConnectionsStore()
const sessions = useSessionsStore()

// Poll clients that can report status, so tunnels raised or dropped outside
// Tobiro still show up.
let statusPoll: number | undefined

// Sharing connections as a file: the export holds no passwords, so the whole
// exchange is a plain YAML file a colleague can read before importing.
const notice = ref<{ text: string; tone: 'ok' | 'bad' } | null>(null)
const sharing = ref(false)
let noticeTimer: number | undefined

function announce(text: string, tone: 'ok' | 'bad' = 'ok') {
  notice.value = { text, tone }
  if (noticeTimer !== undefined) window.clearTimeout(noticeTimer)
  noticeTimer = window.setTimeout(() => (notice.value = null), 9_000)
}

function fileName(path: string) {
  return path.split(/[/\\]/).pop() || path
}

async function exportConnections() {
  sharing.value = true
  try {
    const result = await store.exportToFile()
    if (result.cancelled) return
    announce(
      `Saved ${result.connections} ${result.connections === 1 ? 'connection' : 'connections'} to ${fileName(result.file)}. Passwords are not in the file.`,
    )
  } catch (err) {
    announce(errorMessage(err, 'Could not export the connections'), 'bad')
  } finally {
    sharing.value = false
  }
}

async function importConnections() {
  sharing.value = true
  try {
    const result = await store.importFromFile()
    if (result.cancelled) return

    const parts: string[] = []
    if (result.connections > 0) {
      parts.push(`Added ${result.connections} ${result.connections === 1 ? 'connection' : 'connections'}`)
      if (result.accounts > 0) {
        parts.push(`${result.accounts} ${result.accounts === 1 ? 'account needs' : 'accounts need'} a password`)
      }
    }
    if (result.skipped?.length) {
      parts.push(`skipped ${result.skipped.join(', ')} — already here`)
    }
    announce(parts.length ? `${parts.join(' · ')}.` : 'That file added nothing new.')
  } catch (err) {
    announce(errorMessage(err, 'Could not import that file'), 'bad')
  } finally {
    sharing.value = false
  }
}

/**
 * Opens the connection another Biebie application asked for.
 *
 * Go holds the request rather than passing it in the event, because a deep link
 * can launch this application and arrive before the window exists. Asking for it
 * here covers both cases with one path: the request is collected on mount, and
 * again whenever Go says another has arrived.
 *
 * The dialog is only opened, never submitted. Choosing an account, and typing
 * whatever it asks for, stays with the person at the keyboard.
 */
async function collectConnectRequest() {
  const request = await TakeConnectRequest()
  const id = request?.profileId
  if (!id) return

  await store.refresh()
  await router.push({ name: 'connection', params: { id }, query: { connect: '1' } })
}

onMounted(async () => {
  sessions.init()
  await store.loadBootstrap()
  await store.refresh()
  await sessions.refresh()

  EventsOn('profile:changed', () => void store.refresh())
  EventsOn('session:changed', (raw: unknown) => {
    const session = coerceSession(raw)
    if (session) store.applySession(session)
  })
  EventsOn('peer:connect_requested', () => void collectConnectRequest())

  // A deep link that launched this application was waiting before any of the
  // listeners above existed.
  await collectConnectRequest()

  statusPoll = window.setInterval(() => {
    void sessions.refresh()
  }, 4_000)
})

onUnmounted(() => {
  if (statusPoll !== undefined) window.clearInterval(statusPoll)
  if (noticeTimer !== undefined) window.clearTimeout(noticeTimer)
  sessions.dispose()
})
</script>

<template>
  <div class="flex h-full flex-col bg-surface-0">
    <!-- Draggable strip for the macOS hidden-inset title bar. -->
    <header class="app-header drag-region flex shrink-0 items-center gap-8 border-b border-line">
      <div class="flex items-center gap-3">
        <img :src="logo" alt="Tobiro" class="h-12 w-12 shrink-0 object-contain" />
        <div class="leading-tight">
          <p class="text-[15px] font-semibold tracking-tight text-ink">Tobiro</p>
          <p class="text-[11px] tracking-wide text-ink-faint">
            VPNs, SSH tunnels, one workspace. <span class="ml-1 text-ink-faint">v{{ store.bootstrap?.version ?? '—' }}</span>
          </p>
        </div>
      </div>

      <nav class="app-navigation no-drag">
        <RouterLink
          :to="{ name: 'dashboard' }"
          class="app-nav-link"
          :class="
            route.name === 'dashboard' || route.name === 'connection'
              ? 'app-nav-link--active'
              : ''
          "
        >
          <svg aria-hidden="true" class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><rect x="3" y="3" width="7" height="7" rx="2"/><rect x="14" y="3" width="7" height="7" rx="2"/><rect x="3" y="14" width="7" height="7" rx="2"/><rect x="14" y="14" width="7" height="7" rx="2"/></svg>
          Connections
        </RouterLink>
        <RouterLink
          :to="{ name: 'settings' }"
          class="app-nav-link"
          :class="
            route.name === 'settings'
              ? 'app-nav-link--active'
              : ''
          "
        >
          <svg aria-hidden="true" class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"><path d="M4 7h16M4 17h16"/><circle cx="9" cy="7" r="3" fill="var(--color-surface-3)"/><circle cx="15" cy="17" r="3" fill="var(--color-surface-3)"/></svg>
          Settings
        </RouterLink>
      </nav>

      <div class="app-header-actions ml-auto flex items-center gap-2">
        <span
          v-if="sessions.anyConnected"
          class="inline-flex items-center gap-2 rounded-full bg-ok/10 px-3 py-1 text-xs font-medium text-ok"
        >
          <span class="h-1.5 w-1.5 rounded-full bg-ok" />
          Tunnel active
        </span>

        <button
          type="button"
          :disabled="sharing"
          aria-label="Import connections"
          title="Import connections from a shared file"
          class="no-drag grid h-9 w-9 place-items-center rounded-lg border border-line bg-menu-button text-ink-muted transition hover:text-ink disabled:opacity-40"
          @click="importConnections"
        >
          <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 3v12m0 0-4-4m4 4 4-4M4 17v2a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-2" />
          </svg>
        </button>
        <button
          type="button"
          :disabled="sharing"
          aria-label="Export connections"
          title="Export connections to share (no passwords)"
          class="no-drag grid h-9 w-9 place-items-center rounded-lg border border-line bg-menu-button text-ink-muted transition hover:text-ink disabled:opacity-40"
          @click="exportConnections"
        >
          <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 15V3m0 0-4 4m4-4 4 4M4 17v2a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-2" />
          </svg>
        </button>
      </div>
    </header>

    <main class="app-main flex-1 overflow-y-auto">
      <RouterView />
    </main>

    <Transition
      enter-active-class="transition duration-200"
      enter-from-class="translate-y-2 opacity-0"
      leave-active-class="transition duration-200"
      leave-to-class="translate-y-2 opacity-0"
    >
      <button
        v-if="notice"
        type="button"
        class="no-drag fixed bottom-6 left-1/2 max-w-[34rem] -translate-x-1/2 rounded-xl border px-4 py-2.5 text-left text-sm shadow-lg"
        :class="
          notice.tone === 'bad'
            ? 'border-bad/30 bg-bad/15 text-bad'
            : 'border-line-strong bg-surface-3 text-ink'
        "
        @click="notice = null"
      >
        {{ notice.text }}
      </button>
    </Transition>

    <!-- Authentication can pause here for a one-time code. -->
    <OtpDialog v-if="sessions.otpRequest" />

    <!-- An SSH tunnel pauses here the first time it meets a server. -->
    <HostKeyDialog v-if="sessions.hostKeyRequest" />
  </div>
</template>
