<script setup lang="ts">
import { computed, ref, watch } from 'vue'

import { OpenInKube } from '@bindings/go/main/App'
import type { main } from '@bindings/go/models'
import AppModal from '@/components/AppModal.vue'
import { errorMessage } from '@/lib/state'

const props = defineProps<{
  connection: main.ConnectionView
}>()

const emit = defineEmits<{
  close: []
  done: [message: string]
}>()

const clusterId = ref('')
const clusterName = ref('')
const customerId = ref('')
const customerName = ref('')
const server = ref('')
const saving = ref(false)
const error = ref('')

const forwards = computed(() => props.connection.session?.forwards ?? [])

function slug(value: string) {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

function guessServer() {
  for (const forward of forwards.value) {
    if (forward.remotePort === 6443) {
      const host = forward.remoteHost?.trim()
      if (!host || host === 'localhost' || host === '127.0.0.1') {
        if (props.connection.profile.host) {
          server.value = `https://${props.connection.profile.host}:6443`
          return
        }
      }
      server.value = `https://${host}:${forward.remotePort}`
      return
    }
  }
}

watch(
  () => props.connection,
  (connection) => {
    clusterId.value = ''
    clusterName.value = ''
    customerName.value = connection.profile.group || ''
    customerId.value = slug(connection.profile.group || connection.profile.name)
    server.value = ''
    error.value = ''
    guessServer()
  },
  { immediate: true },
)

async function submit() {
  error.value = ''
  saving.value = true
  try {
    const result = await OpenInKube({
      profileId: props.connection.profile.id,
      clusterId: clusterId.value.trim(),
      clusterName: clusterName.value.trim(),
      customerId: customerId.value.trim(),
      customerName: customerName.value.trim(),
      server: server.value.trim(),
    })
    const how = result.delivered
      ? 'Biebie Kube opened the cluster.'
      : result.launched
        ? 'Launched Biebie Kube with this cluster.'
        : 'Handoff created.'
    emit('done', how)
    emit('close')
  } catch (err) {
    error.value = errorMessage(err, 'Could not open Biebie Kube')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <AppModal title="Open in Biebie Kube" @close="emit('close')">
    <p class="mb-4 text-sm leading-relaxed text-ink-muted">
      Name the Kubernetes cluster this tunnel reaches. Tobiro only hands over
      identifiers — nothing here signs you in.
    </p>

    <form class="space-y-4" @submit.prevent="submit">
      <label class="block text-sm">
        <span class="mb-1 block font-medium text-ink">Cluster id</span>
        <input
          v-model="clusterId"
          required
          class="w-full rounded-lg border border-line bg-surface-2 px-3 py-2 font-mono text-sm"
          placeholder="rke2-prod"
        />
      </label>

      <label class="block text-sm">
        <span class="mb-1 block font-medium text-ink">Cluster name</span>
        <input
          v-model="clusterName"
          required
          class="w-full rounded-lg border border-line bg-surface-2 px-3 py-2 text-sm"
          placeholder="RKE2 Production"
        />
      </label>

      <div class="grid gap-4 sm:grid-cols-2">
        <label class="block text-sm">
          <span class="mb-1 block font-medium text-ink">Customer id</span>
          <input
            v-model="customerId"
            required
            class="w-full rounded-lg border border-line bg-surface-2 px-3 py-2 font-mono text-sm"
          />
        </label>
        <label class="block text-sm">
          <span class="mb-1 block font-medium text-ink">Customer name</span>
          <input
            v-model="customerName"
            required
            class="w-full rounded-lg border border-line bg-surface-2 px-3 py-2 text-sm"
          />
        </label>
      </div>

      <label class="block text-sm">
        <span class="mb-1 block font-medium text-ink">API server <span class="font-normal text-ink-muted">(optional)</span></span>
        <input
          v-model="server"
          class="w-full rounded-lg border border-line bg-surface-2 px-3 py-2 font-mono text-sm"
          placeholder="https://172.16.20.65:6443"
        />
      </label>

      <p v-if="error" role="alert" class="rounded-lg bg-bad/10 px-3 py-2 text-sm text-bad">{{ error }}</p>

      <div class="flex justify-end gap-2 pt-2">
        <button type="button" class="rounded-lg px-4 py-2 text-sm text-ink-muted hover:text-ink" @click="emit('close')">
          Cancel
        </button>
        <button
          type="submit"
          class="rounded-lg bg-brand/15 px-4 py-2 text-sm font-semibold text-brand hover:bg-brand/25 disabled:opacity-50"
          :disabled="saving"
        >
          {{ saving ? 'Opening…' : 'Open in Biebie Kube' }}
        </button>
      </div>
    </form>
  </AppModal>
</template>
