<script setup lang="ts">
import AppModal from '@/components/AppModal.vue'

defineProps<{
  name: string
  /** Connections that currently list this one under “needs first”. */
  dependents: { name: string }[]
}>()

const emit = defineEmits<{
  close: []
  confirm: []
}>()
</script>

<template>
  <AppModal
    :title="`Delete “${name}”?`"
    subtitle="This cannot be undone. Every account and stored password for this connection is removed."
    @close="emit('close')"
  >
    <p v-if="dependents.length" class="text-sm text-ink-muted">
      These connections currently wait for this one first. Deleting clears that link — they will not auto-connect through it anymore:
    </p>
    <ul v-if="dependents.length" class="mt-3 list-disc space-y-1 pl-5 text-sm text-ink">
      <li v-for="dependent in dependents" :key="dependent.name">{{ dependent.name }}</li>
    </ul>
    <p v-else class="text-sm text-ink-muted">
      No other connection depends on this one.
    </p>

    <template #footer>
      <button
        type="button"
        class="no-drag rounded-xl px-4 py-2 text-sm font-medium text-ink-muted transition hover:bg-ink/5 hover:text-ink"
        @click="emit('close')"
      >
        Cancel
      </button>
      <button
        type="button"
        class="no-drag rounded-xl bg-bad px-4 py-2 text-sm font-semibold text-white transition hover:bg-bad/80"
        @click="emit('confirm')"
      >
        Delete connection
      </button>
    </template>
  </AppModal>
</template>
