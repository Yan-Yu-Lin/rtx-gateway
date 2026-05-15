export function formatNumber(value: number | null | undefined): string {
  return new Intl.NumberFormat('en-US').format(value ?? 0)
}

export function formatPercent(value: number): string {
  return `${(value * 100).toFixed(1)}%`
}

export function formatDateTime(value?: string): string {
  if (!value) return 'never'
  return new Intl.DateTimeFormat('en-US', {
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value))
}

export function formatDuration(ms?: number): string {
  if (ms === undefined || ms === null) return 'n/a'
  if (ms < 1000) return `${ms} ms`
  return `${(ms / 1000).toFixed(1)} s`
}

export function startOfToday(): Date {
  const value = new Date()
  value.setHours(0, 0, 0, 0)
  return value
}

export function daysAgo(days: number): Date {
  const value = new Date()
  value.setDate(value.getDate() - days)
  value.setHours(0, 0, 0, 0)
  return value
}

export function tomorrow(): Date {
  const value = startOfToday()
  value.setDate(value.getDate() + 1)
  return value
}
