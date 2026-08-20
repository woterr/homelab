import type { ReactNode } from 'react'
import type { Tone } from '../../lib/status'

interface BadgeProps {
  tone?: Tone
  dot?: boolean
  children: ReactNode
}

const DOT_TONES: Record<Tone, string> = {
  neutral: '',
  accent: 'status__dot--accent',
  ok: 'status__dot--ok',
  warn: 'status__dot--warn',
  danger: 'status__dot--danger',
  info: 'status__dot--info',
  violet: 'status__dot--accent',
}

export function Badge({ tone = 'neutral', dot, children }: BadgeProps) {
  return (
    <span className={`status status--${tone}`}>
      {dot && <span className={`status__dot ${DOT_TONES[tone]}`} />}
      {children}
    </span>
  )
}