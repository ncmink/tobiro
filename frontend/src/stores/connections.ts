import { computed, ref } from 'vue'
import { acceptHMRUpdate, defineStore } from 'pinia'

import {
  AddCredential,
  CreateConnection,
  DeleteConnection,
  DeleteCredential,
  ExportConnections,
  GetBootstrap,
  ImportConnections,
  ImportSSHConfigHosts,
  ListConnections,
  ListGroups,
  ListProviderStatus,
  MoveConnection,
  RefreshProviderDetection,
  SaveTunnel,
  SetDefaultCredential,
  SetGroupHidden,
  GetClientStartup,
  GetNetworkDiagnosis,
  SetClientStartup,
  UpdateConnection,
  UpdateCredential,
} from '@bindings/go/main/App'
import type { domain, main } from '@bindings/go/models'
import { connectionMap, isDashboardVisible } from '@/lib/visibility'
import { errorMessage } from '@/lib/state'

/**
 * Holds connection profiles and their accounts.
 *
 * Everything here comes from the Go core through Wails bindings. Note what is
 * absent: passwords. The backend only ever sends `hasPassword`, so no secret
 * can end up in this store, in devtools, or in localStorage.
 */
/** Whether hidden groups are currently revealed. A view preference, not data. */
const showHiddenKey = 'tobiro.show-hidden-groups'
const legacyShowHiddenKeys = ['biebie-access.show-hidden-groups', 'januz.show-hidden-groups']
/** Parent connection IDs whose nested SSH tunnels / dependents are collapsed on the dashboard. */
const collapsedChildrenKey = 'tobiro.collapsed-children'

function readShowHidden(): boolean {
  try {
    for (const key of [showHiddenKey, ...legacyShowHiddenKeys]) {
      if (localStorage.getItem(key) === '1') {
        if (key !== showHiddenKey) {
          localStorage.setItem(showHiddenKey, '1')
        }
        return true
      }
    }
  } catch { /* ignore */ }
  return false
}

function readCollapsedChildren(): string[] {
  try {
    const raw = localStorage.getItem(collapsedChildrenKey)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (Array.isArray(parsed)) {
      return parsed.filter((id): id is string => typeof id === 'string' && id.length > 0)
    }
  } catch { /* ignore */ }
  return []
}

function writeCollapsedChildren(ids: string[]) {
  try {
    localStorage.setItem(collapsedChildrenKey, JSON.stringify(ids))
  } catch { /* ignore */ }
}

export const useConnectionsStore = defineStore('connections', () => {
  const connections = ref<main.ConnectionView[]>([])
  const groups = ref<domain.GroupView[]>([])
  const bootstrap = ref<main.BootstrapView | null>(null)
  const providerStatus = ref<domain.ProviderStatusView[]>([])

  const loading = ref(false)
  const loaded = ref(false)
  const error = ref('')
  const showHidden = ref(readShowHidden())
  const collapsedChildrenIds = ref<string[]>(readCollapsedChildren())

  const byId = computed(
    () => (id: string) => connections.value.find((c) => c.profile.id === id) ?? null,
  )

  const isEmpty = computed(() => loaded.value && connections.value.length === 0)

  /** Group names offered in the connection form, ungrouped excluded. */
  const groupNames = computed(() => groups.value.map((g) => g.name).filter((name) => name !== ''))

  /** How many connections sit in a hidden group, whether revealed or not. */
  const hiddenCount = computed(() =>
    groups.value.filter((g) => g.hidden).reduce((total, g) => total + g.connections, 0),
  )

  const visibleConnections = computed(() => {
    if (showHidden.value) return connections.value
    const byId = connectionMap(connections.value)
    return connections.value.filter((c) => isDashboardVisible(c, byId, false))
  })

  async function loadBootstrap() {
    try {
      bootstrap.value = await GetBootstrap()
    } catch (err) {
      error.value = errorMessage(err, 'Could not read application settings')
    }
  }

  async function refresh() {
    loading.value = true
    try {
      const [list, groupList] = await Promise.all([ListConnections(), ListGroups()])
      connections.value = list
      groups.value = groupList
      loaded.value = true
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, 'Could not load connections')
    } finally {
      loading.value = false
    }
  }

  function setShowHidden(value: boolean) {
    showHidden.value = value
    localStorage.setItem(showHiddenKey, value ? '1' : '0')
  }

  function toggleShowHidden() {
    setShowHidden(!showHidden.value)
  }

  function isChildrenCollapsed(profileId: string): boolean {
    return collapsedChildrenIds.value.includes(profileId)
  }

  function setChildrenCollapsed(profileId: string, collapsed: boolean) {
    const exists = collapsedChildrenIds.value.includes(profileId)
    if (collapsed && !exists) {
      collapsedChildrenIds.value = [...collapsedChildrenIds.value, profileId]
      writeCollapsedChildren(collapsedChildrenIds.value)
    } else if (!collapsed && exists) {
      collapsedChildrenIds.value = collapsedChildrenIds.value.filter((id) => id !== profileId)
      writeCollapsedChildren(collapsedChildrenIds.value)
    }
  }

  function toggleChildrenCollapsed(profileId: string) {
    setChildrenCollapsed(profileId, !isChildrenCollapsed(profileId))
  }

  function setAllChildrenCollapsed(profileIds: string[], collapsed: boolean) {
    if (collapsed) {
      const merged = new Set([...collapsedChildrenIds.value, ...profileIds])
      collapsedChildrenIds.value = Array.from(merged)
    } else {
      const toExpand = new Set(profileIds)
      collapsedChildrenIds.value = collapsedChildrenIds.value.filter((id) => !toExpand.has(id))
    }
    writeCollapsedChildren(collapsedChildrenIds.value)
  }

  /** Hides or reveals a whole group. Nothing is deleted — this is presentation. */
  async function setGroupHidden(name: string, hidden: boolean) {
    groups.value = await SetGroupHidden(name, hidden)
    await refresh()
  }

  async function loadProviderStatus(reDetect = false) {
    try {
      providerStatus.value = reDetect ? await RefreshProviderDetection() : await ListProviderStatus()
    } catch (err) {
      error.value = errorMessage(err, 'Could not detect VPN clients')
    }
  }

  async function createConnection(input: domain.ProfileInput) {
    const created = await CreateConnection(input)
    await refresh()
    return created
  }

  async function updateConnection(id: string, input: domain.ProfileInput) {
    const updated = await UpdateConnection(id, input)
    await refresh()
    return updated
  }

  /**
   * Saves the SSH half of a connection: its server, login and port map.
   *
   * Allowed while the tunnel is up. The running tunnel keeps the ports it bound
   * at connect time, so the saved map and the live one can differ until the
   * next connect.
   */
  async function saveTunnel(id: string, input: domain.TunnelInput) {
    const updated = await SaveTunnel(id, input)
    await refresh()
    return updated
  }

  /**
   * Files a connection under another group. An empty name takes it out of every
   * group. Allowed while connected — a group is not part of how it connects.
   */
  async function moveConnection(id: string, group: string) {
    const moved = await MoveConnection(id, group)
    await refresh()
    return moved
  }

  async function removeConnection(id: string) {
    await DeleteConnection(id)
    await refresh()
  }

  async function addCredential(profileId: string, input: domain.CredentialInput) {
    const created = await AddCredential(profileId, input)
    await refresh()
    return created
  }

  async function updateCredential(credentialId: string, input: domain.CredentialInput) {
    const updated = await UpdateCredential(credentialId, input)
    await refresh()
    return updated
  }

  async function removeCredential(credentialId: string) {
    await DeleteCredential(credentialId)
    await refresh()
  }

  async function makeDefaultCredential(credentialId: string) {
    await SetDefaultCredential(credentialId)
    await refresh()
  }

  async function clientStartup(profileId: string) {
    return GetClientStartup(profileId)
  }

  async function setClientStartup(profileId: string, enabled: boolean) {
    return SetClientStartup(profileId, enabled)
  }

  async function networkDiagnosis(profileId: string) {
    return GetNetworkDiagnosis(profileId)
  }

  /** Writes every connection to a YAML file the user picks. Never any password. */
  async function exportToFile() {
    return ExportConnections()
  }

  /** Adds the connections from an exported YAML file, skipping names already here. */
  async function importFromFile() {
    const result = await ImportConnections()
    if (result.connections > 0) await refresh()
    return result
  }

  async function importSshConfigFromHosts(parentId: string, aliases: string[]) {
    const result = await ImportSSHConfigHosts(parentId, aliases)
    if (result.connections > 0) await refresh()
    return result
  }

  function applySession(session: {
    profileId: string
    state: string
    assignedIp?: string
    interface?: string
    identity?: string
    forwards?: domain.PortForward[]
    error?: string
  }) {
    connections.value = connections.value.map((connection) => {
      if (connection.profile.id !== session.profileId) return connection
      return {
        ...connection,
        state: session.state,
        session: {
          ...(connection.session ?? { id: '', profileId: session.profileId, credentialId: '' }),
          profileId: session.profileId,
          state: session.state,
          assignedIp: session.assignedIp ?? connection.session?.assignedIp ?? '',
          interface: session.interface ?? connection.session?.interface ?? '',
          identity: session.identity ?? connection.session?.identity ?? '',
          forwards: session.forwards ?? connection.session?.forwards ?? [],
          error: session.error ?? connection.session?.error ?? '',
        },
      } as main.ConnectionView
    })
  }

  return {
    connections,
    groups,
    bootstrap,
    providerStatus,
    loading,
    loaded,
    error,
    showHidden,
    byId,
    isEmpty,
    groupNames,
    hiddenCount,
    visibleConnections,
    loadBootstrap,
    refresh,
    setShowHidden,
    toggleShowHidden,
    collapsedChildrenIds,
    isChildrenCollapsed,
    setChildrenCollapsed,
    toggleChildrenCollapsed,
    setAllChildrenCollapsed,
    setGroupHidden,
    loadProviderStatus,
    createConnection,
    updateConnection,
    saveTunnel,
    moveConnection,
    removeConnection,
    addCredential,
    updateCredential,
    removeCredential,
    makeDefaultCredential,
    clientStartup,
    setClientStartup,
    networkDiagnosis,
    exportToFile,
    importFromFile,
    importSshConfigFromHosts,
    applySession,
  }
})

// A setup store is a closure, so a hot reload replaces the module while the
// running instance keeps the actions it was built with — a component calling one
// added a moment ago would find it undefined. Handing the update to Pinia
// rebuilds the instance instead, which is what makes editing a store during
// `wails dev` behave like editing a component.
if (import.meta.hot) {
  import.meta.hot.accept(acceptHMRUpdate(useConnectionsStore, import.meta.hot))
}
