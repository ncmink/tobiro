<script setup lang="ts">
import { ref } from 'vue'

import type { domain } from '@bindings/go/models'

const props = defineProps<{
  forwards: domain.PortForward[]
  /**
   * The SSH server the ports are borrowed from, named so each row can say whose
   * address the far side is.
   *
   * Both halves of a forward are usually written as loopback — `127.0.0.1` here,
   * `localhost` there — and on one line they read as the same machine. Naming
   * the server is the only thing that tells them apart.
   */
  server?: string
  /** Compact drops the explanatory line, for the dashboard card. */
  compact?: boolean
}>()

/**
 * Which address was copied last, so the button can confirm it.
 *
 * Copying is the point of this list: the local address is what gets pasted into
 * a browser, a kubectl flag or a database client, and retyping 127.0.0.1:16443
 * from a card is exactly the kind of small friction this feature exists to end.
 */
const copied = ref('')
let timer: number | undefined

async function copy(address: string) {
  try {
    await navigator.clipboard.writeText(address)
    copied.value = address
    if (timer !== undefined) window.clearTimeout(timer)
    timer = window.setTimeout(() => {
      copied.value = ''
    }, 1500)
  } catch {
    // A clipboard the webview refuses is not worth an error dialog; the
    // address is on screen and can be selected.
  }
}

function isLoopback(host: string): boolean {
  const name = host.trim().toLowerCase()
  return name === '' || name === 'localhost' || name === '127.0.0.1' || name === '::1'
}

/**
 * What the far side of one row is.
 *
 * A forward to the server's own loopback is on the server. A forward to any
 * other name is somewhere else in the customer network, reached from the
 * server — a difference worth keeping, because it is the difference between
 * "the API server on the bastion" and "the database the bastion can see".
 */
function forwardLabel(forward: domain.PortForward): string {
  if (forward.state === 'active') {
    const n = forward.active ?? 1
    return n > 1 ? `${n} open` : 'In use'
  }
  if (forward.state === 'failed') return 'Remote refused'
  if (forward.remoteOk === false) return 'Remote refused'
  return 'Listening'
}

function remoteLabel(forward: domain.PortForward): string {
  if (!props.server) return isLoopback(forward.remoteHost) ? 'On the SSH server' : 'Inside the network'
  return isLoopback(forward.remoteHost) ? `On ${props.server}` : `Via ${props.server}`
}
</script>

<template>
  <div v-if="forwards.length" class="space-y-1.5">
    <button
      v-for="forward in forwards"
      :key="forward.localPort"
      type="button"
      class="no-drag flex w-full items-start gap-3 rounded-xl border border-line bg-surface-2 px-3 py-2 text-left transition hover:border-brand/60"
      :title="
        `Copy 127.0.0.1:${forward.localPort}. ${forward.remoteHost}:${forward.remotePort} is resolved on ${server || 'the SSH server'}, not this machine.`
      "
      @click="copy(`127.0.0.1:${forward.localPort}`)"
    >
      <span class="min-w-0 flex-1">
        <span v-if="forward.name" class="mb-1 block truncate text-[11px] text-ink-muted">
          {{ forward.name }}
        </span>

        <span v-if="compact" class="forward-addresses flex min-w-0 items-baseline gap-2">
          <span class="truncate font-mono text-xs text-ink">127.0.0.1:{{ forward.localPort }}</span>
          <span class="shrink-0 text-xs text-ink-faint" aria-hidden="true">&rarr;</span>
          <span class="truncate font-mono text-xs text-ink-muted">
            {{ forward.remoteHost }}:{{ forward.remotePort }}
          </span>
        </span>
        <span v-else class="grid grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-end gap-2">
          <span class="min-w-0">
            <span class="block truncate text-[10px] tracking-wide text-ink-faint">Local</span>
            <span class="block truncate font-mono text-xs text-ink">
              127.0.0.1:{{ forward.localPort }}
            </span>
          </span>
          <span class="pb-0.5 text-xs text-ink-faint" aria-hidden="true">&rarr;</span>
          <span class="min-w-0">
            <span class="block truncate text-[10px] tracking-wide text-ink-faint">
              {{ remoteLabel(forward) }}
            </span>
            <span class="block truncate font-mono text-xs text-ink-muted">
              {{ forward.remoteHost }}:{{ forward.remotePort }}
            </span>
          </span>
        </span>
      </span>

      <span class="flex shrink-0 flex-col items-end gap-0.5 pt-0.5">
        <span
          v-if="forward.state"
          class="text-[11px] font-medium"
          :class="forward.state === 'failed' ? 'text-bad' : 'text-ok'"
          :title="forward.lastError || ''"
        >
          {{ forwardLabel(forward) }}
        </span>
        <span
          class="text-[11px] font-medium"
          :class="copied === `127.0.0.1:${forward.localPort}` ? 'text-ok' : 'text-ink-faint'"
        >
          {{ copied === `127.0.0.1:${forward.localPort}` ? 'Copied' : 'Copy' }}
        </span>
      </span>
    </button>

    <p v-if="!compact" class="text-xs leading-relaxed text-ink-muted">
      Use the local address from this machine. The remote address is resolved on the
      <span v-if="server" class="font-mono">{{ server }}</span>
      <span v-else>SSH</span>
      server — <span class="font-mono">localhost</span> there is that machine, not this one.
    </p>
  </div>
</template>

<style scoped>
@container (max-width: 360px) {
  .forward-addresses { flex-wrap: wrap; gap: 0.25rem; }
  .forward-addresses > span:first-child { flex-basis: 100%; }
}
</style>
