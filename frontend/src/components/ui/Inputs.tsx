import { type InputHTMLAttributes, type SelectHTMLAttributes, type ReactNode } from 'react'

interface FieldProps {
  label?: ReactNode
  hint?: ReactNode
  children: ReactNode
}

export function Field({ label, hint, children }: FieldProps) {
  return (
    <label className="field">
      {label && <span className="field__label">{label}</span>}
      {children}
      {hint && <span className="text-tertiary" style={{ fontSize: 'var(--fs-xs)' }}>{hint}</span>}
    </label>
  )
}

export function Input(props: InputHTMLAttributes<HTMLInputElement>) {
  return <input className="input" {...props} />
}

export function Select(props: SelectHTMLAttributes<HTMLSelectElement>) {
  return <select className="select" {...props} />
}

interface SwitchProps {
  checked: boolean
  onChange: (v: boolean) => void
  label?: string
  disabled?: boolean
}

export function Switch({ checked, onChange, label, disabled }: SwitchProps) {
  return (
    <span
      className="field__label"
      style={{ display: 'inline-flex', alignItems: 'center', gap: 'var(--sp-2)', cursor: 'pointer' }}
      onClick={() => !disabled && onChange(!checked)}
    >
      <span className="switch">
        <input type="checkbox" checked={checked} readOnly tabIndex={-1} />
        <span className="switch__track">
          <span className="switch__thumb" />
        </span>
      </span>
      {label}
    </span>
  )
}

interface SegProps<T extends string> {
  value: T
  onChange: (v: T) => void
  options: { value: T; label: ReactNode }[]
}

export function Seg<T extends string>({ value, onChange, options }: SegProps<T>) {
  return (
    <div className="seg" role="tablist">
      {options.map((o) => (
        <button
          key={o.value}
          role="tab"
          aria-selected={o.value === value}
          onClick={() => onChange(o.value)}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}