import { ref } from 'vue'

import { errorMessage } from '@/lib/state'
import { useConnectionsStore } from '@/stores/connections'

export type SharingNotice = { text: string; tone: 'ok' | 'bad' }

const notice = ref<SharingNotice | null>(null)
const sharing = ref(false)
let noticeTimer: number | undefined

function announce(text: string, tone: 'ok' | 'bad' = 'ok') {
  notice.value = { text, tone }
  if (noticeTimer !== undefined) window.clearTimeout(noticeTimer)
  noticeTimer = window.setTimeout(() => (notice.value = null), 9_000)
}

function fileName(path: string) {
  return path.split(/[/\\]/).pop() || path
}

export function useConnectionSharing() {
  const store = useConnectionsStore()

  async function exportConnections() {
    sharing.value = true
    try {
      const result = await store.exportToFile()
      if (result.cancelled) return
      announce(
        `Saved ${result.connections} ${result.connections === 1 ? 'connection' : 'connections'} to ${fileName(result.file)}. Passwords are not in the file.`,
      )
    } catch (err) {
      announce(errorMessage(err, 'Could not export the connections'), 'bad')
    } finally {
      sharing.value = false
    }
  }

  async function importConnections() {
    sharing.value = true
    try {
      const result = await store.importFromFile()
      if (result.cancelled) return

      const parts: string[] = []
      if (result.connections > 0) {
        parts.push(`Added ${result.connections} ${result.connections === 1 ? 'connection' : 'connections'}`)
        if (result.accounts > 0) {
          parts.push(`${result.accounts} ${result.accounts === 1 ? 'account needs' : 'accounts need'} a password`)
        }
      }
      if (result.skipped?.length) {
        parts.push(`skipped ${result.skipped.join(', ')} — already here`)
      }
      announce(parts.length ? `${parts.join(' · ')}.` : 'That file added nothing new.')
    } catch (err) {
      announce(errorMessage(err, 'Could not import that file'), 'bad')
    } finally {
      sharing.value = false
    }
  }

  function dismissNotice() {
    notice.value = null
    if (noticeTimer !== undefined) window.clearTimeout(noticeTimer)
  }

  return {
    notice,
    sharing,
    exportConnections,
    importConnections,
    dismissNotice,
  }
}
