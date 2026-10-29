<script setup lang="ts">
import { computed, ref } from 'vue'

import type { main } from '@bindings/go/models'
import AppModal from '@/components/AppModal.vue'
import CredentialSelector from '@/components/CredentialSelector.vue'

const props = defineProps<{
  connection: main.ConnectionView
  mode?: 'connect' | 'fill'
}>()
const emit = defineEmits<{ close: [], connect: [credentialId: string] }>()

const fill = computed(() => props.mode === 'fill')

const title = computed(() =>
  fill.value ? `Open ${props.connection.profile.name}` : `Connect to ${props.connection.profile.name}`,
)
const subtitle = computed(() =>
  fill.value
    ? 'Choose which account to fill into the vendor login prompt. You still click Connect there.'
    : 'Choose which account to sign in with.',
)
const actionLabel = computed(() => (fill.value ? 'Open and fill' : 'Connect'))

// Preselect the last account used on this profile, falling back to the
// profile default. Both are tracked by credential UUID, never by username.
const selected = ref(
  props.connection.suggestedCredentialId || props.connection.credentials[0]?.id || '',
)
</script>

<template>
  <AppModal :title="title" :subtitle="subtitle" @close="emit('close')">
    <CredentialSelector v-model="selected" :credentials="connection.credentials" />

    <p v-if="!connection.credentials.length" class="text-sm text-ink-muted">
      This connection has no accounts yet.
    </p>

    <template #footer>
      <button type="button" class="btn-ghost" @click="emit('close')">
        Cancel
      </button>
      <button
        type="button"
        class="btn-brand"
        :disabled="!selected"
        @click="emit('connect', selected)"
      >
        {{ actionLabel }}
      </button>
    </template>
  </AppModal>
</template>
