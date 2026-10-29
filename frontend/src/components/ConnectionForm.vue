<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'

import { ListVendorProfiles } from '@bindings/go/main/App'
import type { domain, main } from '@bindings/go/models'
import AppModal from '@/components/AppModal.vue'
import AppSelect from '@/components/AppSelect.vue'
import { errorMessage } from '@/lib/state'

const props = defineProps<{
  connection?: main.ConnectionView | null
  providers: domain.ProviderInfo[]
  /**
   * Which kind of connection is being added, chosen before this form opens.
   *
   * How a connection reaches the network decides what the whole form asks for,
   * and it is not something an existing connection changes into: a tunnel and a
   * vendor client have nothing in common to carry over. So it arrives already
   * decided, and editing one never offers to make it the other.
   */
  kind?: string
  /** Existing group names, offered as suggestions so they stay consistent. */
  groups?: string[]
  /** Every other connection, offered as the one this may need up first. */
  connections?: main.ConnectionView[]
}>()

const emit = defineEmits<{
  close: []
  save: [input: domain.ProfileInput]
}>()

const editing = computed(() => Boolean(props.connection))

const form = reactive({
  name: props.connection?.profile.name ?? '',
  kind: props.connection?.profile.kind || props.kind || 'vpn',
  group: props.connection?.profile.group ?? '',
  requires: props.connection?.profile.requires ?? '',
  provider: props.connection?.profile.provider ?? props.providers[0]?.id ?? 'forcepoint',
  host: props.connection?.profile.host ?? '',
  port: props.connection?.profile.port ?? 443,
  vendorProfile: props.connection?.profile.vendorProfile ?? '',
  clientPath: props.connection?.profile.clientPath ?? '',
  notes: props.connection?.profile.notes ?? '',
})

const isSSH = computed(() => form.kind === 'ssh')

/** The kind is no longer a field here, so the heading is what names it. */
const heading = computed(() => {
  if (editing.value) return isSSH.value ? 'Edit tunnel details' : 'Edit VPN connection'
  return isSSH.value ? 'Add an SSH tunnel' : 'Add a VPN connection'
})

/**
 * The connections that may be named as "needed first".
 *
 * Itself is out, and so is anything already waiting on it: two connections that
 * need each other could never be brought up, so the choice is not offered
 * rather than saved and then refused.
 */
const requirementOptions = computed(() => {
  const all = props.connections ?? []
  const own = props.connection?.profile.id
  if (!own) return all

  const waiting = new Set<string>([own])
  for (let pass = 0; pass < all.length; pass++) {
    const before = waiting.size
    for (const candidate of all) {
      if (candidate.profile.requires && waiting.has(candidate.profile.requires)) {
        waiting.add(candidate.profile.id)
      }
    }
    if (waiting.size === before) break
  }
  return all.filter((c) => !waiting.has(c.profile.id))
})

// Offer the vendor's usual port when the provider changes on a new connection.
watch(
  () => form.provider,
  (provider) => {
    if (isSSH.value) return
    form.vendorProfile = provider === props.connection?.profile.provider
      ? props.connection.profile.vendorProfile ?? ''
      : ''
    if (editing.value) return
    const info = props.providers.find((p) => p.id === provider)
    if (info) form.port = info.defaultPort
  },
)

const saving = ref(false)
const error = ref('')

/** Which provider is selected decides whether a gateway is asked for at all. */
const providerInfo = computed(() => props.providers.find((p) => p.id === form.provider))
const needsEndpoint = computed(() => providerInfo.value?.requiresEndpoint ?? true)

/** Some providers connect by a name configured on the vendor side instead. */
const vendorProfileField = computed(() => {
  const field = providerInfo.value?.vendorProfileField
  if (!field?.label) return undefined
  return field
})
const checksForcepointStatus = computed(() => form.provider === 'forcepoint')
const profileLabel = computed(() => vendorProfileField.value?.label ?? 'Vendor profile')

/** Adapters that implement VendorProfileLister on this platform. */
const vendorProfileListerProviders = new Set(['macosvpn', 'tailscale'])

/**
 * What the provider already has configured on this machine.
 *
 * macOS VPN and Tailscale can enumerate saved services or logged-in accounts;
 * other providers with a vendor field fall back to manual entry.
 */
const discovered = ref<domain.VendorProfileOption[]>([])
const discovering = ref(false)
const discoveryError = ref('')

watch(
  [() => form.provider, () => vendorProfileField.value?.label],
  async ([provider, label], _previous, onCleanup) => {
    discovered.value = []
    discoveryError.value = ''
    discovering.value = false
    if (isSSH.value || !label || !vendorProfileListerProviders.has(provider)) return

    let cancelled = false
    let timer: ReturnType<typeof setTimeout> | undefined
    onCleanup(() => {
      cancelled = true
      if (timer) clearTimeout(timer)
    })
    discovering.value = true
    const timeoutHint = provider === 'tailscale'
      ? 'Could not load Tailscale accounts. Enter a nickname or tailnet from tailscale switch --list.'
      : 'Could not load saved VPN connections. Enter the name shown in System Settings.'
    try {
      const options = await Promise.race([
        ListVendorProfiles(provider),
        new Promise<never>((_resolve, reject) => {
          timer = setTimeout(() => reject(new Error(timeoutHint)), 5000)
        }),
      ])
      if (!cancelled) discovered.value = options ?? []
    } catch (err) {
      if (!cancelled) {
        discoveryError.value = errorMessage(
          err,
          provider === 'tailscale'
            ? 'Could not load Tailscale accounts. You can enter a nickname or tailnet below.'
            : 'Could not load saved VPN connections. You can enter the name below.',
        )
      }
    } finally {
      if (timer) clearTimeout(timer)
      if (!cancelled) discovering.value = false
    }
  },
  { immediate: true },
)

function submit() {
  error.value = ''
  if (form.name.trim() === '') {
    error.value = 'Give this connection a name.'
    return
  }

  if (isSSH.value) {
    submitSSH()
    return
  }

  if (needsEndpoint.value && form.host.trim() === '') {
    error.value = checksForcepointStatus.value ? 'Enter an address reachable through this VPN for the status check.' : 'Enter the VPN gateway address.'
    return
  }
  const vendorField = vendorProfileField.value
  if (vendorField?.required && form.vendorProfile.trim() === '') {
    error.value = `Enter the ${vendorField.label}.`
    return
  }

  saving.value = true
  emit('save', {
    name: form.name.trim(),
    kind: 'vpn',
    group: form.group.trim(),
    requires: form.requires,
    provider: form.provider,
    host: needsEndpoint.value ? form.host.trim() : '',
    port: needsEndpoint.value ? Number(form.port) : 0,
    vendorProfile: vendorProfileField.value?.label || props.connection?.profile.provider === form.provider ? form.vendorProfile.trim() : '',
    clientPath: form.clientPath.trim(),
    notes: form.notes,
  } as domain.ProfileInput)
}

/**
 * Saves the connection itself, not the tunnel.
 *
 * The server, the login and the port map are set up on the connection's page
 * once it exists — the same way its accounts are — so they are absent here and
 * are left untouched by this save.
 */
function submitSSH() {
  saving.value = true
  emit('save', {
    name: form.name.trim(),
    kind: 'ssh',
    group: form.group.trim(),
    requires: form.requires,
    provider: 'ssh',
    host: '',
    port: 0,
    vendorProfile: '',
    clientPath: '',
    notes: form.notes,
    ssh: { user: '', forwards: [] },
  } as unknown as domain.ProfileInput)
}

defineExpose({
  fail(message: string) {
    error.value = message
    saving.value = false
  },
})
</script>

<template>
  <AppModal
    :title="heading"
    :subtitle="
      isSSH
        ? 'Change the name and group here. Server and ports are in Settings; passwords and keys are in Accounts.'
        : 'Choose the VPN client and how this connection is identified. Login accounts are managed separately.'
    "
    @close="emit('close')"
  >
    <form class="space-y-4" @submit.prevent="submit">
      <div>
        <label class="mb-1.5 block text-sm font-medium text-ink" for="conn-name">
          Connection name
        </label>
        <input
          id="conn-name"
          v-model="form.name"
          type="text"
          placeholder=""
          class="w-full rounded-xl border border-line bg-surface-2 px-3 py-2 text-sm text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
        />
      </div>

      <div>
        <label class="mb-1.5 block text-sm font-medium text-ink" for="conn-group">
          Customer / group <span class="font-normal text-ink-faint">(optional)</span>
        </label>
        <input
          id="conn-group"
          v-model="form.group"
          type="text"
          list="conn-group-options"
          placeholder="Company or client this connection belongs to"
          class="w-full rounded-xl border border-line bg-surface-2 px-3 py-2 text-sm text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
        />
        <datalist id="conn-group-options">
          <option v-for="group in groups ?? []" :key="group" :value="group" />
        </datalist>
        <p class="mt-1.5 text-xs leading-relaxed text-ink-muted">
          Use a customer or team name to keep related connections together.
        </p>
      </div>

      <!-- A bastion inside the customer's network cannot be dialled until the
           VPN is carrying the route. Recording that here is what lets one
           Connect bring up the whole path. -->
      <div v-if="isSSH && requirementOptions.length">
        <label class="mb-1.5 block text-sm font-medium text-ink" for="conn-requires">
          Needs another connection first <span class="font-normal text-ink-faint">(optional)</span>
        </label>
        <AppSelect id="conn-requires" v-model="form.requires">
          <option value="">Nothing — this connects on its own</option>
          <option v-for="option in requirementOptions" :key="option.profile.id" :value="option.profile.id">
            {{ option.profile.name }}{{ option.profile.kind === 'ssh' ? ' (SSH tunnel)' : '' }}
          </option>
        </AppSelect>
        <p class="mt-1.5 text-xs leading-relaxed text-ink-muted">
          Choose a connection that must be available first. Disconnecting it also closes dependent connections.
        </p>
      </div>

      <div v-if="!isSSH">
        <label class="mb-1.5 block text-sm font-medium text-ink" for="conn-provider">
          VPN client
        </label>
        <AppSelect id="conn-provider" v-model="form.provider">
          <option v-for="provider in providers" :key="provider.id" :value="provider.id">
            {{ provider.displayName }}
          </option>
        </AppSelect>
      </div>

      <div v-if="!isSSH && needsEndpoint" class="grid grid-cols-[minmax(0,1fr)_100px] gap-3">
        <div>
          <label class="mb-1.5 block text-sm font-medium text-ink" for="conn-host">{{ checksForcepointStatus ? 'VPN status check address' : 'VPN gateway address' }}</label>
          <input
            id="conn-host"
            v-model="form.host"
            type="text"
            :placeholder="checksForcepointStatus ? 'e.g. internal.example.com' : 'e.g. vpn.example.com'"
            spellcheck="false"
            class="w-full rounded-xl border border-line bg-surface-2 px-3 py-2 font-mono text-sm text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
          />
        </div>
        <div>
          <label class="mb-1.5 block text-sm font-medium text-ink" for="conn-port">{{ checksForcepointStatus ? 'Check port' : 'VPN port' }}</label>
          <input
            id="conn-port"
            v-model.number="form.port"
            type="number"
            min="1"
            max="65535"
            class="w-full rounded-xl border border-line bg-surface-2 px-3 py-2 font-mono text-sm text-ink outline-none transition focus:border-brand"
          />
        </div>
      </div>

      <p
        v-else-if="!isSSH"
        class="rounded-xl border border-line bg-surface-2 px-3 py-2 text-xs leading-relaxed text-ink-muted"
      >
        {{ providerInfo?.endpointNote }}
      </p>

      <p v-if="!isSSH && needsEndpoint" class="text-xs leading-relaxed text-ink-muted">
        {{ checksForcepointStatus
          ? 'An address and port reachable through this VPN, used to help check its status. The VPN gateway and connection are configured in Forcepoint.'
          : 'Use the gateway and port provided by your VPN administrator. SSH destinations are configured on their own tunnel.' }}
      </p>

      <div v-if="!isSSH && vendorProfileField?.label">
        <label class="mb-1.5 block text-sm font-medium text-ink" for="conn-vendor">
          {{ profileLabel }}
          <span v-if="!vendorProfileField.required" class="font-normal text-ink-faint">(optional)</span>
        </label>

        <p v-if="discovering" role="status" class="text-xs text-ink-muted">
          {{ form.provider === 'tailscale' ? 'Looking for Tailscale accounts on this machine…' : 'Looking for saved VPN connections…' }}
          You can enter the name below.
        </p>

        <!-- Offer what the machine actually has, so nothing has to be typed
             from memory. Only macOS can list its own services today. -->
        <div v-if="discovered.length" class="max-h-52 space-y-1.5 overflow-y-auto">
          <button
            v-for="option in discovered"
            :key="option.name"
            type="button"
            class="flex w-full items-center gap-3 rounded-xl border px-3 py-2 text-left transition"
            :class="
              form.vendorProfile === option.name
                ? 'border-brand bg-brand/10'
                : 'border-line bg-surface-2 hover:border-ink-faint'
            "
            @click="form.vendorProfile = option.name"
          >
            <span class="min-w-0 flex-1">
              <span class="block truncate text-sm font-medium text-ink">{{ option.name }}</span>
              <span class="block text-xs text-ink-muted">{{ option.detail }}</span>
            </span>
            <span
              class="shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium"
              :class="option.connected ? 'bg-ok/15 text-ok' : 'bg-surface-3 text-ink-faint'"
            >
              {{ option.state || (option.connected ? 'Active' : '') }}
            </span>
          </button>
        </div>

        <input
          id="conn-vendor"
          v-model="form.vendorProfile"
          type="text"
          :placeholder="vendorProfileField.hint || (form.provider === 'macosvpn' ? 'Exact name shown in System Settings' : 'Exact connection name in FortiClient')"
          class="mt-2 w-full rounded-xl border border-line bg-surface-2 px-3 py-2 text-sm text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
        />

        <p v-if="vendorProfileField.hint && !discovering" class="mt-1.5 text-xs leading-relaxed text-ink-muted">
          {{ vendorProfileField.hint }}
        </p>
        <p v-if="discoveryError" class="mt-1.5 text-xs text-bad">{{ discoveryError }}</p>
        <p
          v-else-if="form.provider === 'macosvpn' && !discovering && !discovered.length"
          class="mt-1.5 text-xs leading-relaxed text-ink-muted"
        >
          No saved VPN connections were found. Enter an existing name, or create the VPN in System Settings first.
        </p>
        <p
          v-else-if="form.provider === 'tailscale' && !discovering && !discovered.length"
          class="mt-1.5 text-xs leading-relaxed text-ink-muted"
        >
          No Tailscale accounts were found. Open Tailscale, sign in or join the tailnet, then try again.
        </p>
      </div>

      <details v-if="!isSSH" :open="Boolean(connection?.profile.clientPath)" class="rounded-xl border border-line p-3">
        <summary class="no-drag cursor-pointer text-sm font-medium text-ink-muted">Advanced · VPN application location</summary>
        <label class="mt-3 mb-1.5 block text-sm font-medium text-ink" for="conn-path">
          VPN application path <span class="font-normal text-ink-faint">(optional)</span>
        </label>
        <input
          id="conn-path"
          v-model="form.clientPath"
          type="text"
          placeholder="Leave blank to detect automatically"
          spellcheck="false"
          class="w-full rounded-xl border border-line bg-surface-2 px-3 py-2 font-mono text-sm text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
        />
      </details>

      <div>
        <label class="mb-1.5 block text-sm font-medium text-ink" for="conn-notes">
          Notes <span class="font-normal text-ink-faint">(optional)</span>
        </label>
        <textarea
          id="conn-notes"
          v-model="form.notes"
          rows="2"
          class="w-full resize-none rounded-xl border border-line bg-surface-2 px-3 py-2 text-sm text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
        />
      </div>

      <p v-if="error" class="rounded-xl bg-bad/10 px-3 py-2 text-sm text-bad">{{ error }}</p>
    </form>

    <template #footer>
      <button type="button" class="btn-ghost" @click="emit('close')">
        Cancel
      </button>
      <button type="button" class="btn-brand" :disabled="saving" @click="submit">
        {{ saving ? 'Saving…' : 'Save' }}
      </button>
    </template>
  </AppModal>
</template>
