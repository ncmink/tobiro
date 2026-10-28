<script setup lang="ts">
defineProps<{ id?: string; modelValue: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
</script>

<template>
  <!--
    The native control keeps its keyboard behaviour and the platform option
    list; only the closed field is restyled, with the browser's own arrow
    replaced by one drawn in the palette so it sits level with the text inputs
    beside it.
  -->
  <div class="relative">
    <select
      :id="id"
      :value="modelValue"
      class="w-full appearance-none rounded-xl border border-line bg-surface-2 py-2 pl-3 pr-10 text-sm text-ink outline-none transition hover:border-line-strong focus:border-brand"
      @change="emit('update:modelValue', ($event.target as HTMLSelectElement).value)"
    >
      <slot />
    </select>
    <span
      class="pointer-events-none absolute inset-y-0 right-2.5 flex items-center text-ink-faint"
      aria-hidden="true"
    >
      <svg class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
        <path
          fill-rule="evenodd"
          d="M5.23 7.21a.75.75 0 011.06.02L10 11.17l3.71-3.94a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z"
          clip-rule="evenodd"
        />
      </svg>
    </span>
  </div>
</template>
