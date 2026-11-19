import type { domain } from '@/lib/bindings'

export const NAME_TAG_KEY = 'Name'

export const MAX_TAGS = 50
export const MAX_TAG_KEY_LEN = 128
export const MAX_TAG_VALUE_LEN = 256

export type TagRow = { key: string; value: string }

/** All tags for display and search, including the virtual Name tag. */
export function displayTags(name: string, tags: readonly TagRow[] | null | undefined): TagRow[] {
  const out: TagRow[] = [{ key: NAME_TAG_KEY, value: name.trim() }]
  for (const t of tags ?? []) {
    if (t.key.trim()) out.push({ key: t.key.trim(), value: t.value.trim() })
  }
  return out
}

export function normaliseStoredTags(tags: readonly TagRow[] | null | undefined): TagRow[] {
  const out: TagRow[] = []
  for (const t of tags ?? []) {
    const key = t.key.trim()
    if (!key || key === NAME_TAG_KEY) continue
    out.push({ key, value: t.value.trim() })
  }
  return out
}

export function validateStoredTags(tags: readonly TagRow[] | null | undefined): string | null {
  const cleaned = normaliseStoredTags(tags)
  if (cleaned.length > MAX_TAGS) {
    return `At most ${MAX_TAGS} tags are allowed.`
  }
  const seen = new Set<string>()
  for (const t of cleaned) {
    if (t.key.length > MAX_TAG_KEY_LEN) {
      return `Tag key is too long (max ${MAX_TAG_KEY_LEN} characters).`
    }
    if (t.value.length > MAX_TAG_VALUE_LEN) {
      return `Tag value for "${t.key}" is too long (max ${MAX_TAG_VALUE_LEN} characters).`
    }
    if (seen.has(t.key)) {
      return `Duplicate tag key "${t.key}".`
    }
    seen.add(t.key)
  }
  return null
}

/**
 * Dashboard tag matching — mirrors domain.MatchTagQuery in Go.
 */
export function matchTagQuery(
  name: string,
  tags: readonly TagRow[] | null | undefined,
  query: string,
): boolean {
  const q = query.trim()
  if (!q) return true

  const eq = q.indexOf('=')
  const key = (eq >= 0 ? q.slice(0, eq) : q).trim()
  if (!key) return false
  const value = eq >= 0 ? q.slice(eq + 1).trim() : ''

  const all = displayTags(name, tags)

  if (eq >= 0) {
    if (key === NAME_TAG_KEY) {
      if (!value) return true
      return name.toLowerCase().includes(value.toLowerCase())
    }
    for (const t of all) {
      if (t.key !== key) continue
      if (!value) return true
      return t.value.toLowerCase().includes(value.toLowerCase())
    }
    return false
  }

  const lower = q.toLowerCase()
  return all.some(
    (t) => t.key.toLowerCase().includes(lower) || t.value.toLowerCase().includes(lower),
  )
}

export function connectionMatchesSearch(
  profile: domain.VPNProfile,
  providerName: string,
  query: string,
): boolean {
  const q = query.trim()
  if (!q) return true
  const lower = q.toLowerCase()

  if (q.includes('=')) {
    return matchTagQuery(profile.name, profile.tags, q)
  }

  if (profile.name.toLowerCase().includes(lower)) return true
  if (profile.host.toLowerCase().includes(lower)) return true
  if (profile.group.toLowerCase().includes(lower)) return true
  if (providerName.toLowerCase().includes(lower)) return true
  return matchTagQuery(profile.name, profile.tags, q)
}