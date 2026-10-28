<script setup lang="ts">
import { computed, ref, watch } from 'vue'

import { ListSSHConfigHosts } from '@bindings/go/main/App'
import type { main } from '@bindings/go/models'
import AppModal from '@/components/AppModal.vue'
import { errorMessage } from '@/lib/state'
import { useConnectionsStore } from '@/stores/connections'

const props = defineProps<{
  parent: main.ConnectionView
}>()

const emit = defineEmits<{
  close: []
  done: [message: string]
}>()

const store = useConnectionsStore()
const hosts = ref<main.SSHConfigHostView[]>([])
const selected = ref<Set<string>>(new Set())
const loading = ref(true)
const importing = ref(false)
const error = ref('')

const selectable = computed(() => hosts.value.filter((h) => !h.exists))
const allSelected = computed(
  () => selectable.value.length > 0 && selectable.value.every((h) => selected.value.has(h.alias)),
)

function endpoint(h: main.SSHConfigHostView) {
  const user = h.user?.trim()
  const host = h.hostName?.trim() || h.alias
  const port = h.port && h.port !== 22 ? `:${h.port}` : ''
  return user ? `${user}@${host}${port}` : `${host}${port}`
}

async function loadHosts() {
  loading.value = true
  error.value = ''
  selected.value = new Set()
  try {
    hosts.value = await ListSSHConfigHosts()
  } catch (err) {
    error.value = errorMessage(err, 'Could not read your SSH config')
    hosts.value = []
  } finally {
    loading.value = false
  }
}

function toggle(alias: string) {
  const next = new Set(selected.value)
  if (next.has(alias)) next.delete(alias)
  else next.add(alias)
  selected.value = next
}

function toggleAll() {
  if (allSelected.value) {
    selected.value = new Set()
    return
  }
  selected.value = new Set(selectable.value.map((h) => h.alias))
}

async function submit() {
  if (selected.value.size === 0) return
  importing.value = true
  error.value = ''
  try {
    const result = await store.importSshConfigFromHosts(
      props.parent.profile.id,
      [...selected.value],
    )
    const parts: string[] = []
    if (result.connections > 0) {
      parts.push(
        `Added ${result.connections} ${result.connections === 1 ? 'tunnel' : 'tunnels'} under “${props.parent.profile.name}”`,
      )
      if (result.accounts > 0) {
        parts.push(
          `${result.accounts} ${result.accounts === 1 ? 'account needs' : 'accounts need'} a password`,
        )
      }
    }
    if (result.skipped?.length) {
      parts.push(`skipped ${result.skipped.join(', ')} — already here`)
    }
    emit('done', parts.length ? `${parts.join(' · ')}.` : 'Nothing new was imported.')
    emit('close')
  } catch (err) {
    error.value = errorMessage(err, 'Could not import SSH tunnels')
  } finally {
    importing.value = false
  }
}

watch(
  () => props.parent.profile.id,
  () => void loadHosts(),
  { immediate: true },
)
</script>

<template>
  <AppModal
    :title="'Import from SSH config'"
    :subtitle="`Tunnels will connect through ${parent.profile.name}.`"
    width="max-w-xl"
    @close="emit('close')"
  >
    <p v-if="loading" class="text-sm text-ink-muted">Reading ~/.ssh/config…</p>
    <p v-else-if="error" class="rounded-xl bg-bad/10 px-4 py-3 text-sm text-bad">{{ error }}</p>
    <p v-else-if="hosts.length === 0" class="text-sm text-ink-muted">
      No Host entries were found in ~/.ssh/config, or the file does not exist yet.
    </p>
    <div v-else class="space-y-3">
      <label
        v-if="selectable.length"
        class="flex cursor-pointer items-center gap-2 rounded-lg border border-line px-3 py-2 text-sm text-ink-muted hover:bg-surface-2"
      >
        <input
          type="checkbox"
          class="h-4 w-4 accent-brand"
          :checked="allSelected"
          @change="toggleAll"
        />
        Select all ({{ selectable.length }})
      </label>

      <ul class="max-h-80 space-y-1 overflow-y-auto rounded-xl border border-line p-1">
        <li v-for="host in hosts" :key="host.alias">
          <label
            class="flex items-start gap-3 rounded-lg px-3 py-2.5"
            :class="
              host.exists
                ? 'cursor-not-allowed opacity-50'
                : 'cursor-pointer hover:bg-surface-2'
            "
          >
            <input
              type="checkbox"
              class="mt-0.5 h-4 w-4 shrink-0 accent-brand"
              :disabled="host.exists"
              :checked="selected.has(host.alias)"
              @change="toggle(host.alias)"
            />
            <span class="min-w-0 flex-1">
              <span class="block font-medium text-ink">{{ host.alias }}</span>
              <span class="block truncate font-mono text-xs text-ink-muted">{{ endpoint(host) }}</span>
              <span v-if="host.forwardCount" class="mt-0.5 block text-xs text-ink-faint">
                {{ host.forwardCount }} port {{ host.forwardCount === 1 ? 'forward' : 'forwards' }}
              </span>
              <span v-if="host.exists" class="mt-0.5 block text-xs text-ink-faint">Already added</span>
            </span>
          </label>
        </li>
      </ul>
    </div>

    <template #footer>
      <button
        type="button"
        class="rounded-lg px-3 py-2 text-sm text-ink-muted hover:bg-ink/5"
        @click="emit('close')"
      >
        Cancel
      </button>
      <button
        type="button"
        class="rounded-lg bg-brand px-4 py-2 text-sm font-semibold text-surface-1 hover:bg-brand-strong disabled:opacity-40"
        :disabled="importing || selected.size === 0"
        @click="submit"
      >
        {{ importing ? 'Importing…' : selected.size ? `Import ${selected.size}` : 'Import' }}
      </button>
    </template>
  </AppModal>
</template>
