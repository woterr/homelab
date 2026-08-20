interface MeterProps {
  value: number
  max?: number
  tone?: 'ok' | 'warn' | 'danger' | 'auto'
  size?: 'sm' | 'lg'
  title?: string
  wave?: boolean
}

function autoTone(pct: number): 'ok' | 'warn' | 'danger' {
  if (pct >= 90) return 'danger'
  if (pct >= 70) return 'warn'
  return 'ok'
}

export function Meter({ value, max = 100, tone = 'auto', size = 'sm', title, wave }: MeterProps) {
  const pct = max > 0 ? Math.max(0, Math.min(100, (value / max) * 100)) : 0
  const t = tone === 'auto' ? autoTone(pct) : tone
  const cls = wave ? 'wave-bar' : 'meter'
  const fill = wave ? 'wave-bar__fill' : `meter__fill meter__fill--${t}`
  return (
    <div
      className={`${cls} ${size === 'lg' ? `${cls}--lg` : ''}`}
      role="progressbar"
      aria-valuenow={Math.round(pct)}
      aria-valuemin={0}
      aria-valuemax={100}
      title={title}
    >
      <div className={fill} style={{ width: `${pct}%` }}>
        <span className={`${cls}__edge`} />
      </div>
    </div>
  )
}