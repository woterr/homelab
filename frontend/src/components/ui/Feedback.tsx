import type { ReactNode } from 'react'

export function Empty({
  icon = '▧',
  title = 'Nothing here yet',
  hint,
  action,
}: {
  icon?: ReactNode
  title?: string
  hint?: string
  action?: ReactNode
}) {
  return (
    <div className="empty">
      <div className="empty__icon">{icon}</div>
      <div style={{ fontWeight: 600 }}>{title}</div>
      {hint && <div style={{ fontSize: 'var(--fs-xs)', maxWidth: '24rem' }}>{hint}</div>}
      {action}
    </div>
  )
}

export function Skeleton({ height = '1rem', width = '100%', style }: { height?: string; width?: string; style?: React.CSSProperties }) {
  return <div className="skeleton" style={{ height, width, ...style }} />
}

export function SkeletonRows({ rows = 4 }: { rows?: number }) {
  return (
    <div className="stack" style={{ padding: 'var(--sp-4)' }}>
      {Array.from({ length: rows }).map((_, i) => (
        <Skeleton key={i} height="2.25rem" />
      ))}
    </div>
  )
}

export function Spinner({ size = 18 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      style={{ animation: 'spin 0.8s linear infinite', color: 'var(--accent)' }}
      aria-label="loading"
    >
      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
      <circle cx="12" cy="12" r="9" stroke="currentColor" strokeOpacity="0.25" strokeWidth="3" />
      <path d="M21 12a9 9 0 0 0-9-9" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
    </svg>
  )
}