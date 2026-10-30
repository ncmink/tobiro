<script setup lang="ts">
import { computed, watch } from 'vue'
import { RouterLink } from 'vue-router'

import type { main } from '@bindings/go/models'
import ConnectionStatus from '@/components/ConnectionStatus.vue'
import ForwardList from '@/components/ForwardList.vue'
import ProviderIcon from '@/components/ProviderIcon.vue'
import { descendants, type ChainNode } from '@/lib/chain'
import { canConnect, formatDuration, isActive, stateStyle } from '@/lib/state'
import { canFillNativeLogin } from '@/lib/nativeFill'
import { useConnectionsStore } from '@/stores/connections'
import { useSessionsStore } from '@/stores/sessions'

defineOptions({ name: 'ConnectionCard' })

const props = withDefaults(
  defineProps<{
    connection: main.ConnectionView
    /** Nested under a parent card; the hierarchy already says what this sits behind. */
    nested?: boolean
    node?: ChainNode<main.ConnectionView>
  }>(),
  { nested: false },
)

const emit = defineEmits<{
  connect: [connection: main.ConnectionView]
  disconnect: [connection: main.ConnectionView]
  openNative: [connection: main.ConnectionView]
  menu: [connection: main.ConnectionView, event: MouseEvent]
}>()

const connectionsStore = useConnectionsStore()
const sessions = useSessionsStore()

const profileId = computed(() => props.connection.profile.id)
const state = computed(() => {
  return sessions.sessionFor(profileId.value)?.state ?? props.connection.state ?? 'disconnected'
})
const session = computed(() => sessions.sessionFor(profileId.value) ?? props.connection.session)
const busy = computed(() => sessions.isBusy(profileId.value) || stateStyle(state.value).busy)
const active = computed(() => isActive(state.value))
const error = computed(() => sessions.errorFor(profileId.value))

const endpoint = computed(() => props.connection.endpoint)

const isSSH = computed(() => props.connection.profile.kind === 'ssh')
const forwards = computed(() => session.value?.forwards ?? [])

/**
 * The connection this one is reached through, if any.
 *
 * Nested cards omit this: they already sit under that parent. A root whose
 * parent is not in the list still names it, so a search that hides the VPN
 * does not make the tunnel look independent.
 */
const requires = computed(() => props.connection.requires ?? null)
const missingParent = computed(() => props.node?.missingParent === true)
const showThrough = computed(() => !props.nested && missingParent.value && Boolean(requires.value?.name))

const childNodes = computed(() => props.node?.children ?? [])
const childHeading = computed(() => {
  if (childNodes.value.length === 0) return ''
  const allSSH = childNodes.value.every((child) => child.item.profile.kind === 'ssh')
  return allSSH ? 'SSH tunnels' : 'Dependents'
})
const childrenCollapsed = computed(() => connectionsStore.isChildrenCollapsed(profileId.value))
const childErrorsCount = computed(() =>
  descendants(props.node ?? { item: props.connection, children: [], missingParent: false, cyclic: false }).filter(
    (child) => Boolean(sessions.errorFor(child.profile.id)),
  ).length,
)

const activeDependents = computed(() =>
  descendants(props.node ?? { item: props.connection, children: [], missingParent: false, cyclic: false }).filter(
    (child) => isActive(sessions.sessionFor(child.profile.id)?.state ?? child.state),
  ),
)

const disconnectLabel = computed(() => {
  if (props.nested) return 'Disconnect'
  if (activeDependents.value.length > 0) return 'Disconnect all'
  return 'Disconnect'
})

watch(
  () => props.node?.cyclic === true,
  (cyclic) => {
    if (cyclic) {
      console.warn(
        `connection ${props.connection.profile.id} is in a requires-cycle; showing it as a standalone card`,
      )
    }
  },
  { immediate: true },
)

const accountLabel = computed(() => {
  if (props.connection.managesOwnAccount) {
    return session.value?.identity || 'This machine'
  }
  const id = session.value?.credentialId
  if (!id) return 'Signed in via vendor client'
  return props.connection.credentials.find((c) => c.id === id)?.label ?? 'Unknown account'
})

const capabilities = computed(() => props.connection.capabilities)
const hasAccounts = computed(() => props.connection.credentials.length > 0)
const needsAccount = computed(
  () => capabilities.value.supportsPassword !== false && !capabilities.value.accountOptional,
)

/** Only offer Connect when the adapter says it can actually drive the client. */
const connectable = computed(() => canConnect(props.connection))

// What is in the way, most fundamental first. A tunnel with no server yet
// cannot be helped by adding an account to it, so whatever the connection
// itself says is missing is said before anything about accounts.
const blockedReason = computed(() => {
  if (!capabilities.value.installed) {
    return `${props.connection.providerName} is not installed on this machine.`
  }
  if (!capabilities.value.canConnect) return capabilities.value.notes
  if (needsAccount.value && !hasAccounts.value) {
    return isSSH.value
      ? 'Save a password on the tunnel to connect.'
      : 'Add an account before connecting.'
  }
  return ''
})
const nativeLabel = computed(() => capabilities.value.nativeClientLabel || (
  props.connection.profile.provider === 'forcepoint' ? 'Open Forcepoint' : 'Open client'
))
const manualClient = computed(() =>
  capabilities.value.installed && capabilities.value.canOpenNativeClient && !capabilities.value.canConnect,
)
const manualHint = computed(() => {
  if (!manualClient.value) return ''
  if (canFillNativeLogin(props.connection)) {
    if (props.connection.profile.provider === 'forcepoint') {
      return 'Open Forcepoint, click Connect, click the login field, then press ⌘I — Tobiro fills username and password.'
    }
    return 'Open the client login window, then press ⌘I — Tobiro fills username and password.'
  }
  if (props.connection.profile.provider === 'forcepoint') {
    return 'Open Forcepoint, then click Connect in its window.'
  }
  return 'Finish connecting in the client window.'
})
const connectionHint = computed(() => (manualClient.value ? '' : blockedReason.value))

function openMenu(event: MouseEvent) {
  const bounds = (event.currentTarget as HTMLElement).getBoundingClientRect()
  emit('menu', props.connection, new MouseEvent('contextmenu', {
    clientX: bounds.right,
    clientY: bounds.bottom,
  }))
}

</script>

<template>
  <article
    class="connection-card"
    :class="[
      nested ? 'connection-card--nested' : 'connection-card--root',
      { 'connection-card--connected': state === 'connected', 'connection-card--hidden': connection.hidden },
    ]"
  >
    <div class="connection-row" :class="{ 'connection-row--iconless': isSSH }">
      <ProviderIcon
        v-if="!isSSH"
        :provider="connection.profile.provider"
        :size="nested ? 'sm' : 'md'"
      />

      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-2">
          <RouterLink
            :to="{ name: 'connection', params: { id: profileId } }"
            class="no-drag truncate rounded-md font-semibold text-ink underline-offset-4 hover:text-brand hover:underline focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-brand"
            :class="nested ? 'text-sm' : 'text-[15px] leading-snug'"
          >
            {{ connection.profile.name }}
          </RouterLink>
          <ConnectionStatus
            :state="state"
            compact
            :duration="nested && active ? formatDuration(sessions.durationFor(profileId)) : ''"
          />
        </div>
        <div class="mt-1 flex items-center gap-1.5 text-xs text-ink-muted">
          <span v-if="!nested" class="shrink-0">{{ isSSH ? 'SSH tunnel' : connection.providerName }}</span>
          <span v-if="!nested && endpoint" class="text-ink-faint" aria-hidden="true">&middot;</span>
          <span v-if="endpoint" :title="endpoint" class="truncate font-mono text-[11px] text-ink-faint">{{ endpoint }}</span>
        </div>
        <p v-if="node?.cyclic" class="mt-1 text-xs text-warn">
          Dependency cycle — shown as a standalone connection.
        </p>
        <p v-if="showThrough" class="mt-1 text-xs text-ink-muted">Via {{ requires?.name }}</p>
        <p v-else-if="!nested && missingParent" class="mt-1 text-xs text-warn">Parent connection unavailable</p>
      </div>

      <div class="connection-actions">
        <button
          v-if="active && capabilities.canDisconnect"
          type="button"
          class="connection-button connection-button--secondary hover:text-bad"
          :disabled="sessions.isBusy(profileId) || state === 'disconnecting'"
          @click="emit('disconnect', connection)"
        >
          {{ state === 'disconnecting' ? 'Disconnecting…' : disconnectLabel }}
        </button>
        <button
          v-else-if="!active && connectable"
          type="button"
          class="connection-button connection-button--primary"
          :disabled="busy"
          @click="emit('connect', connection)"
        >
          {{ busy ? 'Connecting…' : 'Connect' }}
        </button>
        <button
          v-else-if="capabilities.canOpenNativeClient"
          type="button"
          class="connection-button"
          :class="manualClient && !active ? 'connection-button--primary' : 'connection-button--secondary'"
          :title="manualHint || undefined"
          :disabled="busy"
          @click="emit('openNative', connection)"
        >
          {{ nativeLabel }}
        </button>
        <button
          type="button"
          class="connection-menu no-drag"
          :aria-label="`More options for ${connection.profile.name}`"
          :title="`More options for ${connection.profile.name}`"
          aria-haspopup="menu"
          @click="openMenu"
        >
          <svg aria-hidden="true" class="h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
            <circle cx="5" cy="12" r="1.8" /><circle cx="12" cy="12" r="1.8" /><circle cx="19" cy="12" r="1.8" />
          </svg>
        </button>
      </div>
    </div>

    <div v-if="connectionHint && !active" class="connection-help">
      <p class="truncate text-[11px] text-ink-faint" :title="connectionHint">{{ connectionHint }}</p>
      <RouterLink
        v-if="!manualClient && needsAccount && !hasAccounts && capabilities.installed && capabilities.canConnect"
        :to="{ name: 'connection', params: { id: profileId } }"
        class="shrink-0 text-[11px] font-semibold text-brand hover:underline"
      >
        Set up account →
      </RouterLink>
    </div>

    <dl v-if="state === 'connected' && !nested" class="connection-facts">
      <div><dt>Account</dt><dd :title="accountLabel">{{ accountLabel }}</dd></div>
      <div><dt>{{ isSSH ? 'Listening on' : 'Assigned IP' }}</dt><dd class="font-mono">{{ session?.assignedIp || '—' }}</dd></div>
      <div><dt>Duration</dt><dd class="font-mono">{{ formatDuration(sessions.durationFor(profileId)) }}</dd></div>
    </dl>

    <div v-if="state === 'connected' && forwards.length" class="mt-3">
      <p class="mb-2 text-xs font-medium text-ink-muted">Local ports <span class="font-normal">· click to copy</span></p>
      <ForwardList :forwards="forwards" :server="connection.profile.host" compact />
    </div>

    <p v-if="error" role="alert" class="mt-3 rounded-xl bg-bad/10 px-3 py-2 text-xs leading-relaxed text-bad">{{ error }}</p>

    <section
      v-if="childNodes.length"
      class="connection-children"
      :class="{ 'connection-children--collapsed': childrenCollapsed }"
      :aria-label="`${childHeading} via ${connection.profile.name}`"
    >
      <button
        type="button"
        class="connection-children-toggle no-drag"
        :aria-expanded="!childrenCollapsed"
        :title="childrenCollapsed ? `Show ${childNodes.length} ${childHeading.toLowerCase()}` : `Hide ${childHeading.toLowerCase()}`"
        @click="connectionsStore.toggleChildrenCollapsed(profileId)"
      >
        <span class="flex min-w-0 items-center gap-2">
          <svg
            aria-hidden="true"
            class="h-3.5 w-3.5 shrink-0 text-ink-faint transition-transform duration-150"
            :class="{ 'rotate-90': !childrenCollapsed }"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.2"
          >
            <path stroke-linecap="round" stroke-linejoin="round" d="M9 6l6 6-6 6" />
          </svg>
          <span class="text-xs font-semibold text-ink-muted">
            {{ childHeading }}
            <span class="ml-1 rounded-full bg-surface-2 px-1.5 py-0.5 font-normal text-ink-faint">{{ childNodes.length }}</span>
          </span>
          <span
            v-if="activeDependents.length"
            class="rounded-full bg-ok/15 px-2 py-0.5 text-[11px] font-medium text-ok"
          >
            {{ activeDependents.length }} active
          </span>
          <span
            v-if="childrenCollapsed && childErrorsCount"
            class="rounded-full bg-bad/15 px-2 py-0.5 text-[11px] font-medium text-bad"
          >
            {{ childErrorsCount }} {{ childErrorsCount === 1 ? 'issue' : 'issues' }}
          </span>
        </span>
        <span class="text-[11px] font-medium text-ink-faint transition group-hover:text-ink">
          {{ childrenCollapsed ? 'Show' : 'Hide' }}
        </span>
      </button>
      <div v-if="!childrenCollapsed" class="mt-3 space-y-2">
        <ConnectionCard
          v-for="child in childNodes"
          :key="child.item.profile.id"
          nested
          :connection="child.item"
          :node="child"
          @connect="emit('connect', $event)"
          @disconnect="emit('disconnect', $event)"
          @open-native="emit('openNative', $event)"
          @menu="(c, event) => emit('menu', c, event)"
        />
      </div>
    </section>
  </article>
</template>

<style scoped>
@reference '../style.css';

.connection-card {
  container-type: inline-size;
}
.connection-card--root {
  @apply rounded-xl border border-line bg-surface-1 px-4 py-3.5 transition hover:border-line-strong;
  box-shadow: 0 2px 8px #00000010, inset 0 1px #ffffff03;
}
.connection-card--root.connection-card--connected {
  border-color: color-mix(in srgb, var(--color-ok) 28%, var(--color-line));
  background: linear-gradient(125deg, #22c55e06, transparent 65%), var(--color-surface-1);
}
.connection-card--nested {
  @apply rounded-lg border border-line bg-surface-0/50 px-3 py-2.5;
}
.connection-card.connection-card--hidden {
  background: transparent;
  box-shadow: none;
}
.connection-row {
  @apply flex items-center gap-3.5;
}
.connection-card--nested .connection-row {
  align-items: center;
}
.connection-actions {
  @apply flex shrink-0 items-center justify-end gap-1.5;
}
.connection-button {
  --wails-draggable: no-drag;
  @apply inline-flex h-8 items-center justify-center rounded-lg px-3 text-xs font-semibold transition disabled:cursor-wait disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand;
}
.connection-button--primary {
  @apply border border-brand/25 bg-brand/10 text-brand hover:border-brand/40 hover:bg-brand/20;
}
.connection-button--secondary {
  @apply border border-line-strong text-ink hover:bg-surface-3;
}
.connection-menu {
  @apply grid h-8 w-8 place-items-center rounded-lg text-ink-muted transition hover:bg-surface-3 hover:text-ink focus-visible:outline-2 focus-visible:outline-brand;
}
.connection-help {
  @apply mt-2.5 flex items-center justify-between gap-2 border-t border-line/50 pt-2;
}
.connection-facts {
  @apply mt-3 grid grid-cols-3 gap-3 rounded-lg border border-line/60 bg-surface-0/40 px-3 py-2 text-xs;
}
.connection-facts dt { @apply text-[10px] font-medium uppercase tracking-wider text-ink-faint; }
.connection-facts dd { @apply mt-1 truncate text-ink; }
.connection-children {
  @apply mt-3 border-t border-line pt-2.5;
}
.connection-children-toggle {
  @apply -mx-1.5 flex w-[calc(100%+0.75rem)] items-center justify-between gap-2 rounded-lg px-1.5 py-1 text-left transition hover:bg-surface-2/70 focus-visible:outline-2 focus-visible:outline-brand;
}
@container (max-width: 360px) {
  .connection-row { flex-wrap: wrap; }
  .connection-actions { width: 100%; max-width: none; padding-left: 2.75rem; justify-content: flex-start; }
  .connection-row--iconless .connection-actions { padding-left: 0; }
  .connection-facts { grid-template-columns: 1fr; }
}
</style>
