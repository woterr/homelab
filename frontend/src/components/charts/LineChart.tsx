import { useEffect, useMemo, useRef, useState } from 'react'
import {
  timeFormat as d3TimeFormat,
  scaleTime,
  scaleLinear,
  line as d3Line,
  area as d3Area,
  curveMonotoneX,
  extent,
} from 'd3'

export interface ChartSeries {
  key: string
  label: string
  color: string
  points: { ts: number; value: number }[]
}

export interface Threshold {
  value: number
  label?: string
  color?: string
  fill?: boolean
}

interface LineChartProps {
  series: ChartSeries[]
  thresholds?: Threshold[]
  height?: number
  unit?: string
  showPoints?: boolean
  yTicks?: number
}

interface Margins {
  top: number
  right: number
  bottom: number
  left: number
}

const DIM: Margins = { top: 14, right: 16, bottom: 22, left: 44 }

function useMeasure() {
  const ref = useRef<HTMLDivElement>(null)
  const [size, setSize] = useState({ width: 0, height: 0 })
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const ro = new ResizeObserver((entries) => {
      const r = entries[0].contentRect
      setSize({ width: r.width, height: r.height })
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  return [ref, size] as const
}

const fmtTime = d3TimeFormat('%b %d %H:%M:%S')

export function LineChart({
  series,
  thresholds = [],
  height = 240,
  unit = '',
  showPoints = false,
  yTicks = 4,
}: LineChartProps) {
  const [ref, { width }] = useMeasure()
  const [hover, setHover] = useState<{ x: number; y: number; cx: number; cy: number } | null>(null)
  const svgRef = useRef<SVGSVGElement>(null)
  const rectRef = useRef<DOMRect | null>(null)
  const h = height

  const data = useMemo(() => {
    const all = series.flatMap((s) => s.points.map((p) => p.value))
    const times = series.flatMap((s) => s.points.map((p) => p.ts))
    const tMin = times.length ? Math.min(...times) : Date.now() - 60_000
    const tMax = times.length ? Math.max(...times) : Date.now()
    const vMin = all.length ? Math.min(...all, ...thresholds.map((t) => t.value)) : 0
    const vMax = all.length ? Math.max(...all, ...thresholds.map((t) => t.value)) : 1
    return { tMin, tMax, vMin, vMax }
  }, [series, thresholds])

  const [lo, hi] = useMemo(() => {
    let lo = data.vMin
    let hi = data.vMax
    const pad = Math.max((hi - lo) * 0.1, hi * 0.02, 1e-9)
    lo = lo - pad
    hi = hi + pad
    if (hi - lo < 1e-6) hi = lo + 1
    return [lo, hi]
  }, [data.vMin, data.vMax])

  const x = useMemo(
    () => scaleTime().domain([data.tMin, data.tMax]).range([DIM.left, width - DIM.right]),
    [width, data.tMin, data.tMax],
  )
  const y = useMemo(
    () => scaleLinear().domain([lo, hi]).range([h - DIM.bottom, DIM.top]),
    [h, lo, hi],
  )

  const line = useMemo(() => {
    const gen = d3Line<{ ts: number; value: number }>()
      .x((d) => x(d.ts))
      .y((d) => y(d.value))
      .curve(curveMonotoneX)
    return gen
  }, [x, y])

  const area = useMemo(
    () =>
      d3Area<{ ts: number; value: number }>()
        .x((d) => x(d.ts))
        .y0(h - DIM.bottom)
        .y1((d) => y(d.value))
        .curve(curveMonotoneX),
    [x, y, h],
  )

  const xTicks = useMemo(
    () =>
      x.ticks(Math.max(2, Math.floor(width / 110))).map((t) => ({
        v: t.getTime(),
        label: d3TimeFormat('%H:%M')(t),
      })),
    [x, width],
  )
  const yTicksVals = useMemo(() => y.ticks(yTicks), [y, yTicks])

  const nearest = useMemo(() => {
    if (!hover || series.length === 0) return null
    const s0 = series[0]
    const t = x.invert(hover.x).getTime()
    let best = -1
    let bestD = Infinity
    for (let i = 0; i < s0.points.length; i++) {
      const d = Math.abs(s0.points[i].ts - t)
      if (d < bestD) {
        bestD = d
        best = i
      }
    }
    const p = best >= 0 ? s0.points[best] : null
    if (!p) return null
    return { t: p.ts, val: p.value, x: x(p.ts), y: y(p.value) }
  }, [hover, x, series, y])

  const areaGradId = `area-${series.map((s) => s.key).join('-')}`

  const tooltipVals = useMemo(() => {
    if (!nearest || series.length === 0) return null
    return series.map((s) => {
      // Match each series to its own nearest point (series may share a time
      // axis but not identical timestamps after downsampling).
      let best = -1
      let bestD = Infinity
      for (let i = 0; i < s.points.length; i++) {
        const d = Math.abs(s.points[i].ts - nearest.t)
        if (d < bestD) {
          bestD = d
          best = i
        }
      }
      return best >= 0 ? { label: s.label, value: s.points[best].value, color: s.color } : null
    })
  }, [nearest, series])

  // Tooltip pinned to the hovered data point in screen space (fixed, so it is
  // not clipped by card overflow), instead of floating at the cursor.
  const tipPos = useMemo(() => {
    if (!hover || !nearest) return null
    const rect = rectRef.current
    if (!rect) return null
    const tipW = 200
    const sx = rect.left + nearest.x
    const sy = rect.top + nearest.y
    const left = sx + 12 + tipW > window.innerWidth ? Math.max(8, sx - tipW - 12) : sx + 12
    const top = Math.min(Math.max(8, sy - 16), window.innerHeight - 90)
    return { left, top }
  }, [hover, nearest])

  return (
    <div ref={ref} style={{ width: '100%', height: h, position: 'relative' }}>
      {width > 0 && (
        <svg
          ref={svgRef}
          width={width}
          height={h}
          onMouseMove={(e) => {
            const rect = svgRef.current!.getBoundingClientRect()
            rectRef.current = rect
            setHover({ x: e.clientX - rect.left, y: e.clientY - rect.top, cx: e.clientX, cy: e.clientY })
          }}
          onMouseLeave={() => setHover(null)}
          style={{ display: 'block', cursor: 'crosshair' }}
        >
          <defs>
            {series.length === 1 && (
              <linearGradient id={areaGradId} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={series[0].color} stopOpacity="0.16" />
                <stop offset="100%" stopColor={series[0].color} stopOpacity="0" />
              </linearGradient>
            )}
          </defs>

          {/* grid */}
          {yTicksVals.map((t) => (
            <line
              key={t}
              x1={DIM.left}
              x2={width - DIM.right}
              y1={y(t)}
              y2={y(t)}
              stroke="var(--line-faint)"
              strokeDasharray="2 5"
            />
          ))}
          {xTicks.map((t) => (
            <line
              key={t.v}
              x1={x(t.v)}
              x2={x(t.v)}
              y1={DIM.top}
              y2={h - DIM.bottom}
              stroke="var(--line-faint)"
              strokeDasharray="2 5"
            />
          ))}

          {/* threshold bands */}
          {thresholds.map((t, i) => {
            const yv = y(t.value)
            return (
              <g key={i}>
                {t.fill && (
                  <rect
                    x={DIM.left}
                    y={yv}
                    width={width - DIM.left - DIM.right}
                    height={h - DIM.bottom - yv}
                    fill={(t.color ?? 'var(--danger)').replace(')', '')}
                    opacity={0.05}
                  />
                )}
                <line
                  x1={DIM.left}
                  x2={width - DIM.right}
                  y1={yv}
                  y2={yv}
                  stroke={t.color ?? 'var(--danger)'}
                  strokeWidth={1}
                  strokeDasharray="6 4"
                  opacity={0.55}
                />
                {t.label && (
                  <text
                    x={width - DIM.right - 4}
                    y={yv - 5}
                    textAnchor="end"
                    style={{ fontFamily: 'var(--font-mono)', fontSize: 9 }}
                    fill={t.color ?? 'var(--danger)'}
                    opacity={0.8}
                  >
                    {t.label.toUpperCase()}
                  </text>
                )}
              </g>
            )
          })}

          {/* areas + lines with draw-in animation */}
          {series.length === 1 && series[0].points.length > 1 && (
            <path d={area(series[0].points) ?? undefined} fill={`url(#${areaGradId})`} />
          )}
          {series.map((s) => (
            <g key={s.key}>
              {s.points.length > 1 && (
                <path
                  d={line(s.points) ?? undefined}
                  fill="none"
                  stroke={s.color}
                  strokeWidth={1.6}
                  strokeLinejoin="round"
                  strokeLinecap="round"
                  pathLength={1}
                  className="draw-line"
                  style={{ filter: `drop-shadow(0 0 3px ${s.color})`, animationDelay: '120ms' }}
                />
              )}
              {showPoints &&
                s.points.map((p, i) => (
                  <circle key={i} cx={x(p.ts)} cy={y(p.value)} r={2} fill={s.color} opacity={0.85} />
                ))}
            </g>
          ))}

          {/* last value marker — precision square */}
          {series.map((s) => {
            const last = s.points[s.points.length - 1]
            if (!last) return null
            const xv = x(last.ts)
            const yv = y(last.value)
            return (
              <g key={`${s.key}-last`}>
                <rect
                  x={xv - 4}
                  y={yv - 4}
                  width={8}
                  height={8}
                  fill={s.color}
                  stroke="var(--bg-panel)"
                  strokeWidth={1.5}
                  style={{ filter: `drop-shadow(0 0 4px ${s.color})` }}
                />
              </g>
            )
          })}

          {/* axes */}
          <g
            className="x-axis"
            transform={`translate(0, ${h - DIM.bottom})`}
            style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-tertiary)' }}
          >
            <line x1={0} x2={width - DIM.left - DIM.right} stroke="var(--line-strong)" strokeWidth={1} />
            {xTicks.map((t) => (
              <text key={t.v} x={x(t.v) - DIM.left} y={13} textAnchor="middle" fill="var(--text-tertiary)">
                {t.label}
              </text>
            ))}
          </g>
          <g
            className="y-axis"
            transform={`translate(${DIM.left}, 0)`}
            style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-tertiary)' }}
          >
            <line x1={0} x2={0} y1={DIM.top} y2={h - DIM.bottom} stroke="var(--line-strong)" strokeWidth={1} />
            {yTicksVals.map((t) => (
              <text key={t} x={-6} y={y(t) + 3} textAnchor="end" fill="var(--text-tertiary)">
                {fmtTick(t)}
              </text>
            ))}
          </g>

          {/* crosshair */}
          {nearest && tooltipVals && (
            <g>
              <line
                x1={nearest.x}
                x2={nearest.x}
                y1={DIM.top}
                y2={h - DIM.bottom}
                stroke="var(--line-strong)"
                strokeDasharray="3 3"
              />
              <circle
                cx={nearest.x}
                cy={nearest.y}
                r={4}
                fill={series[0].color}
                stroke="var(--bg-panel)"
                strokeWidth={2}
              />
            </g>
          )}
        </svg>
      )}

      {/* tooltip pinned to the nearest data point in screen space */}
      {hover && nearest && tooltipVals && tipPos && (
        <div
          className="tooltip"
          style={{
            left: tipPos.left,
            top: tipPos.top,
          }}
        >
          <div className="tooltip__title">{fmtTime(new Date(nearest.t))}</div>
          {tooltipVals.map(
            (v, i) =>
              v && (
                <div className="tooltip__row" key={i}>
                  <span style={{ width: 8, height: 3, background: v.color, display: 'inline-block' }} />
                  <span>{v.label}</span>
                  <b>
                    {fmtTick(v.value)}
                    {unit && <span className="k-tert"> {unit}</span>}
                  </b>
                </div>
              ),
          )}
        </div>
      )}
    </div>
  )
}

function fmtTick(v: number): string {
  if (Math.abs(v) >= 1000) return v.toFixed(0)
  if (Math.abs(v) >= 100) return v.toFixed(1)
  if (Math.abs(v) >= 1) return v.toFixed(1)
  return v.toFixed(2)
}

export function Sparkline({ points, color = 'var(--accent)', height = 36, unit }: { points: { ts: number; value: number }[]; color?: string; height?: number; unit?: string }) {
  const [ref, { width }] = useMeasure()
  const [hover, setHover] = useState<{ x: number; cx: number; cy: number } | null>(null)
  const rectRef = useRef<DOMRect | null>(null)
  const h = height

  const x = useMemo(
    () => scaleTime().domain(extent(points, (d) => d.ts) as [number, number]).range([0, width]),
    [points, width],
  )
  const y = useMemo(() => {
    const [lo, hi] = extent(points, (d) => d.value) as [number, number]
    const pad = Math.max((hi - lo) * 0.15, 1e-9)
    return scaleLinear().domain([lo - pad, hi + pad]).range([h, 2])
  }, [points, h])

  const nearest = useMemo(() => {
    if (!hover || points.length === 0) return null
    const t = x.invert(hover.x).getTime()
    let best = -1
    let bestD = Infinity
    for (let i = 0; i < points.length; i++) {
      const d = Math.abs(points[i].ts - t)
      if (d < bestD) {
        bestD = d
        best = i
      }
    }
    return best >= 0 ? { i: best, x: x(points[best].ts), y: y(points[best].value), v: points[best].value } : null
  }, [hover, points, x, y])

  const tipPos = useMemo(() => {
    if (!hover || !nearest) return null
    const rect = rectRef.current
    if (!rect) return null
    const tipW = 160
    const sx = rect.left + nearest.x
    const sy = rect.top + nearest.y
    const left = sx + 10 + tipW > window.innerWidth ? Math.max(8, sx - tipW - 10) : sx + 10
    const top = Math.min(Math.max(8, sy - 30), window.innerHeight - 70)
    return { left, top }
  }, [hover, nearest])

  if (points.length === 0 || width === 0) {
    return <div ref={ref} style={{ height: h }} />
  }

  const l = d3Line<{ ts: number; value: number }>()
    .x((d) => x(d.ts))
    .y((d) => y(d.value))
    .curve(curveMonotoneX)
  const a = d3Area<{ ts: number; value: number }>()
    .x((d) => x(d.ts))
    .y0(h)
    .y1((d) => y(d.value))
    .curve(curveMonotoneX)

  return (
    <div ref={ref} style={{ width: '100%', height: h, position: 'relative' }}>
      <svg
        width={width}
        height={h}
        onMouseMove={(e) => {
          const rect = (e.currentTarget as SVGSVGElement).getBoundingClientRect()
          rectRef.current = rect
          setHover({ x: e.clientX - rect.left, cx: e.clientX, cy: e.clientY })
        }}
        onMouseLeave={() => setHover(null)}
        style={{ display: 'block', cursor: 'crosshair' }}
      >
        <defs>
          <linearGradient id={`spark-${color.replace(/\W/g, '')}`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={color} stopOpacity="0.22" />
            <stop offset="100%" stopColor={color} stopOpacity="0" />
          </linearGradient>
        </defs>
        <path d={a(points) ?? undefined} fill={`url(#spark-${color.replace(/\W/g, '')})`} />
        <path d={l(points) ?? undefined} fill="none" stroke={color} strokeWidth={1.5} strokeLinejoin="round" />
        {nearest && (
          <line x1={nearest.x} x2={nearest.x} y1={0} y2={h} stroke="var(--line-strong)" strokeDasharray="2 3" />
        )}
      </svg>
      {hover && nearest && tipPos && (
        <div
          className="tooltip"
          style={{
            left: tipPos.left,
            top: tipPos.top,
          }}
        >
          <div className="tooltip__title">{fmtTime(new Date(points[nearest.i].ts))}</div>
          <div className="tooltip__row">
            <span style={{ width: 8, height: 3, background: color, display: 'inline-block' }} />
            <span>value</span>
            <b>
              {fmtTick(nearest.v)}
              {unit && <span className="k-tert"> {unit}</span>}
            </b>
          </div>
        </div>
      )}
    </div>
  )
}