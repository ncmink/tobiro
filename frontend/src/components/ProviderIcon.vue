<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{ provider: string; size?: 'sm' | 'md' }>(),
  { size: 'md' },
)

/** Vendor marks are drawn as initials so the app ships no third-party logos. */
const MARKS: Record<string, { initials: string; classes: string }> = {
  forcepoint: { initials: 'FP', classes: 'bg-sky-500/15 text-sky-300 ring-sky-500/30' },
  ivanti: { initials: 'IV', classes: 'bg-emerald-500/15 text-emerald-300 ring-emerald-500/30' },
  forticlient: { initials: 'FC', classes: 'bg-red-500/15 text-red-300 ring-red-500/30' },
	mock: { initials: 'MK', classes: 'bg-violet-500/15 text-violet-300 ring-violet-500/30' },
  tailscale: { initials: 'TS', classes: 'bg-zinc-200/15 text-zinc-200 ring-zinc-400/30' },
  surfshark: { initials: 'SS', classes: 'bg-cyan-500/15 text-cyan-300 ring-cyan-500/30' },
  macosvpn: { initials: 'OS', classes: 'bg-slate-400/15 text-slate-200 ring-slate-400/30' },
  // Not a vendor: an SSH tunnel is this application's own doing.
  ssh: { initials: 'SSH', classes: 'bg-amber-500/15 text-amber-300 ring-amber-500/30' },
}

const mark = computed(
  () => MARKS[props.provider] ?? { initials: '??', classes: 'bg-ink/5 text-ink-muted ring-ink/10' },
)

const sizing = computed(() =>
  props.size === 'sm' ? 'h-8 w-8 text-[11px]' : 'h-10 w-10 text-xs',
)
</script>

<template>
  <span
    :data-provider="provider"
    class="provider-mark inline-flex shrink-0 items-center justify-center rounded-xl font-bold tracking-wide ring-1"
    :class="[mark.classes, sizing]"
  >
    <svg v-if="provider === 'ssh'" aria-hidden="true" class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
      <rect x="2" y="7" width="5" height="10" rx="1.5" />
      <rect x="17" y="7" width="5" height="10" rx="1.5" />
      <path d="M7 12h10m-7-3 3 3-3 3" />
    </svg>
    <template v-else>{{ mark.initials }}</template>
  </span>
</template>

<style>
[data-theme='light'] .provider-mark { color: #52556b; }
[data-theme='light'] [data-provider='forcepoint'] { color: #0369a1; }
[data-theme='light'] [data-provider='ivanti'] { color: #047857; }
[data-theme='light'] [data-provider='forticlient'] { color: #b91c1c; }
[data-theme='light'] [data-provider='mock'] { color: #6d28d9; }
[data-theme='light'] [data-provider='surfshark'] { color: #0e7490; }
[data-theme='light'] [data-provider='ssh'] { color: #92400e; }
</style>
