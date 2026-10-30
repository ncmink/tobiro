<script setup lang="ts">
import { useSessionsStore } from '@/stores/sessions'

const sessions = useSessionsStore()
</script>

<template>
  <div
    v-if="sessions.nativeFillPending"
    class="no-drag fixed inset-0 z-[60] flex items-center justify-center bg-black/70 p-6 backdrop-blur-sm"
    role="dialog"
    aria-modal="true"
    aria-labelledby="fill-countdown-title"
    @click.self="sessions.cancelNativeFill()"
  >
    <div class="w-full max-w-md rounded-2xl border border-line-strong bg-surface-1 p-6 shadow-2xl">
      <h2 id="fill-countdown-title" class="text-base font-semibold text-ink">
        Login fields detected
      </h2>
      <p class="mt-2 text-sm leading-relaxed text-ink-muted">
        <template v-if="sessions.nativeFillPending.hasPassword">
          Tobiro will type your saved username and password into the
          <span class="font-medium text-ink">{{ sessions.nativeFillPending.connectionName }}</span>
          login window.
        </template>
        <template v-else>
          Tobiro will type your saved username into the
          <span class="font-medium text-ink">{{ sessions.nativeFillPending.connectionName }}</span>
          login window.
        </template>
        You still press Connect in the vendor app yourself.
      </p>
      <p class="mt-4 text-center text-4xl font-semibold tabular-nums text-brand" aria-live="polite">
        {{ sessions.nativeFillSecondsLeft }}
      </p>
      <p class="mt-1 text-center text-xs text-ink-faint">seconds until auto-fill</p>
      <div class="mt-6 flex justify-end gap-2">
        <button type="button" class="btn-ghost" @click="sessions.cancelNativeFill()">Cancel</button>
        <button type="button" class="btn-brand" @click="sessions.proceedNativeFill()">Fill now</button>
      </div>
    </div>
  </div>
</template>
