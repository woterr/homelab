// series.ts — series key parsing, grouping and color assignment.

import type { SeriesMeta } from './api'

export interface ParsedKey {
  base: string
  labels: Record<string, string>
}

export function parseKey(key: string): ParsedKey {
  if (!key) return { base: '', labels: {} }
  const i = key.indexOf('{')
  if (i < 0) return { base: key, labels: {} }
  const base = key.slice(0, i)
  const labels: Record<string, string> = {}
  const close = key.lastIndexOf('}')
  const body = close > i ? key.slice(i + 1, close) : ''
  for (const pair of body.split(',')) {
    const eq = pair.indexOf('=')
    if (eq > 0) {
      labels[pair.slice(0, eq)] = pair.slice(eq + 1).replace(/"/g, '')
    }
  }
  return { base, labels }
}

export interface SeriesGroup {
  base: string
  key: string
  kind: 'gauge' | 'counter' | 'derived' | 'unknown'
  min: number
  max: number
  samples: number
  variants: SeriesMeta[]
}

// Group series by base name, so `temperature{sensor="cpu-thermal"}` and
// `temperature{sensor="ssd"}` collapse into one chartable group.
export function groupSeries(list: SeriesMeta[]): SeriesGroup[] {
  const map = new Map<string, SeriesGroup>()
  for (const s of list) {
    const { base } = parseKey(s.name)
    let g = map.get(base)
    if (!g) {
      g = {
        base,
        key: base,
        kind: inferKind(base),
        min: s.min,
        max: s.max,
        samples: 0,
        variants: [],
      }
      map.set(base, g)
    }
    g.samples += s.samples
    g.min = Math.min(g.min, s.min)
    g.max = Math.max(g.max, s.max)
    g.variants.push(s)
  }
  const out = [...map.values()]
  out.sort((a, b) => a.base.localeCompare(b.base))
  return out
}

export function inferKind(base: string): SeriesGroup['kind'] {
  if (/(_total|_count|_bytes?$)/.test(base)) return 'counter'
  if (/^(rx|tx)_/.test(base)) return 'counter'
  if (/(_pct|_percent|load|temp|used|available|utilization|speed)/.test(base)) return 'gauge'
  return 'gauge'
}

// A stable, readable label for a labeled variant, e.g. `cpu.utilization{core="0"}` → `core=0`.
export function variantLabel(key: string): string {
  const { labels } = parseKey(key)
  if (Object.keys(labels).length === 0) return 'total'
  return Object.entries(labels)
    .map(([k, v]) => `${k}=${v}`)
    .join(' ')
}

const PALETTE = [
  'var(--chart-0)',
  'var(--chart-1)',
  'var(--chart-2)',
  'var(--chart-3)',
  'var(--chart-4)',
  'var(--chart-5)',
  'var(--chart-6)',
  'var(--chart-7)',
]

export function seriesColor(seed: string): string {
  let h = 0
  for (let i = 0; i < seed.length; i++) h = (h * 31 + seed.charCodeAt(i)) >>> 0
  return PALETTE[h % PALETTE.length]
}

// Number of samples to display for a given time window.
export function rangeStep(windowMs: number): number {
  if (windowMs <= 5 * 60_000) return 15_000
  if (windowMs <= 30 * 60_000) return 30_000
  if (windowMs <= 2 * 3_600_000) return 60_000
  if (windowMs <= 6 * 3_600_000) return 5 * 60_000
  if (windowMs <= 24 * 3_600_000) return 15 * 60_000
  return 60 * 60_000
}