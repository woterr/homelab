import { useMemo, useState } from 'react'
import { useMetricsStore } from '../../state/useMetricsStore'
import { usePoll } from '../../lib/usePoll'
import { useSeriesHistoryMulti } from '../../lib/useSeriesHistory'
import { Seg } from '../../components/ui/Inputs'
import { Badge } from '../../components/ui/Badge'
import { Empty } from '../../components/ui/Feedback'
import { PageHead } from '../../components/ui/PageHead'
import { LineChart } from '../../components/charts/LineChart'
import { groupSeries, variantLabel, seriesColor, inferKind, rangeStep, type SeriesGroup } from '../../lib/series'
import { fmtNumber } from '../../lib/format'
import { MAX_SELECTED, useTelemetryStore } from '../../state/useTelemetryStore'

const WINDOWS = [
  { label: '1H', ms: 3_600_000 },
  { label: '6H', ms: 6 * 3_600_000 },
  { label: '12H', ms: 12 * 3_600_000 },
  { label: '24H', ms: 24 * 3_600_000 },
  { label: '7D', ms: 7 * 24 * 3_600_000 },
  { label: '30D', ms: 30 * 24 * 3_600_000 },
] as const

function unitFor(base: string): string {
  if (/(temp|temperature|thermal)/.test(base)) return '°C'
  if (/(pct|percent|utilization|used)/.test(base)) return '%'
  if (/(bytes|rx_|tx_|read|write|memory|usage)/.test(base)) return 'B'
  return ''
}

function decimalsFor(base: string): number {
  if (/(load|rx_|tx_|read_bps|write_bps)/.test(base)) return 2
  if (/(temp|pct|percent|utilization|used)/.test(base)) return 1
  return 0
}

function MetricChart({
  group,
  win,
  res,
}: {
  group: SeriesGroup | null
  win: { label: string; ms: number }
  res: number
}) {
  const variantKeys = useMemo(() => (group ? group.variants.map((v) => v.name) : []), [group])
  const histories = useSeriesHistoryMulti(variantKeys, win.ms, `${Math.round(res / 1000)}s`)

  const chartSeries = useMemo(
    () =>
      group
        ? group.variants.map((v, i) => ({
            key: v.name,
            label: variantLabel(v.name),
            color: seriesColor(v.name),
            points: histories[i] ?? [],
          }))
        : [],
    [group, histories],
  )

  if (!group) {
    return (
      <div className="panel metrics-chart">
        <Empty title="Empty slot" hint="Pick a metric from the list to render it here." />
      </div>
    )
  }

  return (
    <div className="panel metrics-chart">
      <div className="panel__head">
        <h3 className="panel__title k-mono" style={{ textTransform: 'none', letterSpacing: '0.02em' }}>
          {group.base}
        </h3>
        <div style={{ display: 'flex', gap: 'var(--sp-2)', flexWrap: 'wrap' }}>
          <Badge tone="accent">{inferKind(group.base)}</Badge>
          {unitFor(group.base) && <Badge tone="info">unit {unitFor(group.base)}</Badge>}
          <Badge tone="neutral">
            min {fmtNumber(group.min, decimalsFor(group.base))} · max {fmtNumber(group.max, decimalsFor(group.base))}
          </Badge>
        </div>
      </div>
      <div className="panel__body">
        <LineChart
          series={chartSeries}
          height={280}
          unit={unitFor(group.base)}
          showPoints={win.ms <= 6 * 3_600_000}
        />
        {chartSeries.length > 1 && (
          <div className="chart-legend">
            {chartSeries.map((s) => (
              <span key={s.key} className="chart-legend__item">
                <span className="chart-legend__swatch" style={{ background: s.color }} />
                {s.label}
              </span>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

export function MetricsPage() {
  const series = useMetricsStore((s) => s.series)
  const refreshSeries = useMetricsStore((s) => s.refreshSeries)
  const selected = useTelemetryStore((s) => s.selected)
  const toggle = useTelemetryStore((s) => s.toggle)
  const [windowIdx, setWindowIdx] = useState(0)

  usePoll(() => refreshSeries(), 60_000)

  const groups = useMemo(() => groupSeries(series), [series])
  const win = WINDOWS[windowIdx]
  const res = rangeStep(win.ms)

  const slot0 = groups.find((g) => g.base === selected[0]) ?? null
  const slot1 = groups.find((g) => g.base === selected[1]) ?? null

  return (
    <div className="stack">
      <PageHead
        index="04"
        title="Telemetry"
        meta={`${series.length} SERIES · UP TO ${MAX_SELECTED} PLOTTED`}
        right={
          <div style={{ display: 'flex', gap: 'var(--sp-3)', alignItems: 'center' }}>
            <span className="k-mono k-tert" style={{ fontSize: 'var(--fs-2xs)' }}>
              {selected.length}/{MAX_SELECTED} SELECTED
            </span>
            <Seg
              value={String(windowIdx)}
              onChange={(v) => setWindowIdx(Number(v))}
              options={WINDOWS.map((w, i) => ({ value: String(i), label: w.label }))}
            />
          </div>
        }
      />

      <div className="metrics-grid">
        <div className="panel metrics-grid__list">
          <div className="panel__head">
            <h3 className="panel__title">Series</h3>
            <span className="k-mono k-tert" style={{ fontSize: 10 }}>{groups.length}</span>
          </div>
          <div className="panel__body--flush metrics-list">
            {groups.length === 0 ? (
              <Empty title="No series yet" hint="Metrics will appear once the collector has ticked a few times." />
            ) : (
              groups.map((g) => {
                const checked = selected.includes(g.base)
                return (
                  <button
                    key={g.base}
                    onClick={() => toggle(g.base)}
                    className={`metrics-item ${checked ? 'metrics-item--active' : ''}`}
                    role="checkbox"
                    aria-checked={checked}
                  >
                    <span className={`metrics-item__check ${checked ? 'metrics-item__check--on' : ''}`}>
                      {checked && <span>✓</span>}
                    </span>
                    <span className="metrics-item__swatch" style={{ background: seriesColor(g.base) }} />
                    <span className="metrics-item__body">
                      <span className="metrics-item__name k-truncate">{g.base}</span>
                      <span className="metrics-item__sub">
                        {g.variants.length} variant{g.variants.length > 1 ? 's' : ''} · {fmtNumber(g.samples, 0)} pts
                      </span>
                    </span>
                  </button>
                )
              })
            )}
          </div>
        </div>

        <div className="metrics-grid__chart" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--sp-4)', minWidth: 0 }}>
          <MetricChart group={slot0} win={win} res={res} />
          <MetricChart group={slot1} win={win} res={res} />
        </div>
      </div>
    </div>
  )
}