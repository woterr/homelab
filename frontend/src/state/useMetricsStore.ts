import { create } from 'zustand'
import { api, type SeriesMeta, type QueryResult, type MetricPoint } from '../lib/api'

interface SeriesCache {
  ts: number
  points: MetricPoint[]
}

interface MetricsState {
  series: SeriesMeta[]
  loadingSeries: boolean
  seriesError: string | null
  cache: Record<string, SeriesCache>
  live: Record<string, number>
  refreshSeries: () => Promise<void>
  query: (key: string, start: number, end: number, resolution: string) => Promise<MetricPoint[]>
  applyLive: (samples: { s: string; v: number }[]) => void
}

export const useMetricsStore = create<MetricsState>((set, get) => ({
  series: [],
  loadingSeries: false,
  seriesError: null,
  cache: {},
  live: {},
  refreshSeries: async () => {
    set({ loadingSeries: true })
    try {
      const series = await api.series()
      set({ series, loadingSeries: false, seriesError: null })
    } catch (e) {
      set({ loadingSeries: false, seriesError: e instanceof Error ? e.message : 'failed' })
    }
  },
  query: async (key, start, end, resolution) => {
    const cacheKey = `${key}|${start}|${end}|${resolution}`
    const hit = get().cache[cacheKey]
    if (hit) return hit.points
    const res: QueryResult = await api.query(key, start, end, resolution)
    const points = res.points ?? []
    set((s) => ({
      cache: { ...s.cache, [cacheKey]: { ts: Date.now(), points } },
    }))
    return points
  },
  applyLive: (samples) => {
    if (samples.length === 0) return
    const live = { ...get().live }
    for (const s of samples) live[s.s] = s.v
    set({ live })
  },
}))