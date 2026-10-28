<script setup lang="ts">
import AppModal from '@/components/AppModal.vue'
import { useSessionsStore } from '@/stores/sessions'

const sessions = useSessionsStore()
</script>

<template>
  <AppModal
    v-if="sessions.hostKeyRequest"
    title="Is this the right server?"
    subtitle="This SSH server has not been connected to from this machine before."
    width="max-w-md"
    @close="sessions.answerHostKey(false)"
  >
    <div class="space-y-4">
      <div>
        <p class="text-xs text-ink-faint">Server</p>
        <p class="mt-0.5 font-mono text-sm text-ink">{{ sessions.hostKeyRequest.host }}</p>
      </div>

      <div>
        <p class="text-xs text-ink-faint">
          Key fingerprint ({{ sessions.hostKeyRequest.algorithm }})
        </p>
        <p class="mt-0.5 font-mono text-sm leading-relaxed break-all text-ink">
          {{ sessions.hostKeyRequest.fingerprint }}
        </p>
      </div>

      <!-- The whole dialog exists for this sentence. Accepting a fingerprint
           without checking it is the one action that makes the tunnel worth
           attacking, so the check is stated rather than implied. -->
      <p class="rounded-xl border border-warn/40 bg-warn/10 px-3 py-2.5 text-xs leading-relaxed text-ink">
        Compare this fingerprint with the one the server's owner gave you. If they do not match,
        something else is answering at this address — do not continue.
      </p>

      <p class="text-xs leading-relaxed text-ink-muted">
        Accepting records the key in
        <span class="font-mono">{{ sessions.hostKeyRequest.knownHosts }}</span>
        so you are only asked once.
      </p>
    </div>

    <template #footer>
      <button
        type="button"
        class="rounded-xl px-4 py-2 text-sm font-medium text-ink-muted transition hover:bg-ink/5 hover:text-ink"
        @click="sessions.answerHostKey(false)"
      >
        Cancel
      </button>
      <button
        type="button"
        class="rounded-xl bg-brand px-4 py-2 text-sm font-semibold text-surface-1 transition hover:bg-brand-strong"
        @click="sessions.answerHostKey(true)"
      >
        Yes, connect
      </button>
    </template>
  </AppModal>
</template>
