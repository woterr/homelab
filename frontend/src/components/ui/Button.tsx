import type { ButtonHTMLAttributes, ReactNode } from 'react'

type Variant = 'primary' | 'ghost' | 'danger' | 'default'
type Size = 'sm' | 'md'

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant
  size?: Size
  icon?: ReactNode
  children?: ReactNode
}

export function Button({
  variant = 'default',
  size = 'md',
  icon,
  children,
  className = '',
  ...rest
}: ButtonProps) {
  const cls = ['btn']
  if (variant !== 'default') cls.push(`btn--${variant}`)
  if (size === 'sm') cls.push('btn--sm')
  if (icon && !children) cls.push('btn--icon')
  if (className) cls.push(className)
  return (
    <button className={cls.join(' ')} {...rest}>
      {icon}
      {children}
    </button>
  )
}