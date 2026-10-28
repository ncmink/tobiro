<script setup lang="ts">
import { computed } from 'vue'
import { stateStyle } from '@/lib/state'

const props = withDefaults(
  defineProps<{ state: string; compact?: boolean; duration?: string }>(),
  { compact: false },
)

const style = computed(() => stateStyle(props.state))
</script>

<template>
  <span
    class="inline-flex items-center gap-1.5 rounded-full border border-line/60 bg-surface-0/60"
    :class="compact ? 'px-2 py-0.5 text-[11px] leading-tight' : 'px-2.5 py-1 text-xs'"
  >
    <span class="h-1.5 w-1.5 shrink-0 rounded-full" :class="style.dot" />
    <span class="font-medium" :class="style.text">{{ style.label }}</span>
    <template v-if="duration">
      <span class="text-ink-faint" aria-hidden="true">·</span>
      <span class="font-mono text-ink-faint">{{ duration }}</span>
    </template>
  </span>
</template>
