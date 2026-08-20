import { useToastStore } from '../../state/useToastStore'
import { IconX } from './icons'

export function ToastStack() {
  const toasts = useToastStore((s) => s.toasts)
  const dismiss = useToastStore((s) => s.dismiss)

  return (
    <div className="toast-stack">
      {toasts.map((t) => (
        <div key={t.id} className={`toast toast--${t.kind}`}>
          <div style={{ flex: 1 }}>
            <strong style={{ fontSize: 'var(--fs-sm)' }}>{t.title}</strong>
            {t.message && <div className="text-secondary" style={{ fontSize: 'var(--fs-xs)', marginTop: 2 }}>{t.message}</div>}
          </div>
          <button onClick={() => dismiss(t.id)} aria-label="Dismiss" style={{ color: 'var(--text-tertiary)' }}>
            <IconX width={14} height={14} />
          </button>
        </div>
      ))}
    </div>
  )
}