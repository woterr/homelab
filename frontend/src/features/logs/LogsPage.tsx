import { useMemo, useState } from 'react'
import { usePoll } from '../../lib/usePoll'
import { api, type LogEntry } from '../../lib/api'
import { PageHead } from '../../components/ui/PageHead'
import { Empty } from '../../components/ui/Feedback'
import { Spinner } from '../../components/ui/Feedback'

type Level = '' | 'debug' | 'info' | 'warn' | 'error'

const LEVELS: Level[] = ['', 'debug', 'info', 'warn', 'error']
const LEVEL_COLOR: Record<string, string> = {
  debug: 'var(--text-tertiary)',
  info: 'var(--accent)',
  warn: 'var(--warn)',
  error: 'var(--danger)',
}

const fmtTime = (iso: string) => {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const hh = String(d.getHours()).padStart(2, '0')
  const mm = String(d.getMinutes()).padStart(2, '0')
  const ss = String(d.getSeconds()).padStart(2, '0')
  return `${hh}:${mm}:${ss}`
}

export function LogsPage() {
  const [logs, setLogs] = useState<LogEntry[] | null>(null)
  const [level, setLevel] = useState<Level>('')

  usePoll(async () => {
    try {
      const entries = await api.logs(500, level || undefined)
      setLogs(entries)
    } catch {
      /* keep last snapshot */
    }
  }, 5000)

  const attrsText = (e: LogEntry) => {
    if (!e.attrs) return ''
    return Object.entries(e.attrs)
      .map(([k, v]) => `${k}=${String(v)}`)
      .join(' ')
  }

  const visible = useMemo(() => logs ?? [], [logs])

  return (
    <div className="stack">
      <PageHead
        index="06"
        title="Logs"
        meta={`${visible.length} BUFFERED LINES · LAST 1000`}
        right={
          <div className="row-actions">
            {LEVELS.map((l) => (
              <button
                key={l}
                type="button"
                className={`cmdlink ${level === l ? 'cmdlink--active' : ''}`}
                style={{ height: '2rem', padding: '0 var(--sp-3)' }}
                onClick={() => setLevel(l)}
              >
                {l === '' ? 'ALL' : l}
              </button>
            ))}
          </div>
        }
      />

      <div className="panel">
        <div className="panel__head">
          <h3 className="panel__title">System log</h3>
          <span className="k-mono k-tert" style={{ fontSize: 'var(--fs-2xs)' }}>
            tail · 5s
          </span>
        </div>
        <div className="panel__body panel__body--flush">
          {logs === null ? (
            <div className="page" style={{ alignItems: 'center', padding: 'var(--sp-12)' }}>
              <Spinner size={24} />
            </div>
          ) : visible.length === 0 ? (
            <Empty title="No log lines" hint="The buffer is empty or filtered out." />
          ) : (
            <div style={{ padding: 'var(--sp-3) var(--sp-4)' }}>
              {visible.map((e, i) => (
                <div className="log-row" key={`${e.time}-${i}`}>
                  <span className="log-row__time" style={{ color: LEVEL_COLOR[e.level] ?? 'var(--text-tertiary)' }}>
                    {fmtTime(e.time)}
                  </span>
                  <span className="k-mono k-faint" style={{ fontSize: 'var(--fs-2xs)', width: '3rem', flexShrink: 0 }}>
                    {e.level.toLowerCase()}
                  </span>
                  <div className="log-row__body">
                    {e.message}
                    {attrsText(e) && (
                      <span className="k-mono" style={{ color: 'var(--text-tertiary)', fontSize: 'var(--fs-xs)', marginLeft: 'var(--sp-3)' }}>
                        {attrsText(e)}
                      </span>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}