<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { RouterView, useRouter } from 'vue-router'

import { TakeConnectRequest } from '@bindings/go/main/App'
import { EventsOn } from '@bindings/runtime/runtime'
import HostKeyDialog from '@/components/HostKeyDialog.vue'
import OtpDialog from '@/components/OtpDialog.vue'
import { useConnectionSharing } from '@/lib/connectionSharing'
import { coerceSession } from '@/lib/state'
import { useConnectionsStore } from '@/stores/connections'
import { useSessionsStore } from '@/stores/sessions'

const router = useRouter()
const store = useConnectionsStore()
const sessions = useSessionsStore()
const { notice, dismissNotice } = useConnectionSharing()

let statusPoll: number | undefined

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

  await collectConnectRequest()

  statusPoll = window.setInterval(() => {
    void sessions.refresh()
  }, 4_000)
})

onUnmounted(() => {
  if (statusPoll !== undefined) window.clearInterval(statusPoll)
  sessions.dispose()
})
</script>

<template>
  <div class="flex h-full flex-col bg-surface-0">
    <header class="app-titlebar drag-region shrink-0 border-b border-line" aria-hidden="true" />

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
        @click="dismissNotice()"
      >
        {{ notice.text }}
      </button>
    </Transition>

    <OtpDialog v-if="sessions.otpRequest" />
    <HostKeyDialog v-if="sessions.hostKeyRequest" />
  </div>
</template>
