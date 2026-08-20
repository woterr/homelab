// format.ts — number, byte and time formatting helpers.

const UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']

export function fmtBytes(n: number, digits = 1): string {
  if (!Number.isFinite(n) || n < 0) return '—'
  if (n === 0) return '0 B'
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), UNITS.length - 1)
  const val = n / 1024 ** i
  return `${val >= 100 ? Math.round(val) : val.toFixed(digits)} ${UNITS[i]}`
}

export function fmtBytesRate(n: number): string {
  return `${fmtBytes(n)}/s`
}

export function fmtSpeedMbps(mbps: number): string {
  if (!Number.isFinite(mbps) || mbps <= 0) return '—'
  if (mbps >= 1000) {
    const g = mbps / 1000
    return `${mbps % 1000 === 0 ? g.toFixed(0) : g.toFixed(2)} Gbps`
  }
  return `${Math.round(mbps)} Mbps`
}

export function fmtPct(n: number, digits = 1): string {
  if (!Number.isFinite(n)) return '—'
  return `${n.toFixed(digits)}%`
}

export function fmtNumber(n: number, digits = 1): string {
  if (!Number.isFinite(n)) return '—'
  return n.toLocaleString(undefined, {
    maximumFractionDigits: digits,
  })
}

export function fmtTemp(c: number): string {
  if (!Number.isFinite(c)) return '—'
  return `${c.toFixed(1)}°C`
}

export function fmtUptime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return '—'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const parts = []
  if (d > 0) parts.push(`${d}d`)
  if (h > 0) parts.push(`${h}h`)
  parts.push(`${m}m`)
  return parts.join(' ')
}

const timeFmt = new Intl.DateTimeFormat(undefined, {
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
})

const dateFmt = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
})

export function fmtTime(ts: string | number | Date): string {
  return timeFmt.format(new Date(ts))
}

export function fmtDateTime(ts: string | number | Date): string {
  return dateFmt.format(new Date(ts))
}

export function fmtRelative(ts: string | number | Date): string {
  const t = new Date(ts).getTime()
  const diff = Date.now() - t
  if (diff < 0) return 'now'
  const s = Math.floor(diff / 1000)
  if (s < 5) return 'now'
  if (s < 60) return `${s}s ago`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  const d = Math.floor(h / 24)
  return `${d}d ago`
}

export function severityTone(sev: string): string {
  switch (sev) {
    case 'info':
      return 'info'
    case 'success':
      return 'ok'
    case 'warning':
      return 'warn'
    case 'error':
      return 'danger'
    case 'critical':
      return 'danger'
    default:
      return 'neutral'
  }
}

export function toMs(unixSecondsOrMs: number, hasMs: boolean): number {
  return hasMs ? unixSecondsOrMs : unixSecondsOrMs * 1000
}
