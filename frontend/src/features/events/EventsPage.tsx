import { useMemo, useState } from 'react'
import { useEventsStore } from '../../state/useEventsStore'
import { usePoll } from '../../lib/usePoll'
import { useLive } from '../../lib/useLive'
import { Badge } from '../../components/ui/Badge'
import { Seg } from '../../components/ui/Inputs'
import { Empty, SkeletonRows } from '../../components/ui/Feedback'
import { PageHead } from '../../components/ui/PageHead'
import { fmtDateTime, severityTone } from '../../lib/format'

const SEVERITIES = [
  { value: 'all', label: 'All' },
  { value: 'info', label: 'Info' },
  { value: 'warning', label: 'Warn' },
  { value: 'error', label: 'Error' },
] as const

export function EventsPage() {
  const events = useEventsStore((s) => s.events)
  const refresh = useEventsStore((s) => s.refresh)
  const [sev, setSev] = useState<string>('all')

  useLive([])
  usePoll(() => refresh(500), 15_000)

  const filtered = useMemo(() => {
    if (sev === 'all') return events
    if (sev === 'error') return events.filter((e) => e.severity === 'error' || e.severity === 'critical')
    return events.filter((e) => e.severity === sev)
  }, [events, sev])

  const counts = useMemo(() => {
    const c = { all: events.length, info: 0, warning: 0, error: 0 }
    for (const e of events) {
      if (e.severity === 'error' || e.severity === 'critical') c.error++
      else if (e.severity === 'warning') c.warning++
      else c.info++
    }
    return c
  }, [events])

  return (
    <div className="stack">
      <PageHead
        index="07"
        title="Events"
        meta={`${counts.all} TOTAL · ${counts.error} ERR · ${counts.warning} WARN`}
        right={
          <Seg value={sev} onChange={setSev} options={[...SEVERITIES]} />
        }
      />

      <div className="panel">
        {events.length === 0 ? (
          <SkeletonRows rows={5} />
        ) : filtered.length === 0 ? (
          <Empty title="No matching events" hint="Try a wider severity filter." />
        ) : (
          <div className="panel__body--flush timeline">
            <div className="timeline__line" />
            {filtered.map((e) => (
              <div className="event-row" key={e.id}>
                <div className={`event-row__dot event-row__dot--${severityTone(e.severity)}`} />
                <span className="event-row__time">{fmtDateTime(e.ts)}</span>
                <div className="event-row__body">
                  <div className="event-row__msg">{e.message}</div>
                  <div className="event-row__meta">
                    <span>{e.source}</span>
                    <span className="k-mono">{e.type}</span>
                  </div>
                </div>
                <Badge tone={severityTone(e.severity) as 'info' | 'ok' | 'warn' | 'danger'}>
                  {e.severity}
                </Badge>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}