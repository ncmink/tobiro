<script setup lang="ts">
import AppModal from '@/components/AppModal.vue'

defineProps<{
  parentName: string
  dependents: { name: string }[]
}>()

const emit = defineEmits<{
  close: []
  confirm: []
}>()
</script>

<template>
  <AppModal
    :title="`Disconnect ${parentName}?`"
    subtitle="This connection has sessions that sit on top of it. The backend closes those first, then this one."
    @close="emit('close')"
  >
    <p class="text-sm text-ink-muted">
      Disconnecting {{ parentName }} will also close:
    </p>
    <ul class="mt-3 list-disc space-y-1 pl-5 text-sm text-ink">
      <li v-for="dependent in dependents" :key="dependent.name">{{ dependent.name }}</li>
    </ul>

    <template #footer>
      <button
        type="button"
        class="rounded-xl px-4 py-2 text-sm font-medium text-ink-muted transition hover:bg-ink/5 hover:text-ink"
        @click="emit('close')"
      >
        Cancel
      </button>
      <button
        type="button"
        class="rounded-xl border border-bad/60 px-4 py-2 text-sm font-semibold text-bad transition hover:bg-bad/10"
        @click="emit('confirm')"
      >
        Disconnect all
      </button>
    </template>
  </AppModal>
</template>
