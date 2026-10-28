<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'

import { ReadSSHCommand } from '@bindings/go/main/App'
import type { domain, main } from '@bindings/go/models'
import { errorMessage } from '@/lib/state'

const props = defineProps<{ connection: main.ConnectionView }>()

const emit = defineEmits<{ save: [input: domain.TunnelInput] }>()

/** One row of the forwards editor, kept as strings while it is being typed. */
interface ForwardRow {
  name: string
  localPort: number | string
  remoteHost: string
  remotePort: number | string
}

function rowsOf(connection: main.ConnectionView): ForwardRow[] {
  return (connection.profile.ssh?.forwards ?? []).map((f) => ({
    name: f.name,
    localPort: f.localPort,
    remoteHost: f.remoteHost,
    remotePort: f.remotePort,
  }))
}

const form = reactive({
  host: props.connection.profile.host,
  port: props.connection.profile.port || 22,
  user: props.connection.profile.ssh?.user ?? '',
  forwards: rowsOf(props.connection) as ForwardRow[],
})

// A save elsewhere — an import, another window, the connection being reloaded —
// should be reflected here rather than sitting behind stale fields. An edit in
// progress is left alone: overwriting what someone is typing is worse than
// showing them a value a moment out of date.
watch(
  () => props.connection.profile,
  (profile) => {
    if (dirty.value) return
    form.host = profile.host
    form.port = profile.port || 22
    form.user = profile.ssh?.user ?? ''
    form.forwards = rowsOf(props.connection)
  },
)

const saved = computed(() => {
  const profile = props.connection.profile
  return shape({
    host: profile.host,
    port: profile.port || 22,
    user: profile.ssh?.user ?? '',
    forwards: rowsOf(props.connection),
  })
})

const dirty = computed(() => shape(form) !== saved.value)

/** A comparable rendering of the whole tunnel, used only to spot a change. */
function shape(state: {
  host: string
  port: number | string
  user: string
  forwards: ForwardRow[]
}): string {
  const ports = state.forwards
    .map((f) => `${f.name}|${f.localPort}|${f.remoteHost}|${f.remotePort}`)
    .join(' ')
  return `${state.host.trim()}|${Number(state.port) || 22}|${state.user.trim()}|${ports}`
}

const saving = ref(false)
const error = ref('')
const note = ref('')

function addForward() {
  form.forwards.push({ name: '', localPort: '', remoteHost: 'localhost', remotePort: '' })
}

function addAPIForward() {
  form.forwards.push({
    name: 'Kubernetes API',
    localPort: 16443,
    remoteHost: 'localhost',
    remotePort: 6443,
  })
}

function removeForward(index: number) {
  form.forwards.splice(index, 1)
}

function revert() {
  form.host = props.connection.profile.host
  form.port = props.connection.profile.port || 22
  form.user = props.connection.profile.ssh?.user ?? ''
  form.forwards = rowsOf(props.connection)
  error.value = ''
  note.value = ''
}

/**
 * The paste field.
 *
 * The command is the artefact engineers actually pass around, so reading one
 * fills the whole section rather than asking for the same six values again. It
 * is parsed in Go, so the rules that accept it here are the ones that validate
 * it on save.
 */
const command = ref('')
const reading = ref(false)

async function readCommand() {
  if (command.value.trim() === '') return
  reading.value = true
  error.value = ''
  note.value = ''
  try {
    const parsed = await ReadSSHCommand(command.value)
    form.host = parsed.profile.host
    form.port = parsed.profile.port
    form.user = parsed.profile.ssh.user
    form.forwards = parsed.profile.ssh.forwards.map((f) => ({
      name: f.name,
      localPort: f.localPort,
      remoteHost: f.remoteHost,
      remotePort: f.remotePort,
    }))
    command.value = ''
    note.value = parsed.privateKeyPath
      ? `Filled in. The key ${parsed.privateKeyPath} can be added in Accounts.`
      : 'Filled in from the command. Check it over, then save.'
  } catch (err) {
    error.value = errorMessage(err, 'Could not read that command')
  } finally {
    reading.value = false
  }
}

function submit() {
  error.value = ''
  note.value = ''

  for (const row of form.forwards) {
    if (!Number(row.localPort) || !Number(row.remotePort)) {
      error.value = 'Every forwarded port needs a port on this machine and one on the far side.'
      return
    }
  }

  saving.value = true
  emit(
    'save',
    {
      host: form.host.trim(),
      port: Number(form.port) || 22,
      ssh: {
        user: form.user.trim(),
        forwards: form.forwards.map((row) => ({
          name: row.name.trim(),
          localPort: Number(row.localPort),
          remoteHost: row.remoteHost.trim() || 'localhost',
          remotePort: Number(row.remotePort),
        })),
      },
    } as unknown as domain.TunnelInput,
  )
}

defineExpose({
  done() {
    saving.value = false
    error.value = ''
      note.value = 'Saved.'
  },
  fail(message: string) {
    error.value = message
    saving.value = false
  },
})
</script>

<template>
  <section class="rounded-2xl border border-line bg-surface-1">
    <header class="border-b border-line px-6 py-4">
      <h2 class="text-base font-semibold text-ink">Server &amp; ports</h2>
      <p class="mt-0.5 text-xs leading-relaxed text-ink-muted">
        Configure the SSH destination and saved port mappings. Changes apply on the next connection.
      </p>
    </header>

    <form class="space-y-4 px-6 py-5" @submit.prevent="submit">
      <div>
        <label class="mb-1.5 block text-sm font-medium text-ink" for="tunnel-command">
          Paste an ssh command <span class="font-normal text-ink-faint">(optional)</span>
        </label>
        <div class="flex gap-2">
          <input
            id="tunnel-command"
            v-model="command"
            type="text"
            spellcheck="false"
            placeholder="ssh -N -L 16443:localhost:6443 server1@172.16.20.65"
            class="w-full min-w-0 rounded-xl border border-line bg-surface-2 px-3 py-2 font-mono text-xs text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
            @keydown.enter.prevent="readCommand"
          />
          <button
            type="button"
            class="no-drag shrink-0 rounded-xl border border-line-strong px-3 py-2 text-sm font-medium text-ink transition hover:border-brand/60 hover:text-brand disabled:opacity-50"
            :disabled="reading || command.trim() === ''"
            @click="readCommand"
          >
            {{ reading ? 'Reading…' : 'Fill in' }}
          </button>
        </div>
      </div>

      <div class="grid grid-cols-[1fr_120px] gap-3">
        <div>
          <label class="mb-1.5 block text-sm font-medium text-ink" for="tunnel-host">
            SSH server
          </label>
          <input
            id="tunnel-host"
            v-model="form.host"
            type="text"
            placeholder="172.16.20.65"
            spellcheck="false"
            class="w-full rounded-xl border border-line bg-surface-2 px-3 py-2 font-mono text-sm text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
          />
        </div>
        <div>
          <label class="mb-1.5 block text-sm font-medium text-ink" for="tunnel-port">Port</label>
          <input
            id="tunnel-port"
            v-model.number="form.port"
            type="number"
            min="1"
            max="65535"
            class="w-full rounded-xl border border-line bg-surface-2 px-3 py-2 font-mono text-sm text-ink outline-none transition focus:border-brand"
          />
        </div>
      </div>

      <div>
        <label class="mb-1.5 block text-sm font-medium text-ink" for="tunnel-user">SSH username</label>
        <input
          id="tunnel-user"
          v-model="form.user"
          type="text"
          placeholder="server1"
          spellcheck="false"
          class="w-full rounded-xl border border-line bg-surface-2 px-3 py-2 font-mono text-sm text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
        />
        <p class="mt-1.5 text-xs leading-relaxed text-ink-muted">
          All accounts for this tunnel use this SSH username. Manage passwords and keys in Accounts.
        </p>
      </div>

      <div>
        <div class="mb-1.5 flex items-center justify-between">
          <span class="text-sm font-medium text-ink">Saved port mappings</span>
          <div class="flex gap-2">
            <button
              type="button"
              class="no-drag rounded-lg border border-line-strong px-2.5 py-1 text-xs font-medium text-ink-muted transition hover:border-brand/60 hover:text-brand"
              @click="addAPIForward"
            >
              + Kubernetes API
            </button>
            <button
              type="button"
              class="no-drag rounded-lg border border-line-strong px-2.5 py-1 text-xs font-medium text-ink-muted transition hover:border-brand/60 hover:text-brand"
              @click="addForward"
            >
              + Port
            </button>
          </div>
        </div>

        <div v-if="form.forwards.length" class="space-y-2">
          <div
            v-for="(row, index) in form.forwards"
            :key="index"
            class="rounded-xl border border-line bg-surface-2 p-2.5"
          >
            <div class="flex items-center gap-2">
              <input
                v-model="row.name"
                type="text"
                placeholder="What this is for"
                class="min-w-0 flex-1 rounded-lg border border-line bg-surface-1 px-2.5 py-1.5 text-xs text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
              />
              <button
                type="button"
                class="no-drag shrink-0 rounded-lg px-2 py-1.5 text-xs font-medium text-ink-faint transition hover:bg-bad/10 hover:text-bad"
                @click="removeForward(index)"
              >
                Remove
              </button>
            </div>
            <div class="mt-2 flex items-center gap-2">
              <input
                v-model="row.localPort"
                type="number"
                min="1"
                max="65535"
                placeholder="16443"
                class="w-24 rounded-lg border border-line bg-surface-1 px-2.5 py-1.5 font-mono text-xs text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
              />
              <span class="shrink-0 text-xs text-ink-faint">on this machine →</span>
              <input
                v-model="row.remoteHost"
                type="text"
                placeholder="localhost"
                spellcheck="false"
                class="min-w-0 flex-1 rounded-lg border border-line bg-surface-1 px-2.5 py-1.5 font-mono text-xs text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
              />
              <span class="shrink-0 text-xs text-ink-faint">:</span>
              <input
                v-model="row.remotePort"
                type="number"
                min="1"
                max="65535"
                placeholder="6443"
                class="w-24 rounded-lg border border-line bg-surface-1 px-2.5 py-1.5 font-mono text-xs text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
              />
            </div>
          </div>
        </div>

        <p v-else class="rounded-xl border border-dashed border-line px-3 py-4 text-center text-xs text-ink-muted">
          No ports yet. A tunnel with nothing to forward has nothing to do.
        </p>

        <p class="mt-1.5 text-xs leading-relaxed text-ink-muted">
          The address on the right is resolved on the SSH server, so
          <span class="font-mono">localhost</span> means the server itself. Ports listen on
          127.0.0.1 only.
        </p>
      </div>

      <p v-if="error" class="rounded-xl bg-bad/10 px-3 py-2 text-sm text-bad">{{ error }}</p>
      <p v-else-if="note" class="text-xs text-ok">{{ note }}</p>

      <div class="flex items-center justify-end gap-2 border-t border-line pt-4">
        <button
          v-if="dirty"
          type="button"
          class="no-drag rounded-xl px-4 py-2 text-sm font-medium text-ink-muted transition hover:bg-ink/5 hover:text-ink"
          @click="revert"
        >
          Discard changes
        </button>
        <button
          type="submit"
          class="no-drag rounded-xl bg-brand px-4 py-2 text-sm font-semibold text-surface-1 transition hover:bg-brand-strong disabled:opacity-50"
          :disabled="saving || !dirty"
        >
          {{ saving ? 'Saving…' : 'Save tunnel' }}
        </button>
      </div>
    </form>
  </section>
</template>
