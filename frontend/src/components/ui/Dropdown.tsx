import { useEffect, useRef, useState } from 'react'

export interface DropdownOption {
  value: string
  label: string
  group?: string
}

interface DropdownProps {
  value: string | string[]
  options: DropdownOption[]
  onChange: (value: string | string[]) => void
  multiple?: boolean
  placeholder?: string
  empty?: string
}

export function Dropdown({ value, options, onChange, multiple, placeholder = 'Select…', empty = 'Nothing available' }: DropdownProps) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onDoc)
    return () => document.removeEventListener('mousedown', onDoc)
  }, [])

  const opts = Array.isArray(value) ? value : value ? [value] : []
  const selected = options.filter((o) => opts.includes(o.value))
  const label = multiple
    ? selected.length === 0
      ? placeholder
      : selected.map((s) => s.label).join(', ')
    : selected[0]?.label ?? placeholder

  // Preserve options order and group headers.
  const groups = new Map<string, DropdownOption[]>()
  for (const o of options) {
    const g = o.group ?? ''
    if (!groups.has(g)) groups.set(g, [])
    groups.get(g)!.push(o)
  }

  const pick = (v: string) => {
    if (multiple) {
      const arr = Array.isArray(value) ? value : []
      const next = arr.includes(v) ? arr.filter((x) => x !== v) : [...arr, v]
      onChange(next)
    } else {
      onChange(v)
      setOpen(false)
    }
  }

  return (
    <div className="dropdown" ref={ref}>
      <button type="button" className="dropdown__trigger" onClick={() => setOpen((o) => !o)} aria-haspopup="listbox" aria-expanded={open}>
        <span className={`dropdown__value ${opts.length === 0 ? 'dropdown__value--empty' : ''}`}>{label}</span>
        <span className="dropdown__caret" aria-hidden>▾</span>
      </button>
      {open && (
        <div className="dropdown__menu" role="listbox">
          {groups.size === 0 && <div className="dropdown__empty">{empty}</div>}
          {[...groups.entries()].map(([g, items]) => (
            <div key={g || '__root'}>
              {g && <div className="dropdown__group">{g}</div>}
              {items.map((o) => {
                const on = opts.includes(o.value)
                return (
                  <button
                    key={o.value}
                    type="button"
                    role="option"
                    aria-selected={on}
                    className={`dropdown__option ${on ? 'dropdown__option--on' : ''}`}
                    onClick={() => pick(o.value)}
                  >
                    <span className="dropdown__check" aria-hidden>{on ? '✓' : ''}</span>
                    <span className="dropdown__optlabel">{o.label}</span>
                  </button>
                )
              })}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
