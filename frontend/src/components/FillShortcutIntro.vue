<script setup lang="ts">
import { RouterLink } from 'vue-router'

import AppModal from '@/components/AppModal.vue'
import { DismissFillShortcutIntro, OpenPrivacySettings } from '@bindings/go/main/App'
import { useFillShortcutStore } from '@/stores/fillShortcut'

const store = useFillShortcutStore()

async function close() {
  await DismissFillShortcutIntro()
  await store.refresh()
}

async function openPane(pane: 'accessibility' | 'automation' | 'listen') {
  await OpenPrivacySettings(pane)
}
</script>

<template>
  <AppModal
    title="Fill VPN login from any app"
    subtitle="Use a global shortcut while the vendor login window is open."
    width="max-w-xl"
    @close="close"
  >
    <div class="space-y-5 text-sm text-ink-muted">
      <p>
        Tobiro can type your saved username and password into Forcepoint, FortiClient, and other
        supported VPN clients. You still click Connect or Authenticate yourself.
      </p>

      <ol class="list-decimal space-y-2 pl-5">
        <li>Open the connection in Tobiro once (or use <strong class="text-ink">Open client</strong>).</li>
        <li>Show the vendor login window.</li>
        <li>
          Press
          <kbd class="rounded-md border border-line-strong bg-surface-0 px-1.5 py-0.5 font-mono text-xs text-ink">{{ store.display }}</kbd>.
        </li>
      </ol>

      <div class="rounded-xl border border-line bg-surface-0/50 p-4">
        <h3 class="text-sm font-semibold text-ink">Permissions on this Mac</h3>
        <ul class="mt-3 space-y-3 text-xs leading-relaxed">
          <li>
            <span class="font-medium text-ink">Accessibility</span> — lets Tobiro read VPN login
            fields and type for you. Also required for the global shortcut.
            <button
              type="button"
              class="no-drag ml-1 text-brand underline-offset-2 hover:underline"
              @click="openPane('accessibility')"
            >
              Open settings
            </button>
          </li>
          <li>
            <span class="font-medium text-ink">Automation</span> — allow Tobiro to control
            <span class="font-mono text-ink-faint">System Events</span> when macOS asks.
            <button
              type="button"
              class="no-drag ml-1 text-brand underline-offset-2 hover:underline"
              @click="openPane('automation')"
            >
              Open settings
            </button>
          </li>
          <li>
            <span class="font-medium text-ink">Input Monitoring</span> — only if the shortcut stops
            responding after an macOS update.
            <button
              type="button"
              class="no-drag ml-1 text-brand underline-offset-2 hover:underline"
              @click="openPane('listen')"
            >
              Open settings
            </button>
          </li>
        </ul>
        <p class="mt-3 text-[11px] text-ink-faint">
          Dev builds from <span class="font-mono">wails dev</span> may need these toggles again after
          each rebuild.
        </p>
      </div>

      <p>
        Change the shortcut in
        <RouterLink
          :to="{ name: 'settings', query: { tab: 'shortcuts' } }"
          class="text-brand hover:underline"
          @click="close"
        >
          Settings
        </RouterLink>
        — click the shortcut field and press your keys.
      </p>
    </div>

    <template #footer>
      <button
        type="button"
        class="no-drag rounded-xl bg-brand px-4 py-2 text-sm font-medium text-surface-1"
        @click="close"
      >
        Got it
      </button>
    </template>
  </AppModal>
</template>
