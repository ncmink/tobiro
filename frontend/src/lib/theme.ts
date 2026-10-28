import { computed, readonly, ref, watch } from 'vue'
import { WindowSetBackgroundColour } from '@bindings/runtime/runtime'

export type ThemePreference = 'system' | 'dark' | 'light'
const storageKey = 'tobiro.theme'
const legacyStorageKeys = ['biebie-access.theme', 'januz.theme']
const systemTheme = window.matchMedia('(prefers-color-scheme: dark)')
const systemDark = ref(systemTheme.matches)

function savedPreference(): ThemePreference {
  try {
    for (const key of [storageKey, ...legacyStorageKeys]) {
      const value = localStorage.getItem(key)
      if (value === 'dark' || value === 'light' || value === 'system') {
        if (key !== storageKey) {
          localStorage.setItem(storageKey, value)
        }
        return value === 'system' ? 'system' : value
      }
    }
    return 'system'
  } catch {
    return 'system'
  }
}

const preference = ref<ThemePreference>(savedPreference())
const resolved = computed(() => preference.value === 'system'
  ? (systemDark.value ? 'dark' : 'light')
  : preference.value)

export function setTheme(value: ThemePreference) {
  preference.value = value
  try { localStorage.setItem(storageKey, value) } catch { /* Still applies for this session. */ }
}

/** Runs before Vue mounts so the first rendered UI uses the saved appearance. */
export function initTheme() {
  const onSystemChange = (event: MediaQueryListEvent) => { systemDark.value = event.matches }
  const onStorage = (event: StorageEvent) => {
    if (event.key === storageKey || event.key === null) preference.value = savedPreference()
  }
  systemTheme.addEventListener('change', onSystemChange)
  window.addEventListener('storage', onStorage)
  const stop = watch(resolved, (theme) => {
    document.documentElement.dataset.theme = theme
    document.documentElement.style.colorScheme = theme
    // The web preview has no native runtime. In Wails, match the window backing
    // colour as well, including exposed areas while resizing the window.
    if ('runtime' in window) {
      const rgb = theme === 'dark' ? [20, 21, 26] : [246, 247, 251]
      WindowSetBackgroundColour(rgb[0], rgb[1], rgb[2], 255)
    }
  }, { immediate: true, flush: 'sync' })
  return () => {
    stop()
    systemTheme.removeEventListener('change', onSystemChange)
    window.removeEventListener('storage', onStorage)
  }
}

export function useTheme() {
  return { preference: readonly(preference), resolved: readonly(resolved), setTheme }
}
