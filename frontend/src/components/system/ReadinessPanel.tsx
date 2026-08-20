import { useMemo } from 'react'

interface Sub {
  label: string
  score: number
  note: string
}

function classify(score: number): { label: string; cls: string } {
  if (score >= 0.95) return { label: 'NOMINAL', cls: 'status--ok' }
  if (score >= 0.6) return { label: 'CAUTION', cls: 'status--warn' }
  return { label: 'DEGRADED', cls: 'status--danger' }
}

export interface ReadinessData {
  subs: Sub[]
}

export function ReadinessPanel({ subs }: ReadinessData) {
  const pct = useMemo(() => {
    if (subs.length === 0) return 0
    return Math.round((subs.reduce((a, s) => a + s.score, 0) / subs.length) * 100)
  }, [subs])

  const segs = 20
  const on = Math.round((pct / 100) * segs)

  return (
    <div className="readiness">
      <div className="readiness__head">
        <div>
          <div className="k-label">System Readiness</div>
          <div className="tel__value" style={{ marginTop: 2 }}>{pct}%</div>
        </div>
        <div className="seg-meter" style={{ width: 96 }}>
          {Array.from({ length: segs }).map((_, i) => (
            <span
              key={i}
              className={`seg-meter__seg ${i < on ? 'seg-meter__seg--on' : ''} ${
                i < on ? (pct >= 90 ? 'ok' : pct >= 70 ? 'warn' : 'danger') : ''
              }`}
              style={{ width: 3 }}
            />
          ))}
        </div>
      </div>
      <div className="readiness__subs">
        {subs.map((s) => {
          const c = classify(s.score)
          return (
            <div className="readiness__row" key={s.label}>
              <span className="readiness__label">{s.label}</span>
              <span className="readiness__meter">
                <span
                  className="readiness__meter-fill"
                  style={{ width: `${Math.round(s.score * 100)}%` }}
                />
              </span>
              <span className={`status ${c.cls}`} style={{ width: 68 }}>
                {c.label}
              </span>
              <span className="k-faint" style={{ fontFamily: 'var(--font-mono)', fontSize: 10, width: 92, textAlign: 'right' }}>
                {s.note}
              </span>
            </div>
          )
        })}
      </div>
    </div>
  )
}