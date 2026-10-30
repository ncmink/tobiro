import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import {
  GetFillPermissions,
  GetFillShortcut,
  OpenPrivacySettings,
  ResetFillShortcut,
  SetFillShortcut,
  ShowFillShortcutIntro,
} from '@bindings/go/main/App'
import type { main } from '@bindings/go/models'
import { errorMessage } from '@/lib/state'

export const useFillShortcutStore = defineStore('fillShortcut', () => {
  const shortcut = ref<main.FillShortcutView | null>(null)
  const permissions = ref<main.FillPermissionsView | null>(null)
  const loading = ref(false)
  const saveError = ref('')

  const supported = computed(() => shortcut.value?.supported === true)
  const display = computed(() => shortcut.value?.display || '⌘I')
  const showIntro = computed(
    () => supported.value && shortcut.value != null && !shortcut.value.introSeen,
  )

  async function refresh() {
    loading.value = true
    saveError.value = ''
    try {
      shortcut.value = await GetFillShortcut()
      permissions.value = await GetFillPermissions()
    } finally {
      loading.value = false
    }
  }

  async function applyCombo(combo: string) {
    saveError.value = ''
    try {
      await SetFillShortcut(combo)
      await refresh()
      return true
    } catch (err) {
      saveError.value = errorMessage(err, 'Could not save the shortcut')
      await refresh()
      return false
    }
  }

  async function reset() {
    saveError.value = ''
    try {
      await ResetFillShortcut()
      await refresh()
    } catch (err) {
      saveError.value = errorMessage(err, 'Could not reset the shortcut')
    }
  }

  async function openPrivacy(pane: 'accessibility' | 'automation' | 'listen') {
    await OpenPrivacySettings(pane)
  }

  async function replayIntro() {
    await ShowFillShortcutIntro()
    await refresh()
  }

  const loginDetected = ref<{
    profileId: string
    connectionName: string
    shortcutDisplay: string
  } | null>(null)

  function showLoginDetected(payload: {
    profileId?: string
    connectionName?: string
    shortcutDisplay?: string
  }) {
    const profileId = payload.profileId?.trim()
    if (!profileId) return
    loginDetected.value = {
      profileId,
      connectionName: payload.connectionName?.trim() || 'This connection',
      shortcutDisplay: payload.shortcutDisplay?.trim() || display.value,
    }
  }

  function dismissLoginDetected() {
    loginDetected.value = null
  }

  return {
    shortcut,
    permissions,
    loading,
    saveError,
    supported,
    display,
    showIntro,
    refresh,
    applyCombo,
    reset,
    openPrivacy,
    replayIntro,
    loginDetected,
    showLoginDetected,
    dismissLoginDetected,
  }
})

export function comboFromKeyboardEvent(event: KeyboardEvent): string | null {
  if (event.key === 'Escape') return null
  const key = normalizeShortcutKey(event)
  if (!key) return null

  const mods: string[] = []
  if (event.metaKey) mods.push('cmd')
  if (event.ctrlKey) mods.push('ctrl')
  if (event.altKey) mods.push('option')
  if (event.shiftKey) mods.push('shift')
  if (mods.length === 0) return null
  mods.sort()
  return [...mods, key].join('+')
}

function normalizeShortcutKey(event: KeyboardEvent): string {
  if (['Meta', 'Control', 'Alt', 'Shift', 'OS'].includes(event.key)) return ''
  if (event.key.length === 1) return event.key.toLowerCase()
  if (/^F\d{1,2}$/i.test(event.key)) return event.key.toLowerCase()
  return ''
}
