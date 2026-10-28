<script setup lang="ts">
import { computed, reactive, ref } from 'vue'

import type { domain } from '@bindings/go/models'
import AppModal from '@/components/AppModal.vue'
import AppSelect from '@/components/AppSelect.vue'

const props = withDefaults(
  defineProps<{
    credential?: domain.CredentialView | null
    authMethods: domain.AuthMethodInfo[]
    /** Which access kind this account is for; decides what can prove it. */
    kind?: string
  }>(),
  { kind: 'vpn' },
)

const emit = defineEmits<{
  close: []
  save: [input: domain.CredentialInput]
}>()

const editing = computed(() => Boolean(props.credential))
const isSSH = computed(() => props.kind === 'ssh')

const form = reactive({
  label: props.credential?.label ?? '',
  username: props.credential?.username ?? '',
  authMethod: props.credential?.authMethod || 'password',
  isDefault: props.credential?.isDefault ?? false,
  privateKeyPath: props.credential?.privateKeyPath ?? '',
  password: '',
})

// A private key means nothing to a VPN gateway and an OTP means nothing to an
// SSH server, so the list is narrowed rather than offering both and failing on
// save.
const available = computed(() =>
  props.authMethods.filter((m) => m.implemented && (m.kinds ?? []).includes(props.kind)),
)
const selected = computed(() => available.value.find((m) => m.id === form.authMethod))
const needsSecret = computed(() => selected.value?.needsSecret ?? true)
const needsKeyFile = computed(() => form.authMethod === 'ssh_key')

/** A key passphrase is optional; a gateway password is not. */
const offersPassphrase = computed(() => needsKeyFile.value)

const saving = ref(false)
const error = ref('')

async function submit() {
  error.value = ''
  if (form.label.trim() === '') {
    error.value = 'Give this account a display name.'
    return
  }
  if (needsKeyFile.value && form.privateKeyPath.trim() === '') {
    error.value = 'Enter the path to the private key file.'
    return
  }
  if (!editing.value && needsSecret.value && form.password === '') {
    error.value = 'Enter the password, or it will not be saved to the keychain.'
    return
  }

  // The parent owns the save; it either closes this dialog or calls fail(),
  // which is what clears the saving state.
  saving.value = true
  emit('save', {
    label: form.label.trim(),
    username: form.username.trim(),
    authMethod: form.authMethod,
    isDefault: form.isDefault,
    privateKeyPath: needsKeyFile.value ? form.privateKeyPath.trim() : '',
    password: form.password,
  } as domain.CredentialInput)
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
    :title="editing ? 'Edit account' : 'Add account'"
    subtitle="The password is stored in your operating system's credential store, never in application data."
    @close="emit('close')"
  >
    <form class="space-y-4" @submit.prevent="submit">
      <div>
        <label class="mb-1.5 block text-sm font-medium text-ink" for="cred-label">
          Display name
        </label>
        <input
          id="cred-label"
          v-model="form.label"
          type="text"
          placeholder="Ming"
          autocomplete="off"
          class="w-full rounded-xl border border-line bg-surface-2 px-3 py-2 text-sm text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
        />
        <p class="mt-1 text-xs text-ink-faint">
          How you will recognise this identity, e.g. "Ming" or "DevOps Shared".
        </p>
      </div>

      <div v-if="!isSSH">
        <label class="mb-1.5 block text-sm font-medium text-ink" for="cred-username">
          Username
        </label>
        <input
          id="cred-username"
          v-model="form.username"
          type="text"
          placeholder="devops"
          autocomplete="off"
          spellcheck="false"
          class="w-full rounded-xl border border-line bg-surface-2 px-3 py-2 font-mono text-sm text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
        />
        <p class="mt-1 text-xs text-ink-faint">
          Usernames do not need to be unique — the same one may be used on several accounts.
        </p>
      </div>

      <div>
        <label class="mb-1.5 block text-sm font-medium text-ink" for="cred-auth">
          Authentication
        </label>
        <AppSelect id="cred-auth" v-model="form.authMethod">
          <option v-for="method in available" :key="method.id" :value="method.id">
            {{ method.displayName }}
          </option>
        </AppSelect>
        <p v-if="selected?.description" class="mt-1 text-xs leading-relaxed text-ink-faint">
          {{ selected.description }}
        </p>
      </div>

      <div v-if="needsKeyFile">
        <label class="mb-1.5 block text-sm font-medium text-ink" for="cred-key">
          Private key file
        </label>
        <input
          id="cred-key"
          v-model="form.privateKeyPath"
          type="text"
          placeholder="~/.ssh/id_ed25519"
          autocomplete="off"
          spellcheck="false"
          class="w-full rounded-xl border border-line bg-surface-2 px-3 py-2 font-mono text-sm text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
        />
        <p class="mt-1 text-xs leading-relaxed text-ink-faint">
          The file stays where it is. Only its path is saved here — never the key itself.
        </p>
      </div>

      <div v-if="needsSecret || offersPassphrase">
        <label class="mb-1.5 block text-sm font-medium text-ink" for="cred-password">
          {{ offersPassphrase ? 'Key passphrase' : 'Password' }}
          <span v-if="offersPassphrase" class="font-normal text-ink-faint">
            (only if the key has one)
          </span>
        </label>
        <input
          id="cred-password"
          v-model="form.password"
          type="password"
          :placeholder="editing ? 'Leave blank to keep what is saved' : '••••••••••'"
          autocomplete="new-password"
          class="w-full rounded-xl border border-line bg-surface-2 px-3 py-2 text-sm text-ink outline-none transition placeholder:text-ink-faint focus:border-brand"
        />
      </div>

      <label class="flex cursor-pointer items-center gap-2.5 text-sm text-ink">
        <input v-model="form.isDefault" type="checkbox" class="h-4 w-4 accent-brand" />
        Make default account
      </label>

      <p v-if="error" class="rounded-xl bg-bad/10 px-3 py-2 text-sm text-bad">{{ error }}</p>
    </form>

    <template #footer>
      <button
        type="button"
        class="rounded-xl px-4 py-2 text-sm font-medium text-ink-muted transition hover:bg-ink/5 hover:text-ink"
        @click="emit('close')"
      >
        Cancel
      </button>
      <button
        type="button"
        class="rounded-xl bg-brand px-4 py-2 text-sm font-semibold text-surface-1 transition hover:bg-brand-strong disabled:opacity-50"
        :disabled="saving"
        @click="submit"
      >
        {{ saving ? 'Saving…' : 'Save' }}
      </button>
    </template>
  </AppModal>
</template>
