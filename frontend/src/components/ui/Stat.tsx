import type { ReactNode } from 'react'

interface StatProps {
  label: ReactNode
  value: ReactNode
  sub?: ReactNode
  size?: 'md' | 'lg'
  accent?: ReactNode
}

export function Stat({ label, value, sub, size = 'lg', accent }: StatProps) {
  return (
    <div className="tel">
      <div className="tel__label">
        <span>{label}</span>
        {accent}
      </div>
      <div className={`tel__value ${size === 'md' ? 'tel__value--sm' : ''}`}>{value}</div>
      {sub && <div className="tel__sub">{sub}</div>}
    </div>
  )
}