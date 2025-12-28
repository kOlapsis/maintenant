/**
 * Schedule formatting utilities for heartbeat monitors.
 */

/**
 * Converts an interval in seconds to a human-readable string.
 * Examples:
 *   30 -> "Every 30 seconds"
 *   60 -> "Every 1 minute"
 *   300 -> "Every 5 minutes"
 *   3600 -> "Every 1 hour"
 *   21600 -> "Every 6 hours"
 *   86400 -> "Every 24 hours"
 */
export function formatInterval(intervalSeconds: number): string {
  if (intervalSeconds <= 0) return 'No interval set'

  if (intervalSeconds < 60) {
    return `Every ${intervalSeconds} second${intervalSeconds !== 1 ? 's' : ''}`
  }

  const minutes = Math.round(intervalSeconds / 60)
  if (minutes < 60) {
    if (minutes === 1) return 'Every minute'
    return `Every ${minutes} minutes`
  }

  const hours = Math.round(intervalSeconds / 3600)
  if (hours < 24) {
    if (hours === 1) return 'Every hour'
    return `Every ${hours} hours`
  }

  const days = Math.round(intervalSeconds / 86400)
  if (days === 1) return 'Every day'
  return `Every ${days} days`
}

/**
 * Computes the next expected ping datetime based on the last ping and interval.
 * @param lastPingAt ISO 8601 timestamp of the last ping
 * @param intervalSeconds Interval in seconds between expected pings
 * @returns Date of the next expected ping
 */
export function nextExpectedPing(lastPingAt: string, intervalSeconds: number): Date {
  const last = new Date(lastPingAt)
  return new Date(last.getTime() + intervalSeconds * 1000)
}
