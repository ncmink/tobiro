<script setup lang="ts">
import { computed } from 'vue'

import type { domain } from '@/lib/bindings'
import {
  MAX_TAG_KEY_LEN,
  MAX_TAG_VALUE_LEN,
  MAX_TAGS,
  NAME_TAG_KEY,
  validateStoredTags,
} from '@/lib/tags'

const name = defineModel<string>('name', { required: true })
const tags = defineModel<domain.Tag[]>('tags', { required: true })

const tagError = computed(() => validateStoredTags(tags.value))

function rowError(index: number): string {
  const t = tags.value[index]
  if (!t) return ''
  const key = t.key.trim()
  if (!key && t.value.trim()) return 'Enter a key.'
  if (key && key.length > MAX_TAG_KEY_LEN) return `Key too long (max ${MAX_TAG_KEY_LEN}).`
  if (t.value.length > MAX_TAG_VALUE_LEN) return `Value too long (max ${MAX_TAG_VALUE_LEN}).`
  if (key === NAME_TAG_KEY) return `"${NAME_TAG_KEY}" is reserved for the connection name.`
  if (key) {
    const dup = tags.value.findIndex((other, i) => i !== index && other.key.trim() === key)
    if (dup >= 0) return 'Duplicate key.'
  }
  return ''
}

function addRow() {
  if (tags.value.length >= MAX_TAGS) return
  tags.value = [...tags.value, { key: '', value: '' }]
}

function removeRow(index: number) {
  tags.value = tags.value.filter((_, i) => i !== index)
}

defineExpose({
  validate(): string | null {
    if (!name.value.trim()) return 'Give this connection a name.'
    return tagError.value
  },
})
</script>

<template>
  <div class="space-y-2">
    <div class="grid grid-cols-[7.5rem_minmax(0,1fr)] gap-2.5 text-xs font-medium text-ink-faint">
      <span>Key</span>
      <span>Value</span>
    </div>

    <div class="grid grid-cols-[7.5rem_minmax(0,1fr)] items-center gap-2.5">
      <input
        type="text"
        :value="NAME_TAG_KEY"
        disabled
        class="w-full rounded-xl border border-line bg-surface-3 px-3 py-2 text-sm text-ink-faint select-none"
        aria-label="Tag key Name"
      />
      <input
        v-model="name"
        type="text"
        class="w-full rounded-xl border border-line bg-surface-2 px-3 py-2 text-sm text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
        placeholder="Connection name"
        aria-label="Connection name (Name tag)"
      />
    </div>

    <div
      v-for="(tag, index) in tags"
      :key="index"
      class="grid grid-cols-[7.5rem_minmax(0,1fr)] items-start gap-x-2.5 gap-y-1"
    >
      <input
        v-model="tag.key"
        type="text"
        placeholder="e.g. env"
        class="w-full rounded-xl border border-line bg-surface-2 px-3 py-2 text-sm text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
      />
      <div class="relative min-w-0">
        <input
          v-model="tag.value"
          type="text"
          placeholder="e.g. prod"
          class="w-full rounded-xl border border-line bg-surface-2 py-2 pr-9 pl-3 text-sm text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
        />
        <button
          type="button"
          class="no-drag absolute top-1/2 right-1.5 grid h-6 w-6 -translate-y-1/2 place-items-center rounded-lg text-ink-faint transition hover:bg-bad/15 hover:text-bad"
          title="Remove tag"
          aria-label="Remove tag"
          @click="removeRow(index)"
        >
          <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
      <p v-if="rowError(index)" class="col-span-2 text-xs text-bad">{{ rowError(index) }}</p>
    </div>

    <div class="pt-0.5">
      <button
        type="button"
        class="no-drag rounded-lg border border-line-strong px-2.5 py-1 text-xs font-medium text-ink-muted transition hover:border-brand/60 hover:text-brand disabled:opacity-40"
        :disabled="tags.length >= MAX_TAGS"
        @click="addRow"
      >
        + Add tag
      </button>
    </div>
    <p v-if="tagError" class="text-xs text-bad">{{ tagError }}</p>
    <p class="text-xs leading-relaxed text-ink-muted">
      Search the dashboard by tag or <span class="font-mono">key=value</span>, e.g. <span class="font-mono">env=prod</span>.
    </p>
  </div>
</template>
