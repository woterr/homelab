export type Tone = 'ok' | 'warn' | 'danger' | 'info' | 'accent' | 'violet' | 'neutral'

export function statusTone(status: string): Tone {
  switch (status) {
    case 'running':
    case 'active':
    case 'online':
    case 'ok':
    case 'healthy':
      return 'ok'
    case 'failed':
    case 'exited':
    case 'dead':
    case 'offline':
    case 'critical':
      return 'danger'
    case 'restarting':
    case 'starting':
    case 'activating':
    case 'degraded':
    case 'warning':
      return 'warn'
    case 'paused':
    case 'sleeping':
    case 'inactive':
      return 'neutral'
    default:
      return 'info'
  }
}