/**
 * Format an ISO timestamp as a relative time string (e.g. "50m ago").
 * Returns a fallback for missing, invalid, or far-future timestamps.
 */
export function timeAgo(iso: string | undefined, fallback = '—'): string {
  if (!iso) return fallback
  const d = new Date(iso)
  if (isNaN(d.getTime()) || d.getFullYear() < 2000) return fallback
  const diff = Math.floor((Date.now() - d.getTime()) / 1000)
  if (diff < 0) return 'just now'
  if (diff < 60) return `${diff}s ago`
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`
  return `${Math.floor(diff / 86400)}d ago`
}

/**
 * Same as timeAgo but with French labels.
 */
export function timeAgoFr(iso: string | undefined, fallback = 'Jamais'): string {
  if (!iso) return fallback
  const d = new Date(iso)
  if (isNaN(d.getTime()) || d.getFullYear() < 2000) return fallback
  const diff = Math.floor((Date.now() - d.getTime()) / 1000)
  if (diff < 0) return 'À l\'instant'
  if (diff < 60) return 'À l\'instant'
  if (diff < 3600) return `Il y a ${Math.floor(diff / 60)}m`
  if (diff < 86400) return `Il y a ${Math.floor(diff / 3600)}h`
  return `Il y a ${Math.floor(diff / 86400)}j`
}
