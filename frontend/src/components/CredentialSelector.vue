<script setup lang="ts">
import type { domain } from '@bindings/go/models'

defineProps<{ credentials: domain.CredentialView[]; modelValue: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
</script>

<template>
  <!--
    Accounts are keyed and submitted by credential UUID. The username is shown
    for recognition only — several accounts may legitimately share one.
  -->
  <div class="space-y-2">
    <label
      v-for="credential in credentials"
      :key="credential.id"
      class="no-drag flex cursor-pointer items-center gap-3 rounded-xl border px-4 py-3 transition"
      :class="
        modelValue === credential.id
          ? 'border-brand/60 bg-brand/10'
          : 'border-line bg-surface-2 hover:border-line-strong'
      "
    >
      <input
        type="radio"
        class="h-4 w-4 accent-brand"
        :value="credential.id"
        :checked="modelValue === credential.id"
        @change="emit('update:modelValue', credential.id)"
      />

      <span class="min-w-0 flex-1">
        <span class="flex items-center gap-2">
          <span class="truncate font-medium text-ink">{{ credential.label }}</span>
          <span
            v-if="credential.isDefault"
            class="rounded-md bg-brand/15 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-brand"
          >
            Default
          </span>
        </span>
        <span class="mt-0.5 flex flex-wrap items-center gap-2 text-xs text-ink-muted">
          <span v-if="credential.username" class="font-mono">{{ credential.username }}</span>
          <span v-if="credential.username" aria-hidden="true">&middot;</span>
          <span :class="credential.hasPassword ? 'text-ok' : 'text-warn'">
            {{ credential.hasPassword ? 'Password saved in Keychain' : 'No password saved' }}
          </span>
        </span>
      </span>
    </label>
  </div>
</template>
