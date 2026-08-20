import { useEffect, useMemo, useState } from 'react'
import { useMetricsStore } from '../state/useMetricsStore'

export interface HistoryPoint {
  ts: number
  value: number
}

/**
 * Fetch a time window of a single series and keep it fresh by appending live
 * WebSocket values for that series key.
 */
export function useSeriesHistory(key: string, windowMs: number, resolution: string): HistoryPoint[] {
  const histories = useSeriesHistoryMulti([key], windowMs, resolution)
  return histories[0]
}

/**
 * Fetch time windows of several series in parallel. Returns one point array
 * per key, in the same order. Every key is treated as a top-level hook call,
 * so the number of keys must be stable across renders.
 */
export function useSeriesHistoryMulti(keys: string[], windowMs: number, resolution: string): HistoryPoint[][] {
  const query = useMetricsStore((s) => s.query)
  const live = useMetricsStore((s) => s.live)
  const [base, setBase] = useState<HistoryPoint[][]>([])

  useEffect(() => {
    let active = true
    const end = Date.now()
    const start = end - windowMs
    Promise.all(
      keys.map((key) =>
        query(key, start, end, resolution).then((pts) =>
          pts.map((p) => ({ ts: new Date(p.ts).getTime(), value: p.avg })),
        ),
      ),
    )
      .then((results) => {
        if (active) setBase(results)
      })
      .catch(() => {
        /* handled by callers */
      })
    return () => {
      active = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [keys.join('|'), windowMs, resolution])

  return useMemo(() => {
    if (base.length === 0) return keys.map(() => [])
    return keys.map((key, i) => {
      const b = base[i] ?? []
      const lv = live[key]
      if (lv === undefined) return b
      const lastTs = b.length ? b[b.length - 1].ts : Date.now() - windowMs
      if (lastTs > Date.now() - 4000) return b
      return [...b, { ts: Date.now(), value: lv }]
    })
  }, [base, live, keys, windowMs])
}