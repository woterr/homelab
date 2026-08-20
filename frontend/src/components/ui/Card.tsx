import type { ReactNode } from 'react'

interface CardProps {
  title?: ReactNode
  actions?: ReactNode
  children: ReactNode
  bodyClassName?: string
  className?: string
  flush?: boolean
  brackets?: boolean
  inset?: boolean
}

export function Card({
  title,
  actions,
  children,
  bodyClassName = '',
  className = '',
  flush,
  brackets,
  inset,
}: CardProps) {
  const cls = ['panel']
  if (brackets) cls.push('panel--brackets')
  if (inset) cls.push('panel--inset')
  if (className) cls.push(className)
  return (
    <section className={cls.join(' ')}>
      {title && (
        <header className="panel__head">
          <h3 className="panel__title">{title}</h3>
          {actions}
        </header>
      )}
      <div className={`${flush ? 'panel__body--flush' : 'panel__body'} ${bodyClassName}`}>{children}</div>
    </section>
  )
}