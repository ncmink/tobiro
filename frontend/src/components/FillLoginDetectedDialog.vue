<script setup lang="ts">
import { onUnmounted, watch } from 'vue'
import { RouterLink } from 'vue-router'

import AppModal from '@/components/AppModal.vue'
import { useFillShortcutStore } from '@/stores/fillShortcut'

const store = useFillShortcutStore()
const autoCloseMs = 5000

let autoCloseTimer: number | undefined

function clearAutoClose() {
  if (autoCloseTimer !== undefined) {
    window.clearTimeout(autoCloseTimer)
    autoCloseTimer = undefined
  }
}

function close() {
  clearAutoClose()
  store.dismissLoginDetected()
}

watch(
  () => store.loginDetected,
  (hint) => {
    clearAutoClose()
    if (!hint) return
    autoCloseTimer = window.setTimeout(() => {
      autoCloseTimer = undefined
      store.dismissLoginDetected()
    }, autoCloseMs)
  },
)

onUnmounted(clearAutoClose)
</script>

<template>
  <AppModal
    v-if="store.loginDetected"
    title="Ready to fill your login"
    :subtitle="store.loginDetected.connectionName"
    width="max-w-md"
    @close="close"
  >
    <div class="space-y-4">
      <div
        class="flex flex-col items-center rounded-2xl border border-line-strong bg-surface-0 px-5 py-6 text-center"
      >
        <span class="text-xs font-medium tracking-wide text-ink-faint uppercase">
          Switch to the VPN window, then press
        </span>
        <kbd
          class="mt-3 inline-flex min-w-[4.5rem] items-center justify-center rounded-xl border-2 border-brand/50 bg-brand/12 px-5 py-2.5 font-mono text-2xl font-semibold tracking-wider text-ink shadow-sm"
        >
          {{ store.loginDetected.shortcutDisplay }}
        </kbd>
        <p class="mt-3 max-w-xs text-sm leading-snug text-ink-muted">
          Tobiro fills your saved username and password.
        </p>
      </div>

      <ol class="space-y-2.5 text-sm text-ink-muted">
        <li class="flex gap-3">
          <span
            class="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-surface-2 font-mono text-xs font-semibold text-ink"
          >
            1
          </span>
          <span>Click the <strong class="font-medium text-ink">VPN login window</strong> so it is in front.</span>
        </li>
        <li class="flex gap-3">
          <span
            class="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-surface-2 font-mono text-xs font-semibold text-ink"
          >
            2
          </span>
          <span>
            Press
            <kbd
              class="rounded border border-line-strong bg-surface-0 px-1.5 py-0.5 font-mono text-xs text-ink"
            >{{ store.loginDetected.shortcutDisplay }}</kbd>
            — fields fill automatically.
          </span>
        </li>
        <li class="flex gap-3">
          <span
            class="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-surface-2 font-mono text-xs font-semibold text-ink"
          >
            3
          </span>
          <span>Click <strong class="font-medium text-ink">Connect</strong> or <strong class="font-medium text-ink">Authenticate</strong> in the VPN app.</span>
        </li>
      </ol>

      <p class="text-center text-xs text-ink-faint">
        Wrong shortcut?
        <RouterLink :to="{ name: 'settings' }" class="text-brand hover:underline" @click="close">
          Change in Settings
        </RouterLink>
      </p>
    </div>

    <template #footer>
      <button type="button" class="btn-brand w-full sm:w-auto" @click="close">Got it</button>
    </template>
  </AppModal>
</template>
