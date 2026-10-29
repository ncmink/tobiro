<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'

import type { domain, main } from '@bindings/go/models'
import ConnectDialog from '@/components/ConnectDialog.vue'
import ConnectionForm from '@/components/ConnectionForm.vue'
import ConnectionStatus from '@/components/ConnectionStatus.vue'
import CredentialForm from '@/components/CredentialForm.vue'
import DeleteConnectionDialog from '@/components/DeleteConnectionDialog.vue'
import DisconnectChainDialog from '@/components/DisconnectChainDialog.vue'
import ForwardList from '@/components/ForwardList.vue'
import ProviderIcon from '@/components/ProviderIcon.vue'
import TunnelSettings from '@/components/TunnelSettings.vue'
import { connectionId, connectionParentId, descendants, findNode, forest } from '@/lib/chain'
import { accountToConnectWith, canConnect, errorMessage, formatDuration, isActive } from '@/lib/state'
import { useConnectionsStore } from '@/stores/connections'
import { useSessionsStore } from '@/stores/sessions'

const props = defineProps<{ id: string }>()

const route = useRoute()
const router = useRouter()
const store = useConnectionsStore()
const sessions = useSessionsStore()

const connection = computed(() => store.byId(props.id))

const section = ref<'overview' | 'accounts' | 'settings'>('overview')
const sections = computed(() => [
  { id: 'overview' as const, label: 'Overview' },
  ...(!connection.value?.managesOwnAccount ? [{ id: 'accounts' as const, label: 'Accounts' }] : []),
  { id: 'settings' as const, label: 'Settings' },
])
const showEditForm = ref(false)
const editForm = ref<InstanceType<typeof ConnectionForm> | null>(null)
const tunnelSettings = ref<InstanceType<typeof TunnelSettings> | null>(null)
const credentialForm = ref<InstanceType<typeof CredentialForm> | null>(null)
const editingCredential = ref<domain.CredentialView | null>(null)
const showCredentialForm = ref(false)
const showConnectDialog = ref(false)
const showFillDialog = ref(false)
const pendingDisconnect = ref(false)

// fromPeer explains why this page appeared on its own, for the cases where no
// dialog opens with it. Without it the window jumps somewhere unasked.
const fromPeer = ref(false)
const showDeleteDialog = ref(false)
const pageError = ref('')
const startup = ref<domain.StartupState | null>(null)
const startupBusy = ref(false)
const diagnosis = ref<domain.NetworkDiagnosis | null>(null)
const copiedSecretRef = ref('')
let copiedSecretTimer: number | undefined
let diagnosisPoll: number | undefined

function secretRefOf(credential: domain.CredentialView): string {
  return credential.secretRef || `tobiro/${credential.profileId}/${credential.id}`
}

async function copySecretRef(credential: domain.CredentialView) {
  const refText = secretRefOf(credential)
  try {
    await navigator.clipboard.writeText(refText)
    copiedSecretRef.value = credential.id
    if (copiedSecretTimer !== undefined) window.clearTimeout(copiedSecretTimer)
    copiedSecretTimer = window.setTimeout(() => {
      copiedSecretRef.value = ''
    }, 1500)
  } catch {
    // Ignore clipboard rejection; the reference is still shown in the row and tooltip.
  }
}

const state = computed(() => sessions.sessionFor(props.id)?.state ?? connection.value?.state ?? 'disconnected')
const session = computed(() => sessions.sessionFor(props.id) ?? connection.value?.session)
const active = computed(() => isActive(state.value))
const capabilities = computed(() => connection.value?.capabilities)

const activeAccount = computed(() => {
  const credentialId = session.value?.credentialId
  if (!credentialId || !connection.value) return null
  return connection.value.credentials.find((c) => c.id === credentialId) ?? null
})

const endpoint = computed(() => connection.value?.endpoint ?? '')

const isSSH = computed(() => connection.value?.profile.kind === 'ssh')
const forwards = computed(() => session.value?.forwards ?? [])

/** The connection this one is reached through, with its live state. */
const requires = computed(() => connection.value?.requires ?? null)
const requiresState = computed(() => {
  const needed = requires.value
  if (!needed) return 'disconnected'
  return sessions.sessionFor(needed.profileId)?.state ?? needed.state ?? 'disconnected'
})

const activeDependents = computed(() => {
  const node = findNode(
    forest(store.connections, connectionId, connectionParentId),
    props.id,
    connectionId,
  )
  if (!node) return []
  return descendants(node).filter((child) =>
    isActive(sessions.sessionFor(child.profile.id)?.state ?? child.state),
  )
})

function requestDisconnect() {
  if (activeDependents.value.length > 0) {
    pendingDisconnect.value = true
    return
  }
  void sessions.disconnect(props.id)
}

function confirmDisconnectAll() {
  pendingDisconnect.value = false
  void sessions.disconnect(props.id)
}

/**
 * Whether the saved port map has moved on from the running one.
 *
 * A live tunnel keeps the ports it bound at connect time, so saving an edit
 * while connected leaves the two disagreeing until the next connect. The list
 * shows what is actually bound; this says why it may not be what was just typed.
 */
const forwardsPending = computed(() => {
  if (!isSSH.value || state.value !== 'connected') return false
  const saved = connection.value?.profile.ssh?.forwards ?? []
  const shape = (list: readonly domain.PortForward[]) =>
    list.map((f) => `${f.localPort}:${f.remoteHost}:${f.remotePort}`).join(' ')
  return shape(saved) !== shape(forwards.value)
})

const connectable = computed(() => {
  const target = connection.value
  return target ? canConnect(target) : false
})

function openCredentialForm(credential: domain.CredentialView | null) {
  editingCredential.value = credential
  showCredentialForm.value = true
}

async function saveCredential(input: domain.CredentialInput) {
  try {
    if (editingCredential.value) {
      await store.updateCredential(editingCredential.value.id, input)
    } else {
      await store.addCredential(props.id, input)
    }
    showCredentialForm.value = false
    editingCredential.value = null
  } catch (err) {
    credentialForm.value?.fail(errorMessage(err, 'Could not save the account'))
  }
}

async function saveConnection(input: domain.ProfileInput) {
  try {
    await store.updateConnection(props.id, input)
    showEditForm.value = false
  } catch (err) {
    editForm.value?.fail(errorMessage(err, 'Could not save the connection'))
  }
}

async function saveTunnel(input: domain.TunnelInput) {
  try {
    await store.saveTunnel(props.id, input)
    tunnelSettings.value?.done()
  } catch (err) {
    tunnelSettings.value?.fail(errorMessage(err, 'Could not save the tunnel'))
  }
}

async function removeCredential(credential: domain.CredentialView) {
  pageError.value = ''
  try {
    await store.removeCredential(credential.id)
  } catch (err) {
    pageError.value = errorMessage(err, 'Could not delete the account')
  }
}

async function deleteConnection() {
  pageError.value = ''
  try {
    await store.removeConnection(props.id)
    showDeleteDialog.value = false
    await router.push({ name: 'dashboard' })
  } catch (err) {
    pageError.value = errorMessage(err, 'Could not delete the connection')
  }
}

function deleteDependents() {
  if (!connection.value) return []
  return store.connections
    .filter((c) => c.profile.requires === connection.value!.profile.id)
    .map((c) => ({ name: c.profile.name }))
}

function requestConnect(target: main.ConnectionView) {
  sessions.clearError(props.id)
  const account = accountToConnectWith(target)
  if (account === null) {
    showConnectDialog.value = true
    return
  }
  void sessions.connect(props.id, account)
}

async function confirmConnect(credentialId: string) {
  showConnectDialog.value = false
  await sessions.connect(props.id, credentialId)
  await store.refresh()
}

function requestOpenNative(target: main.ConnectionView) {
  sessions.clearError(props.id)
  if (target.credentials.length === 0) {
    void sessions.openNativeClient(props.id, '')
    return
  }
  if (target.credentials.length === 1) {
    void sessions.openNativeClient(props.id, target.credentials[0].id)
    return
  }
  showFillDialog.value = true
}

async function confirmFill(credentialId: string) {
  showFillDialog.value = false
  await sessions.openNativeClient(props.id, credentialId)
}

async function loadStartup() {
  if (!connection.value?.capabilities.canControlStartup) {
    startup.value = null
    return
  }
  try {
    startup.value = await store.clientStartup(props.id)
  } catch (err) {
    pageError.value = errorMessage(err, 'Could not read start-at-login')
  }
}

async function toggleStartup(enabled: boolean) {
  startupBusy.value = true
  pageError.value = ''
  try {
    startup.value = await store.setClientStartup(props.id, enabled)
  } catch (err) {
    pageError.value = errorMessage(err, 'Could not change start-at-login')
    await loadStartup()
  } finally {
    startupBusy.value = false
  }
}

/**
 * Honours a request from another Biebie application, which arrives as
 * `?connect=1` after App.vue collects it from Go.
 *
 * The connection is opened and armed, not started. The intent came from outside
 * this application, so the click that raises a tunnel stays here — and the
 * account chooser only appears when there is actually an account to choose,
 * rather than as a dialog with nothing in it.
 */
watch(
  [() => route.query.connect, connection],
  ([asked, target]) => {
    if (!asked || !target) return

    // Cleared straight away, so reloading the view or navigating back does not
    // reopen a dialog the user has already dismissed.
    void router.replace({ name: 'connection', params: { id: props.id } })
    fromPeer.value = true

    if (isActive(state.value)) return
    if (!target.capabilities.canConnect) return
    if (!target.capabilities.supportsPassword) return
    if (target.credentials.length === 0) return
    showConnectDialog.value = true
  },
  { immediate: true },
)

watch(() => props.id, () => {
  fromPeer.value = false
  section.value = 'overview'
  showEditForm.value = false
  showCredentialForm.value = false
  showDeleteDialog.value = false
  pageError.value = ''
  void loadStartup()
  void loadDiagnosis()
})

watch(state, () => {
  void loadDiagnosis()
})

async function loadDiagnosis() {
  if (state.value !== 'connected') {
    diagnosis.value = null
    return
  }
  try {
    const result = await store.networkDiagnosis(props.id)
    diagnosis.value = result.known ? result : null
  } catch {
    diagnosis.value = null
  }
}

onMounted(() => {
  void loadStartup()
  void loadDiagnosis()
  diagnosisPoll = window.setInterval(() => {
    if (state.value === 'connected') void loadDiagnosis()
  }, 8_000)
})

onUnmounted(() => {
  if (diagnosisPoll !== undefined) window.clearInterval(diagnosisPoll)
  if (copiedSecretTimer !== undefined) window.clearTimeout(copiedSecretTimer)
})

</script>

<template>
  <div v-if="!connection" class="py-20 text-center">
    <p class="text-ink-muted">This connection no longer exists.</p>
    <RouterLink :to="{ name: 'dashboard' }" class="mt-3 inline-block text-sm text-brand">
      Back to connections
    </RouterLink>
  </div>

  <div v-else class="mx-auto w-full max-w-5xl space-y-5">
    <RouterLink
      :to="{ name: 'dashboard' }"
      class="inline-flex items-center gap-1.5 text-sm text-ink-muted transition hover:text-ink"
    >
      &larr; Connections
    </RouterLink>

    <!--
      Says why the window came forward on its own. It stays until the connection
      is up, because until then the request has not been acted on and the notice
      is still the explanation for what the user is looking at.
    -->
    <p
      v-if="fromPeer && !active"
      class="flex items-start gap-2 rounded-xl border border-brand/30 bg-brand/10 px-4 py-3 text-sm text-ink"
    >
      <span aria-hidden="true">&rarr;</span>
      <span>
        Another Biebie application asked to bring this connection up. Choose an
        account and connect when you are ready.
      </span>
    </p>

    <!-- Overview -->
    <section class="rounded-2xl border border-line bg-surface-1 p-6">
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div class="flex min-w-0 items-start gap-4">
          <ProviderIcon
            v-if="connection.profile.kind !== 'ssh'"
            :provider="connection.profile.provider"
          />
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h1 class="text-xl font-bold text-ink">{{ connection.profile.name }}</h1>
              <span
                v-if="connection.profile.group"
                class="rounded-full bg-surface-2 px-2.5 py-0.5 text-xs text-ink-muted"
                :title="
                  connection.hidden
                    ? 'This group is hidden from the connection list'
                    : 'Connection group'
                "
              >
                {{ connection.profile.group }}{{ connection.hidden ? ' · hidden' : '' }}
              </span>
            </div>
            <p class="mt-0.5 text-sm text-ink-muted">{{ connection.providerName }}</p>
            <p class="mt-0.5 break-all font-mono text-xs text-ink-muted">{{ endpoint }}</p>
          </div>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <ConnectionStatus :state="state" class="mr-2" />
          <button
            v-if="active && capabilities?.canDisconnect"
            type="button"
            class="no-drag rounded-xl border border-line-strong px-4 py-2 text-sm font-medium text-ink transition hover:border-bad/60 hover:text-bad disabled:opacity-50"
            :disabled="sessions.isBusy(id)"
            @click="requestDisconnect"
          >
            {{ activeDependents.length ? 'Disconnect all' : 'Disconnect' }}
          </button>
          <button
            v-else-if="!active && connectable"
            type="button"
            class="no-drag rounded-xl bg-brand px-4 py-2 text-sm font-semibold text-surface-1 transition hover:bg-brand-strong disabled:opacity-50"
            :disabled="sessions.isBusy(id)"
            @click="requestConnect(connection)"
          >
            Connect
          </button>
          <button
            v-if="capabilities?.canOpenNativeClient"
            type="button"
            class="no-drag rounded-xl border border-line-strong px-4 py-2 text-sm font-medium text-ink transition hover:border-brand/60 hover:text-brand"
            @click="requestOpenNative(connection)"
          >
            {{ capabilities.nativeClientLabel || (connection.profile.provider === 'forcepoint' ? 'Open Forcepoint' : 'Open client') }}
          </button>
          <button
            type="button"
            class="no-drag rounded-xl border px-4 py-2 text-sm font-medium transition disabled:cursor-not-allowed disabled:opacity-40"
            :class="
              active
                ? 'border-line text-ink-faint'
                : 'border-bad/30 bg-bad/10 text-bad hover:border-bad/60 hover:bg-bad/20'
            "
            :disabled="active"
            :title="active ? 'Disconnect before deleting this connection' : 'Delete this connection'"
            @click="showDeleteDialog = true"
          >
            Delete
          </button>
        </div>
      </div>

    </section>

    <nav class="flex gap-1 border-b border-line" aria-label="Connection sections">
      <button
        v-for="item in sections"
        :key="item.id"
        type="button"
        :aria-pressed="section === item.id"
        class="no-drag rounded-t-lg border-b-2 px-4 py-3 text-sm font-semibold transition focus-visible:outline-2 focus-visible:outline-brand"
        :class="section === item.id ? 'border-brand text-brand' : 'border-transparent text-ink-muted hover:bg-surface-2 hover:text-ink'"
        @click="section = item.id"
      >{{ item.label }}</button>
    </nav>

    <section v-show="section === 'overview'" aria-label="Connection overview" class="rounded-2xl border border-line bg-surface-1 p-6">
      <h2 class="text-base font-semibold text-ink">Current session</h2>
      <p v-if="!active" class="mt-2 text-sm text-ink-muted">
        {{ isSSH ? 'Connect to open your saved ports. Live addresses and session details will appear here.' : 'Connect to see your assigned address, account and network status.' }}
      </p>
      <p v-else-if="state !== 'connected'" class="mt-2 text-sm text-ink-muted">The connection is updating. Its status is shown above.</p>
      <!-- The order this connection has to be reached in. It is shown whatever
           the state, because it is the answer to "why did Connect start
           something else". -->
      <div
        v-if="requires"
        class="mt-5 flex items-start gap-3 rounded-xl border border-line bg-surface-2 px-4 py-3"
      >
        <div class="min-w-0 flex-1">
          <p class="text-xs text-ink-faint">Reached through</p>
          <RouterLink
            :to="{ name: 'connection', params: { id: requires.profileId } }"
            class="no-drag text-sm font-medium text-ink underline-offset-2 transition hover:text-brand hover:underline"
          >
            {{ requires.name }}
          </RouterLink>
          <p class="mt-1 text-xs leading-relaxed text-ink-muted">
            Connecting here brings {{ requires.name }} up first if it is down, and disconnecting it
            closes this one too.
          </p>
        </div>
        <ConnectionStatus :state="requiresState" />
      </div>

      <dl v-if="state === 'connected'" class="mt-6 grid gap-4 border-t border-line pt-5 sm:grid-cols-4">
        <div>
          <dt class="text-xs text-ink-faint">Account</dt>
          <dd class="mt-1 text-sm text-ink">
            {{
              connection.managesOwnAccount
                ? (session?.identity || 'This machine')
                : (activeAccount?.label ?? 'Signed in via vendor client')
            }}
          </dd>
          <dd
            v-if="!connection.managesOwnAccount && activeAccount?.username"
            class="font-mono text-xs text-ink-faint"
          >
            {{ activeAccount.username }}
          </dd>
        </div>
        <div>
          <dt class="text-xs text-ink-faint">{{ isSSH ? 'Listening on' : 'Assigned IP' }}</dt>
          <dd class="mt-1 font-mono text-sm text-ink">{{ session?.assignedIp || '—' }}</dd>
        </div>
        <div>
          <dt class="text-xs text-ink-faint">Interface</dt>
          <dd class="mt-1 font-mono text-sm text-ink">{{ session?.interface || '—' }}</dd>
        </div>
        <div>
          <dt class="text-xs text-ink-faint">Duration</dt>
          <dd class="mt-1 font-mono text-sm text-ink">
            {{ formatDuration(sessions.durationFor(id)) }}
          </dd>
        </div>
      </dl>

      <div v-if="state === 'connected' && forwards.length" class="mt-5 border-t border-line pt-5">
        <p class="mb-2 text-xs font-medium text-ink-faint">Forwarded ports</p>
        <ForwardList :forwards="forwards" :server="connection.profile.host" />

        <!-- Editing a live tunnel is allowed, so what was saved can differ from
             what is bound. Saying so is the difference between a puzzling list
             and one the engineer knows to expect. -->
        <p
          v-if="forwardsPending"
          class="mt-2 rounded-xl border border-brand/30 bg-brand/10 px-3 py-2 text-xs leading-relaxed text-ink"
        >
          The ports above are the ones this tunnel opened when it connected. The connection has
          been edited since — reconnect to apply the change.
        </p>
      </div>

      <div
        v-if="state === 'connected' && diagnosis"
        class="mt-5 rounded-xl border px-4 py-3"
        :class="
          diagnosis.capturesAllTraffic
            ? 'border-warn/40 bg-warn/10'
            : 'border-ok/30 bg-ok/10'
        "
      >
        <p
          class="text-sm font-semibold"
          :class="diagnosis.capturesAllTraffic ? 'text-warn' : 'text-ok'"
        >
          {{ diagnosis.summary }}
        </p>
        <p class="mt-1 text-xs leading-relaxed text-ink-muted">{{ diagnosis.detail }}</p>
      </div>




    </section>

    <!-- Accounts -->
    <section v-if="!connection.managesOwnAccount" v-show="section === 'accounts'" aria-label="Saved accounts" class="rounded-2xl border border-line bg-surface-1">
      <header class="flex flex-wrap items-center justify-between gap-3 border-b border-line px-6 py-4">
        <div>
          <h2 class="text-base font-semibold text-ink">Accounts</h2>
          <p class="mt-0.5 text-xs text-ink-muted">
            {{ isSSH ? 'Manage passwords and keys for this tunnel. The SSH username is set in Settings.' : 'Manage login identities and passwords. Choose a default account for this connection.' }}
          </p>
        </div>
        <button
          type="button"
          class="no-drag rounded-xl border border-line-strong px-3.5 py-1.5 text-sm font-medium text-ink transition hover:border-brand/60 hover:text-brand"
          @click="openCredentialForm(null)"
        >
          + Add account
        </button>
      </header>

      <ul v-if="connection.credentials.length" class="divide-y divide-line">
        <li
          v-for="credential in connection.credentials"
          :key="credential.id"
          class="flex flex-wrap items-center gap-4 px-6 py-4"
        >
          <div class="min-w-0 flex-1">
            <p class="truncate font-medium text-ink">{{ credential.label }} <span v-if="credential.isDefault" class="ml-2 rounded bg-brand/10 px-2 py-0.5 text-xs font-normal text-brand">Default</span></p>
            <p class="mt-0.5 flex flex-wrap items-center gap-2 text-xs text-ink-muted">
              <template v-if="!isSSH"><span class="font-mono">{{ credential.username || 'no username' }}</span><span aria-hidden="true">&middot;</span></template>
              <span :class="credential.authMethod !== 'password' || credential.hasPassword ? 'text-ok' : 'text-warn'">
                {{ credential.authMethod === 'password' ? (credential.hasPassword ? 'Password saved in Keychain' : 'No password saved') : credential.authMethod === 'ssh_key' ? (credential.hasPassword ? 'SSH key · passphrase saved in Keychain' : 'SSH key') : credential.authMethod }}
              </span>
              <template v-if="credential.hasPassword">
                <span aria-hidden="true">&middot;</span>
                <button
                  type="button"
                  class="no-drag inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-[11px] text-ink-faint transition hover:bg-surface-2 hover:text-ink"
                  :title="`Keychain account reference (Service: Tobiro) — click to copy ${secretRefOf(credential)}`"
                  @click="copySecretRef(credential)"
                >
                  <span class="truncate">{{ secretRefOf(credential) }}</span>
                  <span
                    class="font-sans text-[10px] font-medium"
                    :class="copiedSecretRef === credential.id ? 'text-ok' : 'text-ink-faint'"
                  >
                    {{ copiedSecretRef === credential.id ? 'Copied' : 'Copy' }}
                  </span>
                </button>
              </template>
            </p>
          </div>

          <div class="flex items-center gap-1.5">
            <button
              v-if="!credential.isDefault"
              type="button"
              class="no-drag rounded-lg px-2.5 py-1.5 text-xs font-medium text-ink-muted transition hover:bg-ink/5 hover:text-ink"
              @click="store.makeDefaultCredential(credential.id)"
            >
              Make default
            </button>
            <button
              type="button"
              class="no-drag rounded-lg px-2.5 py-1.5 text-xs font-medium text-ink-muted transition hover:bg-ink/5 hover:text-ink"
              @click="openCredentialForm(credential)"
            >
              Edit
            </button>
            <button
              type="button"
              class="no-drag rounded-lg px-2.5 py-1.5 text-xs font-medium text-bad border border-bad/30 bg-bad/10 transition hover:border-bad/60 hover:bg-bad/20 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-bad"
              @click="removeCredential(credential)"
            >
              Delete
            </button>
          </div>
        </li>
      </ul>

      <!-- A tunnel with no account is not stuck: it falls back to the agent,
           which is how most engineers already reach these servers. -->
      <p v-else class="px-6 py-10 text-center text-sm text-ink-muted">
        {{
          isSSH
            ? 'No saved accounts. SSH can use your agent. Add an account to use a password or a specific key.'
            : 'No accounts yet. Add one to connect.'
        }}
      </p>
    </section>

    <div v-show="section === 'settings'" class="space-y-5" aria-label="Connection settings">
      <section class="flex flex-wrap items-center justify-between gap-4 rounded-2xl border border-line bg-surface-1 p-6">
        <div>
          <h2 class="text-base font-semibold text-ink">Connection details</h2>
          <p class="mt-1 text-sm text-ink-muted">{{ isSSH ? 'Name, group, notes and the connection required first.' : 'Name, group, VPN gateway and client preferences.' }}</p>
        </div>
        <button type="button" class="no-drag rounded-lg border border-line-strong px-4 py-2 text-sm font-medium text-ink hover:border-brand" @click="showEditForm = true">Edit details</button>
      </section>
      <TunnelSettings
        v-if="isSSH"
        :key="id"
        ref="tunnelSettings"
        :connection="connection"
        @save="saveTunnel"
      />
    <!-- Client detection. A tunnel is implemented in this application, so there
         is no installed client to have found or missed. -->
    <section v-if="!isSSH" class="rounded-2xl border border-line bg-surface-1 p-6">
      <h2 class="text-base font-semibold text-ink">Installed client</h2>
      <p v-if="capabilities?.notes" class="mt-2 text-xs leading-relaxed text-ink-muted">{{ capabilities.notes }}</p>
      <dl class="mt-4 grid gap-4 sm:grid-cols-3">
        <div>
          <dt class="text-xs text-ink-faint">Status</dt>
          <dd class="mt-1 text-sm" :class="connection.client.installed ? 'text-ok' : 'text-warn'">
            {{ connection.client.installed ? 'Detected' : 'Not found' }}
          </dd>
        </div>
        <div>
          <dt class="text-xs text-ink-faint">Version</dt>
          <dd class="mt-1 font-mono text-sm text-ink">{{ connection.client.version || '—' }}</dd>
        </div>
        <div>
          <dt class="text-xs text-ink-faint">Control</dt>
          <dd class="mt-1 text-sm text-ink">
            {{ capabilities?.canConnect ? 'Command line' : 'Native client' }}
          </dd>
        </div>
      </dl>
      <p v-if="connection.client.path" class="mt-4 truncate font-mono text-xs text-ink-faint">
        {{ connection.client.path }}
      </p>

      <div
        v-if="capabilities?.canControlStartup && startup"
        class="mt-5 flex items-center justify-between gap-4 rounded-xl border border-line bg-surface-2 px-4 py-3"
      >
        <div class="min-w-0">
          <p class="text-sm font-medium text-ink">Open at login</p>
          <p class="mt-0.5 text-xs leading-relaxed text-ink-muted">{{ startup.detail }}</p>
        </div>
        <button
          type="button"
          role="switch"
          :aria-checked="startup.enabled"
          :disabled="startupBusy"
          class="no-drag relative h-6 w-11 shrink-0 rounded-full transition disabled:opacity-50"
          :class="startup.enabled ? 'bg-brand' : 'bg-line-strong'"
          @click="toggleStartup(!startup.enabled)"
        >
          <span
            class="absolute top-0.5 left-0.5 h-5 w-5 rounded-full bg-white shadow transition"
            :class="startup.enabled ? 'translate-x-5' : 'translate-x-0'"
          />
        </button>
      </div>
    </section>

    <!-- Danger zone -->
    <section class="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-line bg-surface-1 px-6 py-4">
      <div>
        <h2 class="text-sm font-semibold text-ink">Delete connection</h2>
        <p class="mt-0.5 text-xs text-ink-muted">
          Deleting removes every account and its stored password.
        </p>
      </div>
      <button
        type="button"
        class="no-drag rounded-xl px-4 py-2 text-sm font-medium text-bad border border-bad/30 bg-bad/10 transition hover:border-bad/60 hover:bg-bad/20 disabled:cursor-not-allowed disabled:opacity-40"
        :disabled="active"
        :title="active ? 'Disconnect before deleting this connection' : undefined"
        @click="showDeleteDialog = true"
      >
        Delete connection…
      </button>
    </section>

    </div>

    <p v-if="sessions.errorFor(id)" role="alert" class="rounded-xl bg-bad/10 px-4 py-3 text-sm text-bad">{{ sessions.errorFor(id) }}</p>
    <p v-if="pageError" class="rounded-xl bg-bad/10 px-4 py-3 text-sm text-bad">{{ pageError }}</p>

    <ConnectionForm
      v-if="showEditForm"
      ref="editForm"
      :connection="connection"
      :providers="store.bootstrap?.providers ?? []"
      :groups="store.groupNames"
      :connections="store.connections"
      @close="showEditForm = false"
      @save="saveConnection"
    />

    <CredentialForm
      v-if="showCredentialForm"
      ref="credentialForm"
      :credential="editingCredential"
      :auth-methods="store.bootstrap?.authMethods ?? []"
      :kind="connection.profile.kind || 'vpn'"
      @close="showCredentialForm = false"
      @save="saveCredential"
    />

    <ConnectDialog
      v-if="showConnectDialog"
      :connection="connection"
      @close="showConnectDialog = false"
      @connect="confirmConnect"
    />
    <ConnectDialog
      v-if="showFillDialog"
      mode="fill"
      :connection="connection"
      @close="showFillDialog = false"
      @connect="confirmFill"
    />

    <DisconnectChainDialog
      v-if="pendingDisconnect && connection"
      :parent-name="connection.profile.name"
      :dependents="activeDependents.map((c) => ({ name: c.profile.name }))"
      @close="pendingDisconnect = false"
      @confirm="confirmDisconnectAll"
    />

    <DeleteConnectionDialog
      v-if="showDeleteDialog && connection"
      :name="connection.profile.name"
      :dependents="deleteDependents()"
      @close="showDeleteDialog = false"
      @confirm="deleteConnection"
    />
  </div>
</template>
