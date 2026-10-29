<script setup lang="ts">
withDefaults(defineProps<{ title: string; subtitle?: string; width?: string }>(), {
  width: 'max-w-lg',
})

const emit = defineEmits<{ close: [] }>()
</script>

<template>
  <div
    class="no-drag fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-6 backdrop-blur-sm"
    @click.self="emit('close')"
  >
    <div
      class="w-full overflow-hidden rounded-2xl border border-line-strong bg-surface-1 shadow-2xl"
      :class="width"
      role="dialog"
      aria-modal="true"
    >
      <header class="flex items-start justify-between gap-4 border-b border-line bg-surface-2/50 px-6 py-5">
        <div>
          <h2 class="text-base font-semibold text-ink">{{ title }}</h2>
          <p v-if="subtitle" class="mt-0.5 text-sm text-ink-muted">{{ subtitle }}</p>
        </div>
        <button
          type="button"
          class="no-drag grid h-9 w-9 shrink-0 place-items-center rounded-lg border border-transparent text-ink-faint transition hover:border-line hover:bg-ink/5 hover:text-ink"
          aria-label="Close"
          @click="emit('close')"
        >
          <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </header>

      <div class="max-h-[70vh] overflow-y-auto px-6 py-5">
        <slot />
      </div>

      <footer v-if="$slots.footer" class="flex justify-end gap-2 border-t border-line bg-surface-0/40 px-6 py-4">
        <slot name="footer" />
      </footer>
    </div>
  </div>
</template>
