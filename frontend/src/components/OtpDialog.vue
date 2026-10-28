<script setup lang="ts">
import { nextTick, onMounted, ref } from 'vue'

import AppModal from '@/components/AppModal.vue'
import { useSessionsStore } from '@/stores/sessions'

const sessions = useSessionsStore()
const code = ref('')
const field = ref<HTMLInputElement | null>(null)

onMounted(async () => {
  await nextTick()
  field.value?.focus()
})

// The code is passed straight to the waiting connect call and then dropped.
// It is never persisted and never logged.
async function submit() {
  await sessions.submitOtp(code.value)
  code.value = ''
}

async function cancel() {
  code.value = ''
  await sessions.cancelOtp()
}
</script>

<template>
  <AppModal
    v-if="sessions.otpRequest"
    :title="sessions.otpRequest.prompt"
    :subtitle="sessions.otpRequest.message"
    width="max-w-sm"
    @close="cancel"
  >
    <form @submit.prevent="submit">
      <input
        ref="field"
        v-model="code"
        type="text"
        inputmode="numeric"
        autocomplete="one-time-code"
        placeholder="000000"
        maxlength="12"
        class="w-full rounded-xl border border-line bg-surface-2 px-4 py-3 text-center font-mono text-2xl tracking-[0.4em] text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
      />
      <p class="mt-3 text-xs text-ink-faint">
        The code is used once for this login and is not saved anywhere.
      </p>
      <p v-if="sessions.otpError" class="mt-3 rounded-xl bg-bad/10 px-3 py-2 text-sm text-bad">
        {{ sessions.otpError }}
      </p>
    </form>

    <template #footer>
      <button
        type="button"
        class="rounded-xl px-4 py-2 text-sm font-medium text-ink-muted transition hover:bg-ink/5 hover:text-ink"
        @click="cancel"
      >
        Cancel
      </button>
      <button
        type="button"
        class="rounded-xl bg-brand px-4 py-2 text-sm font-semibold text-surface-1 transition hover:bg-brand-strong disabled:opacity-50"
        :disabled="sessions.otpSubmitting"
        @click="submit"
      >
        {{ sessions.otpSubmitting ? 'Verifying…' : 'Continue' }}
      </button>
    </template>
  </AppModal>
</template>
