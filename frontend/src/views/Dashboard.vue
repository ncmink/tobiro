<script setup lang="ts">
import { computed, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'

import type { domain, main } from '@bindings/go/models'
import logo from '@/assets/images/app-logo.png'
import ConnectDialog from '@/components/ConnectDialog.vue'
import ConnectionCard from '@/components/ConnectionCard.vue'
import ConnectionForm from '@/components/ConnectionForm.vue'
import ContextMenu from '@/components/ContextMenu.vue'
import DeleteConnectionDialog from '@/components/DeleteConnectionDialog.vue'
import DisconnectChainDialog from '@/components/DisconnectChainDialog.vue'
import OpenKubeDialog from '@/components/OpenKubeDialog.vue'
import SshConfigImportDialog from '@/components/SshConfigImportDialog.vue'
import type { ContextMenuItem } from '@/lib/menu'
import { useConnectionSharing } from '@/lib/connectionSharing'
import { connectionId, connectionParentId, descendants, findNode, forest } from '@/lib/chain'
import { accountToConnectWith, canConnect, errorMessage, isActive } from '@/lib/state'
import { useConnectionsStore } from '@/stores/connections'
import { useSessionsStore } from '@/stores/sessions'

const router = useRouter()
const store = useConnectionsStore()
const sessions = useSessionsStore()
const { sharing, exportConnections, importConnections } = useConnectionSharing()

const search = ref('')
const kindFilter = ref<'all' | 'vpn' | 'ssh'>('all')
const filterOptions = computed(() => [
  { id: 'all' as const, label: 'All connections', count: store.visibleConnections.length },
  { id: 'vpn' as const, label: 'VPN clients', count: store.visibleConnections.filter(c => c.profile.kind !== 'ssh').length },
  { id: 'ssh' as const, label: 'SSH tunnels', count: store.visibleConnections.filter(c => c.profile.kind === 'ssh').length },
])
function clearFilters() {
  search.value = ''
  kindFilter.value = 'all'
}
/** Which kind the add form is filling in, or null while it is closed. */
const addKind = ref<string | null>(null)
/** Where the "what kind" menu hangs, under whichever button opened it. */
const addMenu = ref<{ x: number; y: number } | null>(null)
const addForm = ref<InstanceType<typeof ConnectionForm> | null>(null)
const editTarget = ref<main.ConnectionView | null>(null)
const editForm = ref<InstanceType<typeof ConnectionForm> | null>(null)
const connectTarget = ref<main.ConnectionView | null>(null)
const fillTarget = ref<main.ConnectionView | null>(null)
const menu = ref<{ x: number; y: number; connection: main.ConnectionView } | null>(null)
const groupError = ref('')
/** Where the last connection went, when it went somewhere out of sight. */
const moved = ref('')
/** Parent the user asked to drop, waiting on a confirm because dependents are up. */
const pendingDisconnect = ref<main.ConnectionView | null>(null)
const pendingDelete = ref<main.ConnectionView | null>(null)
const openKubeTarget = ref<main.ConnectionView | null>(null)
const sshConfigTarget = ref<main.ConnectionView | null>(null)

const filtered = computed(() => {
  const query = search.value.trim().toLowerCase()
  const candidates = store.visibleConnections.filter(c =>
    kindFilter.value === 'all' || (c.profile.kind === 'ssh' ? 'ssh' : 'vpn') === kindFilter.value,
  )
  if (!query) return candidates
  return candidates.filter(
    (c) =>
      c.profile.name.toLowerCase().includes(query) ||
      c.profile.host.toLowerCase().includes(query) ||
      c.profile.group.toLowerCase().includes(query) ||
      c.providerName.toLowerCase().includes(query),
  )
})

/**
 * One card per root. Children are nested on that card via `requires`, not
 * listed again as siblings.
 */
const trees = computed(() => forest(filtered.value, connectionId, connectionParentId))

/** Whole catalogue, so a disconnect confirm names hidden dependents too. */
const catalogueForest = computed(() =>
  forest(store.connections, connectionId, connectionParentId),
)

function dependentsOf(connection: main.ConnectionView) {
  return store.connections.filter((c) => c.profile.requires === connection.profile.id)
}

function isLive(connection: main.ConnectionView) {
  return isActive(sessions.sessionFor(connection.profile.id)?.state ?? connection.state)
}

function activeDependentsOf(connection: main.ConnectionView) {
  const node = findNode(catalogueForest.value, connection.profile.id, connectionId)
  if (!node) return []
  return descendants(node).filter(isLive)
}

/** One section per group, in the order the backend reports groups. */
const sections = computed(() =>
  store.groups
    .filter((group) => store.showHidden || !group.hidden)
    .map((group) => ({
      ...group,
      // The ungrouped connections are not a group, so that section is shown
      // without the control that would put it away.
      hideable: group.name !== '',
      trees: trees.value.filter((node) => node.item.profile.group === group.name),
    }))
    .filter((section) => section.trees.length > 0),
)

/**
 * Until something is actually grouped, headers would just say "Ungrouped".
 * The archive is always offered as a destination but stays out of this test
 * while it is empty, so an untouched list keeps its flat shape.
 */
const grouping = computed(() =>
  store.groups.some((group) => group.name !== '' && group.connections > 0),
)

const connectedCount = computed(
  () =>
    store.visibleConnections.filter((c) => {
      return (sessions.sessionFor(c.profile.id)?.state ?? c.state) === 'connected'
    }).length,
)

/** Everything is filed under a hidden group, so the list looks empty. */
const allHidden = computed(
  () => !store.isEmpty && store.visibleConnections.length === 0 && store.hiddenCount > 0,
)

async function setGroupHidden(name: string, hidden: boolean) {
  groupError.value = ''
  try {
    await store.setGroupHidden(name, hidden)
  } catch (err) {
    groupError.value = errorMessage(err, 'Could not change the group')
  }
}

/**
 * Asks how a new connection reaches the network before the form opens.
 *
 * A vendor client and a tunnel share nothing but a name and a group, so the
 * answer decides what the form asks for rather than sitting inside it as a
 * switch that rewrites the fields under the engineer's hands.
 */
function openAddMenu(event: MouseEvent) {
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  addMenu.value = { x: rect.left, y: rect.bottom + 6 }
}

const addItems: ContextMenuItem[] = [
  { id: 'vpn', label: 'VPN client' },
  { id: 'ssh', label: 'SSH tunnel' },
]

function startAdd(kind: string) {
  addMenu.value = null
  addKind.value = kind
}

async function createConnection(input: domain.ProfileInput) {
  try {
    const created = await store.createConnection(input)
    addKind.value = null
    // A tunnel is not finished at this point: its server and port map are set
    // up on its own page, so that is where the next step is.
    if (created.profile.kind === 'ssh') {
      await router.push({ name: 'connection', params: { id: created.profile.id } })
    }
  } catch (err) {
    addForm.value?.fail(errorMessage(err, 'Could not save the connection'))
  }
}

async function saveConnection(input: domain.ProfileInput) {
  const target = editTarget.value
  if (!target) return
  try {
    await store.updateConnection(target.profile.id, input)
    editTarget.value = null
  } catch (err) {
    editForm.value?.fail(errorMessage(err, 'Could not save the connection'))
  }
}

/**
 * Moving into a hidden group makes the card disappear on purpose, so say where
 * it went instead of leaving the engineer wondering what just happened.
 */
async function moveConnection(connection: main.ConnectionView, group: string) {
  groupError.value = ''
  moved.value = ''
  try {
    await store.moveConnection(connection.profile.id, group)
    const destination = store.groups.find((g) => g.name === group)
    if (destination?.hidden && !store.showHidden) {
      moved.value = `“${connection.profile.name}” moved to ${destination.label}, which is hidden.`
    }
  } catch (err) {
    groupError.value = errorMessage(err, 'Could not move the connection')
  }
}

function openMenu(connection: main.ConnectionView, event: MouseEvent) {
  menu.value = { x: event.clientX, y: event.clientY, connection }
}

/**
 * The ⋯ menu offers what the card offers, plus the actions that otherwise
 * need a trip to the connection page.
 */
const menuItems = computed<ContextMenuItem[]>(() => {
  const target = menu.value?.connection
  if (!target) return []

  const state = sessions.stateFor(target.profile.id) || target.state || 'disconnected'
  const active = isActive(state)
  const capabilities = target.capabilities
  const connectable = canConnect(target)

  const items: ContextMenuItem[] = []
  if (active && capabilities.canDisconnect) {
    items.push({
      id: 'disconnect',
      label: activeDependentsOf(target).length > 0 ? 'Disconnect all' : 'Disconnect',
    })
  } else if (!active && connectable) {
    items.push({ id: 'connect', label: 'Connect' })
  }
  if (capabilities.canOpenNativeClient) {
    items.push({ id: 'native', label: capabilities.nativeClientLabel || 'Open native client' })
  }
  if (active && target.profile.kind === 'ssh') {
    items.push({ id: 'kube', label: 'Open in Biebie Kube', divider: items.length > 0 })
  }

  items.push({ id: 'details', label: 'Open details', divider: items.length > 0 })
  items.push({ id: 'edit', label: 'Edit connection…' })
  if (target.profile.kind !== 'ssh') {
    items.push({ id: 'ssh-config', label: 'Import from SSH config…' })
  }

  // Filing a connection elsewhere is a one-click job here rather than a trip
  // through the edit form. Every group is a destination except the one the
  // connection is already in; the archive is always among them.
  const destinations = store.groups.filter(
    (group) => group.name !== '' && group.name !== target.profile.group,
  )
  destinations.forEach((group, index) => {
    items.push({
      id: `move:${group.name}`,
      label: group.hidden ? `Move to “${group.label}” (hidden)` : `Move to “${group.label}”`,
      divider: index === 0,
    })
  })
  if (target.profile.group) {
    items.push({ id: 'move:', label: 'Remove from group', divider: destinations.length === 0 })
  }

  if (target.profile.group) {
    items.push({
      id: 'group',
      label: target.hidden
        ? `Show “${target.profile.group}” in the list`
        : `Hide group “${target.profile.group}”`,
      divider: true,
    })
  }

  items.push({
    id: 'delete',
    label: active ? 'Delete connection… (disconnect first)' : 'Delete connection…',
    danger: true,
    disabled: active,
    divider: true,
  })
  return items
})

function runMenuAction(id: string) {
  const target = menu.value?.connection
  menu.value = null
  if (!target) return

  if (id.startsWith('move:')) {
    void moveConnection(target, id.slice('move:'.length))
    return
  }

  switch (id) {
    case 'connect':
      requestConnect(target)
      break
    case 'disconnect':
      requestDisconnect(target)
      break
    case 'native':
      requestOpenNative(target)
      break
    case 'kube':
      openKubeTarget.value = target
      break
    case 'details':
      void router.push({ name: 'connection', params: { id: target.profile.id } })
      break
    case 'edit':
      editTarget.value = target
      break
    case 'ssh-config':
      sshConfigTarget.value = target
      break
    case 'group':
      void setGroupHidden(target.profile.group, !target.hidden)
      break
    case 'delete':
      pendingDelete.value = target
      break
  }
}

async function confirmDelete() {
  const target = pendingDelete.value
  pendingDelete.value = null
  if (!target) return
  groupError.value = ''
  try {
    await store.removeConnection(target.profile.id)
  } catch (err) {
    groupError.value = errorMessage(err, 'Could not delete the connection')
  }
}

/** One obvious account connects immediately; a choice opens the picker. */
function requestConnect(connection: main.ConnectionView) {
  sessions.clearError(connection.profile.id)
  const account = accountToConnectWith(connection)
  if (account === null) {
    connectTarget.value = connection
    return
  }
  void sessions.connect(connection.profile.id, account)
}

/**
 * Child disconnect is just that child. Parent disconnect with live dependents
 * asks first; the backend already closes dependents before the parent.
 */
function requestDisconnect(connection: main.ConnectionView) {
  if (activeDependentsOf(connection).length > 0) {
    pendingDisconnect.value = connection
    return
  }
  void sessions.disconnect(connection.profile.id)
}

function confirmDisconnectAll() {
  const target = pendingDisconnect.value
  pendingDisconnect.value = null
  if (target) void sessions.disconnect(target.profile.id)
}

async function confirmConnect(credentialId: string) {
  const target = connectTarget.value
  connectTarget.value = null
  if (!target) return
  await sessions.connect(target.profile.id, credentialId)
  await store.refresh()
}

/** Open the vendor GUI, filling login fields when an account is chosen. */
function requestOpenNative(connection: main.ConnectionView) {
  sessions.clearError(connection.profile.id)
  if (connection.credentials.length === 0) {
    void sessions.openNativeClient(connection.profile.id, '')
    return
  }
  if (connection.credentials.length === 1) {
    void sessions.openNativeClient(connection.profile.id, connection.credentials[0].id)
    return
  }
  fillTarget.value = connection
}

async function confirmFill(credentialId: string) {
  const target = fillTarget.value
  fillTarget.value = null
  if (!target) return
  await sessions.openNativeClient(target.profile.id, credentialId)
}
</script>

<template>
  <div class="dashboard mx-auto w-full max-w-6xl space-y-6">
    <header class="flex flex-wrap items-center justify-between gap-4">
      <RouterLink
        :to="{ name: 'dashboard' }"
        class="no-drag flex min-w-0 items-center gap-4 rounded-xl transition hover:opacity-90"
      >
        <img :src="logo" alt="" class="h-28 w-28 shrink-0 object-contain sm:h-32 sm:w-32" />
        <div class="min-w-0 leading-tight">
          <p class="text-[10px] font-semibold uppercase tracking-[0.2em] text-brand">Your workspace</p>
          <h1 class="text-[28px] font-semibold tracking-tight text-ink">Connections</h1>
          <p class="mt-1 text-sm text-ink-muted">VPNs, SSH tunnels, one workspace.</p>
          <p class="mt-2 flex flex-wrap items-center gap-2 text-xs text-ink-muted">
            {{ store.visibleConnections.length }}
            {{ store.visibleConnections.length === 1 ? 'connection' : 'connections' }}
            &middot;
            <span :class="connectedCount ? 'text-ok' : ''">{{ connectedCount }} connected</span>
            <template v-if="store.hiddenCount && !store.showHidden">
              &middot;
              <span class="text-ink-faint">{{ store.hiddenCount }} hidden</span>
            </template>
            <span class="text-ink-faint">&middot; v{{ store.bootstrap?.version ?? '—' }}</span>
          </p>
        </div>
      </RouterLink>

      <div class="flex max-w-full flex-wrap items-center gap-2">
        <span
          v-if="sessions.anyConnected"
          class="inline-flex h-9 items-center gap-1.5 rounded-lg bg-ok/10 px-3 text-xs font-medium text-ok"
        >
          <span class="h-1.5 w-1.5 rounded-full bg-ok" />
          Tunnel active
        </span>

        <div class="flex items-center gap-0.5 rounded-lg border border-line bg-surface-0 p-1">
          <button
            type="button"
            :disabled="sharing"
            aria-label="Import connections"
            title="Import connections from a shared file"
            class="no-drag icon-action"
            @click="importConnections"
          >
            <svg fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 3v12m0 0-4-4m4 4 4-4M4 17v2a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-2" />
            </svg>
          </button>
          <button
            type="button"
            :disabled="sharing"
            aria-label="Export connections"
            title="Export connections to share (no passwords)"
            class="no-drag icon-action"
            @click="exportConnections"
          >
            <svg fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 15V3m0 0-4 4m4-4 4 4M4 17v2a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-2" />
            </svg>
          </button>
        </div>

        <RouterLink
          :to="{ name: 'settings' }"
          class="no-drag icon-action"
          aria-label="Settings"
          title="Settings"
        >
          <svg fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.7" stroke-linecap="round">
            <path d="M4 7h16M4 17h16" />
            <circle cx="9" cy="7" r="3" fill="var(--color-surface-3)" />
            <circle cx="15" cy="17" r="3" fill="var(--color-surface-3)" />
          </svg>
        </RouterLink>

        <button
          type="button"
          aria-label="Add connection"
          title="Add connection"
          class="no-drag inline-flex h-9 items-center gap-2 rounded-lg bg-brand px-3.5 shadow-sm text-sm font-semibold text-surface-1 transition hover:bg-brand-strong"
          @click="openAddMenu"
        >
          <svg class="h-4 w-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 5v14M5 12h14" />
          </svg>
          Add connection
        </button>
      </div>
    </header>

    <div class="dashboard-toolbar">
      <div class="flex flex-wrap items-center gap-1" role="group" aria-label="Filter connection type">
        <button v-for="option in filterOptions" :key="option.id" type="button"
          class="filter-button" :class="{ 'filter-button--active': kindFilter === option.id }"
          :aria-pressed="kindFilter === option.id" @click="kindFilter = option.id">
          {{ option.label }} <span class="filter-count">{{ option.count }}</span>
        </button>
      </div>
      <div class="flex max-w-full flex-wrap items-center gap-2">
        <button
          v-if="store.hiddenCount"
          type="button"
          class="no-drag inline-flex items-center gap-1.5 rounded-xl border border-line bg-surface-0 px-3 py-2 text-sm font-medium transition hover:border-line-strong"
          :class="store.showHidden ? 'text-ink' : 'text-ink-muted'"
          :title="store.showHidden ? 'Hide the hidden groups again' : 'Reveal the hidden groups'"
          @click="store.toggleShowHidden()"
        >
          <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.8">
            <template v-if="store.showHidden">
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M2.04 12.32a1.01 1.01 0 010-.64C3.42 7.51 7.36 4.5 12 4.5s8.57 3.01 9.96 7.18c.07.21.07.43 0 .64C20.58 16.49 16.64 19.5 12 19.5s-8.57-3.01-9.96-7.18z"
              />
              <path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
            </template>
            <path
              v-else
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M3.98 8.22A10.48 10.48 0 001.93 12C3.23 16.34 7.24 19.5 12 19.5c.99 0 1.95-.14 2.86-.4M6.23 6.23A10.45 10.45 0 0112 4.5c4.76 0 8.77 3.16 10.07 7.5a10.52 10.52 0 01-4.3 5.77M6.23 6.23L3 3m3.23 3.23l3.65 3.65m7.89 7.89L21 21m-3.23-3.23l-3.65-3.65m0 0a3 3 0 10-4.24-4.24"
            />
          </svg>
          {{ store.showHidden ? 'Hide' : 'Show' }} hidden ({{ store.hiddenCount }})
        </button>

        <label class="relative min-w-0 max-w-full">
          <svg aria-hidden="true" class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-faint" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="10.5" cy="10.5" r="6.5"/><path stroke-linecap="round" d="m16 16 4 4"/></svg>
        <input
          v-model="search"
          type="search"
          placeholder="Search connections…"
          aria-label="Search connections"
          class="no-drag min-w-0 w-64 max-w-full rounded-xl border border-line bg-surface-0 py-2.5 pr-3 pl-9 text-sm text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
        />
        </label>
      </div>
    </div>

    <p v-if="store.error" class="rounded-xl bg-bad/10 px-4 py-3 text-sm text-bad">
      {{ store.error }}
    </p>
    <p v-if="groupError" class="rounded-xl bg-bad/10 px-4 py-3 text-sm text-bad">{{ groupError }}</p>

    <p
      v-if="moved"
      class="flex flex-wrap items-center gap-3 rounded-xl border border-line bg-surface-1 px-4 py-3 text-sm text-ink-muted"
    >
      {{ moved }}
      <button
        type="button"
        class="no-drag rounded-lg border border-line px-2.5 py-1 text-xs font-medium text-ink transition hover:border-line-strong"
        @click="store.setShowHidden(true)"
      >
        Show hidden groups
      </button>
      <button
        type="button"
        class="no-drag rounded-lg px-2 py-1 text-xs text-ink-faint transition hover:text-ink"
        @click="moved = ''"
      >
        Dismiss
      </button>
    </p>

    <div
      v-if="!store.loaded && store.loading"
      class="rounded-2xl border border-line bg-surface-1 px-6 py-16 text-center text-sm text-ink-muted"
      role="status"
    >Loading connections…</div>

    <div
      v-else-if="store.isEmpty"
      class="rounded-2xl border border-dashed border-line-strong bg-surface-1 px-6 py-16 text-center"
    >
      <div class="mx-auto mb-5 grid h-16 w-16 place-items-center rounded-2xl border border-brand/20 bg-brand/10 text-brand">
        <svg aria-hidden="true" class="h-8 w-8" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="3" y="3" width="18" height="13" rx="3"/><path stroke-linecap="round" d="M8 21h8m-4-5v5M8 9h8m-4-3v6"/></svg>
      </div>
      <h2 class="text-lg font-semibold text-ink">Your next connection starts here</h2>
      <p class="mx-auto mt-2 max-w-md text-sm text-ink-muted">
        Add a VPN or SSH tunnel, then keep its accounts and connection status together here.
      </p>
      <button
        type="button"
        class="no-drag mt-6 rounded-xl bg-brand px-5 py-2.5 text-sm font-semibold text-surface-1 transition hover:bg-brand-strong"
        @click="openAddMenu"
      >
        Add your first connection
      </button>
    </div>

    <div
      v-else-if="allHidden"
      class="rounded-2xl border border-dashed border-line-strong bg-surface-1 px-6 py-14 text-center"
    >
      <h2 class="text-lg font-semibold text-ink">Every connection is in a hidden group</h2>
      <p class="mx-auto mt-2 max-w-md text-sm text-ink-muted">
        Nothing was deleted — the groups you work with are just out of the way right now.
      </p>
      <button
        type="button"
        class="no-drag mt-6 rounded-xl bg-brand px-5 py-2.5 text-sm font-semibold text-surface-1 transition hover:bg-brand-strong"
        @click="store.setShowHidden(true)"
      >
        Show {{ store.hiddenCount }} hidden
      </button>
    </div>

    <!-- Nothing grouped yet: one flat list, no headers to read past. -->
    <div v-else-if="!grouping" class="grid items-start gap-4 lg:grid-cols-2">
      <ConnectionCard
        v-for="node in trees"
        :key="node.item.profile.id"
        :connection="node.item"
        :node="node"
        @connect="requestConnect"
        @disconnect="requestDisconnect"
        @open-native="requestOpenNative"
        @menu="openMenu"
      />
    </div>

    <div v-else class="space-y-8">
      <section v-for="section in sections" :key="section.name">
        <header class="flex items-center gap-3">
          <h2
            class="text-xs font-semibold tracking-[0.12em] uppercase"
            :class="section.hidden ? 'text-ink-faint' : 'text-ink-muted'"
          >
            {{ section.label }}
          </h2>
          <span class="rounded-full bg-surface-2 px-2 py-0.5 text-xs text-ink-faint">
            {{ section.trees.length }}
          </span>
          <span
            v-if="section.hidden"
            class="rounded-full border border-line px-2 py-0.5 text-xs text-ink-faint"
          >
            Hidden
          </span>
          <span class="h-px flex-1 bg-line" />
          <button
            v-if="section.hideable"
            type="button"
            class="no-drag rounded-lg px-2.5 py-1 text-xs font-medium text-ink-faint transition hover:bg-ink/5 hover:text-ink"
            :title="
              section.hidden
                ? 'Show this group in the connection list'
                : 'Keep this group out of the connection list'
            "
            @click="setGroupHidden(section.name, !section.hidden)"
          >
            {{ section.hidden ? 'Show in list' : 'Hide group' }}
          </button>
        </header>

        <div class="mt-3 grid items-start gap-4 lg:grid-cols-2">
          <ConnectionCard
            v-for="node in section.trees"
            :key="node.item.profile.id"
            :connection="node.item"
            :node="node"
            @connect="requestConnect"
            @disconnect="requestDisconnect"
            @open-native="requestOpenNative"
            @menu="openMenu"
          />
        </div>
      </section>
    </div>

    <div
      v-if="store.loaded && !store.isEmpty && !allHidden && filtered.length === 0"
      class="rounded-2xl border border-dashed border-line-strong bg-surface-1 px-6 py-12 text-center"
      role="status"
    >
      <h2 class="text-base font-semibold text-ink">No matching connections</h2>
      <p class="mt-2 text-sm text-ink-muted">{{ search.trim() ? `Nothing matches “${search}” in this view.` : 'There are no connections of this type in this view.' }}</p>
      <button type="button" class="mt-4 rounded-lg px-3 py-2 text-sm font-medium text-brand hover:bg-brand/10" @click="clearFilters">Clear filters</button>
    </div>

    <ContextMenu
      v-if="menu"
      :x="menu.x"
      :y="menu.y"
      :items="menuItems"
      @close="menu = null"
      @select="runMenuAction"
    />

    <ContextMenu
      v-if="addMenu"
      :x="addMenu.x"
      :y="addMenu.y"
      :items="addItems"
      @close="addMenu = null"
      @select="startAdd"
    />

    <ConnectionForm
      v-if="addKind"
      ref="addForm"
      :kind="addKind"
      :providers="store.bootstrap?.providers ?? []"
      :groups="store.groupNames"
      :connections="store.connections"
      @close="addKind = null"
      @save="createConnection"
    />

    <ConnectionForm
      v-if="editTarget"
      ref="editForm"
      :key="editTarget.profile.id"
      :connection="editTarget"
      :providers="store.bootstrap?.providers ?? []"
      :groups="store.groupNames"
      :connections="store.connections"
      @close="editTarget = null"
      @save="saveConnection"
    />

    <ConnectDialog
      v-if="connectTarget"
      :connection="connectTarget"
      @close="connectTarget = null"
      @connect="confirmConnect"
    />
    <ConnectDialog
      v-if="fillTarget"
      mode="fill"
      :connection="fillTarget"
      @close="fillTarget = null"
      @connect="confirmFill"
    />

    <DisconnectChainDialog
      v-if="pendingDisconnect"
      :parent-name="pendingDisconnect.profile.name"
      :dependents="activeDependentsOf(pendingDisconnect).map((c) => ({ name: c.profile.name }))"
      @close="pendingDisconnect = null"
      @confirm="confirmDisconnectAll"
    />

    <DeleteConnectionDialog
      v-if="pendingDelete"
      :name="pendingDelete.profile.name"
      :dependents="dependentsOf(pendingDelete).map((c) => ({ name: c.profile.name }))"
      @close="pendingDelete = null"
      @confirm="confirmDelete"
    />

    <OpenKubeDialog
      v-if="openKubeTarget"
      :connection="openKubeTarget"
      @close="openKubeTarget = null"
      @done="(text) => (moved = text)"
    />

    <SshConfigImportDialog
      v-if="sshConfigTarget"
      :parent="sshConfigTarget"
      @close="sshConfigTarget = null"
      @done="(text) => (moved = text)"
    />
  </div>
</template>

<style scoped>
@reference '../style.css';
.dashboard-toolbar { @apply flex flex-wrap items-center justify-between gap-3 border-y border-line py-3; }
.filter-button { @apply inline-flex min-h-9 items-center gap-2 rounded-lg px-3 text-xs font-medium text-ink-muted transition hover:bg-surface-2 hover:text-ink; }
.filter-button--active { @apply bg-brand/10 text-brand; }
.filter-count { @apply rounded-md bg-ink/5 px-1.5 py-0.5 text-[10px] tabular-nums; }
.filter-button--active .filter-count { @apply bg-brand/10; }
</style>
