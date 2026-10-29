<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'

import { ClearLogs, GetLogs } from '@bindings/go/main/App'
import type { logging } from '@bindings/go/models'
import { EventsOn } from '@bindings/runtime/runtime'
import ProviderIcon from '@/components/ProviderIcon.vue'
import { useTheme, type ThemePreference } from '@/lib/theme'
import { useConnectionsStore } from '@/stores/connections'

const store = useConnectionsStore()
const { preference, resolved, setTheme } = useTheme()
const themes: { value: ThemePreference; label: string; description: string }[] = [
  { value: 'light', label: 'Light', description: 'A bright, clear workspace' },
  { value: 'dark', label: 'Dark', description: 'A softer view after hours' },
  { value: 'system', label: 'System', description: 'Match your device appearance' },
]

const logs = ref<logging.Entry[]>([])
const detecting = ref(false)

const bootstrap = computed(() => store.bootstrap)

const LEVEL_CLASS: Record<string, string> = {
  info: 'text-ink-muted',
  warn: 'text-warn',
  error: 'text-bad',
}

onMounted(async () => {
  await store.loadProviderStatus()
  logs.value = await GetLogs()
  EventsOn('log:appended', (entry: logging.Entry) => {
    logs.value = [...logs.value, entry].slice(-500)
  })
})

async function redetect() {
  detecting.value = true
  try {
    await store.loadProviderStatus(true)
  } finally {
    detecting.value = false
  }
}

async function clearLogs() {
  await ClearLogs()
  logs.value = []
}

function timeOf(value: string) {
  return new Date(value).toLocaleTimeString()
}

function capabilityList(provider: (typeof store.providerStatus)[number]) {
  const caps = provider.capabilities
  return [
    { label: 'Installed', on: caps.installed },
    { label: 'Command line', on: caps.cliAvailable },
    { label: 'Connect', on: caps.canConnect },
    { label: 'Disconnect', on: caps.canDisconnect },
    { label: 'Read status', on: caps.canReadStatus },
    { label: 'One-time codes', on: caps.supportsOtp },
  ]
}
</script>

<template>
  <div class="space-y-6">
    <RouterLink :to="{ name: 'dashboard' }" class="back-link">
      &larr; Connections
    </RouterLink>

    <header>
      <h1 class="text-2xl font-bold text-ink">Settings</h1>
      <p class="mt-1 text-sm text-ink-muted">
        Tobiro {{ bootstrap?.version }} &middot; part of the biebie.net product family
      </p>
    </header>

    <section class="rounded-2xl border border-line bg-surface-1 p-6" aria-labelledby="appearance-heading">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 id="appearance-heading" class="text-base font-semibold text-ink">Appearance</h2>
          <p class="mt-1 text-sm text-ink-muted">Choose how Tobiro looks on this device.</p>
        </div>
        <span class="rounded-full bg-brand/10 px-3 py-1 text-xs font-medium text-brand">{{ resolved === 'dark' ? 'Dark' : 'Light' }} theme active</span>
      </div>
      <div class="mt-5 grid gap-3 sm:grid-cols-3" role="group" aria-label="App theme">
        <button v-for="theme in themes" :key="theme.value" type="button"
          class="theme-option no-drag" :class="{ 'theme-option--selected': preference === theme.value }"
          :aria-pressed="preference === theme.value" @click="setTheme(theme.value)">
          <span class="theme-preview" :class="`theme-preview--${theme.value}`" aria-hidden="true">
            <span class="theme-preview-bar"><i /><i /><i /></span>
            <span class="theme-preview-content"><span /><span /><span /></span>
          </span>
          <span class="mt-3 flex items-center justify-between gap-2 text-sm font-semibold">
            {{ theme.label }}
            <span class="theme-radio" :class="{ 'theme-radio--selected': preference === theme.value }" aria-hidden="true">
              <svg v-if="preference === theme.value" class="h-3 w-3" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2"><path d="m3 8 3 3 7-7" /></svg>
            </span>
          </span>
          <span class="mt-1 block text-xs text-ink-muted">{{ theme.description }}</span>
        </button>
      </div>
      <p class="mt-4 text-xs text-ink-faint">Your preference is saved automatically.</p>
    </section>

    <!-- Credential storage -->
    <section class="rounded-2xl border border-line bg-surface-1 p-6">
      <h2 class="text-base font-semibold text-ink">Password storage</h2>
      <div class="mt-4 flex flex-wrap items-center gap-3">
        <span
          class="rounded-lg px-2.5 py-1 text-xs font-semibold"
          :class="bootstrap?.secretAvailable ? 'bg-ok/15 text-ok' : 'bg-warn/15 text-warn'"
        >
          {{ bootstrap?.secretBackend }}
        </span>
        <p class="text-sm text-ink-muted">{{ bootstrap?.secretStatusText }}</p>
      </div>
      <p class="mt-4 text-xs leading-relaxed text-ink-faint">
        Passwords are held only by your operating system's credential store. Application data keeps
        a reference to each secret, never the secret itself, and passwords are never sent to the
        user interface.
      </p>
      <dl class="mt-3 space-y-2 text-xs">
        <div class="flex flex-wrap gap-x-2 gap-y-1">
          <dt class="font-medium text-ink-muted">Keychain service</dt>
          <dd class="font-mono text-ink-faint">Tobiro</dd>
        </div>
        <div v-if="bootstrap?.dataPath" class="flex flex-wrap gap-x-2 gap-y-1">
          <dt class="shrink-0 font-medium text-ink-muted">Application data</dt>
          <dd class="min-w-0 truncate font-mono text-ink-faint">{{ bootstrap.dataPath }}</dd>
        </div>
      </dl>
    </section>

    <!-- Provider detection -->
    <section class="rounded-2xl border border-line bg-surface-1">
      <header class="flex items-center justify-between border-b border-line px-6 py-4">
        <div>
          <h2 class="text-base font-semibold text-ink">VPN clients</h2>
          <p class="mt-0.5 text-xs text-ink-muted">
            What each installed client officially allows Tobiro to do.
          </p>
        </div>
        <button
          type="button"
          class="no-drag rounded-xl border border-line-strong px-3.5 py-1.5 text-sm font-medium text-ink transition hover:border-brand/60 hover:text-brand disabled:opacity-50"
          :disabled="detecting"
          @click="redetect"
        >
          {{ detecting ? 'Detecting…' : 'Re-detect' }}
        </button>
      </header>

      <ul class="divide-y divide-line">
        <li v-for="provider in store.providerStatus" :key="provider.provider" class="px-6 py-5">
          <div class="flex items-start gap-4">
            <ProviderIcon :provider="provider.provider" size="sm" />
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="font-medium text-ink">{{ provider.displayName }}</h3>
                <span
                  class="rounded-md px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide"
                  :class="
                    provider.capabilities.installed
                      ? 'bg-ok/15 text-ok'
                      : 'bg-ink/5 text-ink-faint'
                  "
                >
                  {{ provider.capabilities.installed ? 'Installed' : 'Not found' }}
                </span>
                <span v-if="provider.client.version" class="font-mono text-xs text-ink-faint">
                  v{{ provider.client.version }}
                </span>
              </div>

              <div class="mt-3 flex flex-wrap gap-1.5">
                <span
                  v-for="cap in capabilityList(provider)"
                  :key="cap.label"
                  class="rounded-md px-2 py-0.5 text-[11px]"
                  :class="cap.on ? 'bg-ok/10 text-ok' : 'bg-ink/5 text-ink-faint line-through'"
                >
                  {{ cap.label }}
                </span>
              </div>

              <p v-if="provider.capabilities.notes" class="mt-3 text-xs leading-relaxed text-ink-muted">
                {{ provider.capabilities.notes }}
              </p>
              <p v-if="provider.client.path" class="mt-2 truncate font-mono text-[11px] text-ink-faint">
                {{ provider.client.path }}
              </p>
            </div>
          </div>
        </li>
      </ul>
    </section>

    <!-- Activity log -->
    <section class="rounded-2xl border border-line bg-surface-1">
      <header class="flex items-center justify-between border-b border-line px-6 py-4">
        <div>
          <h2 class="text-base font-semibold text-ink">Activity log</h2>
          <p class="mt-0.5 text-xs text-ink-muted">
            Passwords and one-time codes are redacted before anything is written here.
          </p>
        </div>
        <button
          type="button"
          class="no-drag rounded-xl px-3.5 py-1.5 text-sm font-medium text-ink-muted transition hover:bg-ink/5 hover:text-ink"
          @click="clearLogs"
        >
          Clear
        </button>
      </header>

      <div class="max-h-80 overflow-y-auto px-6 py-4">
        <p v-if="!logs.length" class="py-6 text-center text-sm text-ink-muted">Nothing logged yet.</p>
        <ol v-else class="space-y-1.5 font-mono text-xs">
          <li v-for="(entry, index) in logs" :key="index" class="flex gap-3">
            <span class="shrink-0 text-ink-faint">{{ timeOf(entry.time) }}</span>
            <span :class="LEVEL_CLASS[entry.level] ?? 'text-ink-muted'">{{ entry.message }}</span>
          </li>
        </ol>
      </div>
    </section>
  </div>
</template>

<style scoped>
@reference '../style.css';
.theme-option { @apply min-w-0 rounded-xl border border-line bg-surface-0/40 p-3 text-left text-ink transition hover:border-line-strong; }
.theme-option--selected { @apply border-brand bg-brand/5 ring-1 ring-brand/30 hover:border-brand; }
.theme-preview { display: block; overflow: hidden; height: 92px; border-radius: 8px; border: 1px solid #85859b30; background: #f6f7fb; }
.theme-preview-bar { display: flex; align-items: center; gap: 4px; height: 22px; padding: 0 9px; background: #fff; border-bottom: 1px solid #85859b25; }
.theme-preview-bar i { width: 4px; height: 4px; border-radius: 50%; background: #a4a4b5; }
.theme-preview-content { display: grid; grid-template-columns: 1fr 1fr; gap: 7px; padding: 10px; }
.theme-preview-content span { height: 25px; border-radius: 4px; background: #fff; border: 1px solid #85859b25; }
.theme-preview-content span:first-child { grid-column: span 2; width: 45%; height: 5px; background: #9b78c9; border: 0; }
.theme-preview--dark { background: #14151a; }
.theme-preview--dark .theme-preview-bar, .theme-preview--dark .theme-preview-content span:not(:first-child) { background: #252630; }
.theme-preview--system { background: linear-gradient(90deg, #f6f7fb 50%, #14151a 50%); }
.theme-preview--system .theme-preview-bar { background: linear-gradient(90deg, #fff 50%, #252630 50%); }
.theme-preview--system .theme-preview-content span:last-child { background: #252630; }
.theme-radio { @apply grid h-4 w-4 place-items-center rounded-full border border-line-strong; }
.theme-radio--selected { @apply border-brand bg-brand text-surface-1; }
</style>
