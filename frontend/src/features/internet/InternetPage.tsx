import { useEffect, useState } from 'react'
import { useNetworkStore } from '../../state/useNetworkStore'
import { usePoll } from '../../lib/usePoll'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { PageHead } from '../../components/ui/PageHead'
import { Sparkline } from '../../components/charts/LineChart'
import { Empty } from '../../components/ui/Feedback'
import { fmtDateTime } from '../../lib/format'

const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v))

interface SpeedResult {
  downloadMbps: number | null
  uploadMbps: number | null
  testedAt: string | null
  running: boolean
  error: string | null
}

// Real WAN probe: the backend measures outbound latency to public endpoints;
// the fetch itself is local and instant, so we trust the server's numbers.
async function probeLatency(): Promise<{ ms: number; loss: number }> {
  try {
    const res = await fetch('/api/v1/internet', { cache: 'no-store', signal: AbortSignal.timeout(7000) })
    const data = (await res.json()) as { latency_ms?: number; loss?: number; download_mbps?: number; upload_mbps?: number; speed_tested_at?: string }
    return { ms: Math.round(data.latency_ms ?? 0), loss: data.loss ?? 1 }
  } catch {
    return { ms: 0, loss: 1 }
  }
}

// Trigger a throughput test on the backend (POST runs the Cloudflare-based
// measurement server-side; the UI just reads the outcome).
async function runSpeedTest(): Promise<SpeedResult> {
  try {
    const res = await fetch('/api/v1/internet/speed', {
      method: 'POST',
      cache: 'no-store',
      signal: AbortSignal.timeout(90_000),
    })
    if (!res.ok) {
      return { downloadMbps: null, uploadMbps: null, testedAt: null, running: false, error: `speed test failed (${res.status})` }
    }
    const data = (await res.json()) as { download_mbps?: number; upload_mbps?: number; error?: string; tested_at?: string }
    if (data.error) {
      return { downloadMbps: null, uploadMbps: null, testedAt: null, running: false, error: data.error }
    }
    return {
      downloadMbps: Math.round(data.download_mbps ?? 0),
      uploadMbps: Math.round(data.upload_mbps ?? 0),
      testedAt: data.tested_at ?? null,
      running: false,
      error: null,
    }
  } catch {
    return { downloadMbps: null, uploadMbps: null, testedAt: null, running: false, error: 'speed test timed out' }
  }
}

function Gauge({
  value,
  max,
  label,
  unit,
  accent,
}: {
  value: number
  max: number
  label: string
  unit: string
  accent: string
}) {
  const pct = clamp(value / max, 0, 1)
  const needleAngle = Math.PI - pct * Math.PI
  const R = 88
  const cx = 110
  const cy = 118
  const tip = { x: cx + R * Math.cos(needleAngle), y: cy - R * Math.sin(needleAngle) }
  const ticks = [0, 0.25, 0.5, 0.75, 1]
  const display = value.toFixed(0)

  return (
    <div className="speedometer speedometer--lg">
      <svg viewBox="0 0 220 124" className="speedometer__svg" style={{ width: '100%', maxWidth: 230, height: 'auto' }}>
        {/* track */}
        <path
          d={`M ${cx - R} ${cy} A ${R} ${R} 0 0 1 ${cx + R} ${cy}`}
          fill="none"
          stroke="var(--line)"
          strokeWidth={10}
          strokeLinecap="round"
          pathLength={100}
        />
        {/* ticks + end labels */}
        {ticks.map((t) => {
          const a = Math.PI - t * Math.PI
          const r1 = R + 9
          const r2 = R + 13
          return (
            <line
              key={t}
              x1={cx + r1 * Math.cos(a)}
              y1={cy - r1 * Math.sin(a)}
              x2={cx + r2 * Math.cos(a)}
              y2={cy - r2 * Math.sin(a)}
              stroke="var(--text-tertiary)"
              strokeWidth={1}
              opacity={0.55}
            />
          )
        })}
        <text x={cx - R - 5} y={cy + 4} textAnchor="end" style={{ fontFamily: 'var(--font-mono)', fontSize: 7 }} fill="var(--text-faint)">0</text>
        <text x={cx + R + 5} y={cy + 4} textAnchor="start" style={{ fontFamily: 'var(--font-mono)', fontSize: 7 }} fill="var(--text-faint)">{max}</text>
        {/* value arc */}
        <path
          d={`M ${cx - R} ${cy} A ${R} ${R} 0 0 1 ${cx + R} ${cy}`}
          fill="none"
          stroke={accent}
          strokeWidth={10}
          strokeLinecap="round"
          pathLength={100}
          strokeDasharray={`${pct * 100} 100`}
          style={{ transition: 'stroke-dasharray 700ms cubic-bezier(0.4, 0, 0.2, 1)' }}
        />
        {/* needle */}
        <line x1={cx} y1={cy} x2={tip.x} y2={tip.y} stroke="var(--text-primary)" strokeWidth={2} strokeLinecap="round" style={{ transition: 'x2 700ms cubic-bezier(0.4,0,0.2,1), y2 700ms cubic-bezier(0.4,0,0.2,1)' }} />
        <circle cx={cx} cy={cy} r={4.5} fill="var(--text-primary)" />
      </svg>
      <div className="speedometer__value">
        {display}
        <span className="speedometer__unit">{unit}</span>
      </div>
      <div className="speedometer__label">{label}</div>
    </div>
  )
}

function Metric({
  label,
  value,
  sub,
  badge,
}: {
  label: string
  value: string
  sub: string
  badge?: { text: string; tone: 'ok' | 'warn' | 'danger' | 'neutral' }
}) {
  return (
    <div className="panel">
      <div className="tel">
        <div className="tel__label">
          <span>{label}</span>
          {badge && <Badge tone={badge.tone} dot>{badge.text}</Badge>}
        </div>
        <div className="tel__value tel__value--sm">{value}</div>
        <div className="tel__sub"><span>{sub}</span></div>
      </div>
    </div>
  )
}

export function InternetPage() {
  const topology = useNetworkStore((s) => s.topology)
  const refresh = useNetworkStore((s) => s.refresh)

  usePoll(() => refresh(), 30_000)

  const [latency, setLatency] = useState<number | null>(null)
  const [loss, setLoss] = useState<number>(0)
  const [probedAt, setProbedAt] = useState<Date | null>(null)
  const [latHist, setLatHist] = useState<{ ts: number; ms: number }[]>([])
  const [speed, setSpeed] = useState<SpeedResult>({ downloadMbps: null, uploadMbps: null, testedAt: null, running: false, error: null })

  // real latency probe
  useEffect(() => {
    let alive = true
    const run = async () => {
      const { ms, loss: l } = await probeLatency()
      if (!alive) return
      setLatency(ms)
      setLoss(l)
      setProbedAt(new Date())
      setLatHist((h) => [...h.slice(-59), { ts: Date.now(), ms }])
    }
    void run()
    const iv = setInterval(() => void run(), 30_000)
    return () => {
      alive = false
      clearInterval(iv)
    }
  }, [])

  const startSpeedTest = async () => {
    if (speed.running) return
    setSpeed((s) => ({ ...s, running: true, error: null }))
    const result = await runSpeedTest()
    setSpeed((s) => ({ ...s, ...result }))
  }

  const gateway = topology?.gateway
  const selfIp = topology?.self_ip
  const routerNode = topology?.nodes.find((n) => n.ip === gateway)
  const routerOnline = !!routerNode?.online

  const latencyOk = latency !== null && latency > 0
  const latencyTone = latency === null || latency === 0 ? 'neutral' : latency < 60 ? 'ok' : latency < 120 ? 'warn' : 'danger'
  const lossPct = loss * 100
  const down = speed.downloadMbps ?? 0
  const up = speed.uploadMbps ?? 0
  const speedTested = speed.testedAt != null

  return (
    <div className="stack">
      <PageHead index="03" title="Internet" meta="WAN TELEMETRY · TIMED PROBE · 3 TARGETS" />

      <div className="internet-grid">
        <div className="panel internet-grid__gauges">
          <Gauge value={down} max={1000} label="DOWNLOAD" unit="Mbps" accent="var(--chart-1)" />
          <Gauge value={up} max={100} label="UPLOAD" unit="Mbps" accent="var(--chart-2)" />
          <Gauge value={latency ?? 0} max={200} label="LATENCY" unit="ms" accent="var(--chart-5)" />
          <Gauge value={lossPct} max={100} label="PACKET LOSS" unit="%" accent="var(--chart-3)" />
        </div>

        <div className="internet-grid__metrics">
          <Metric
            label="WAN Status"
            value={routerOnline ? 'CONNECTED' : 'PARTIAL'}
            sub={routerOnline ? 'gateway reachable' : 'no gateway route'}
            badge={{ text: routerOnline ? 'link up' : 'degraded', tone: routerOnline ? 'ok' : 'danger' }}
          />
          <Metric
            label="Latency"
            value={latencyOk ? `${latency} ms` : '—'}
            sub={latencyOk ? 'best of 3 timed probes' : 'probe pending'}
            badge={{ text: latencyOk ? 'probed' : 'idle', tone: latencyTone }}
          />
          <Metric
            label="Packet Loss"
            value={`${lossPct.toFixed(1)}%`}
            sub="probe window"
            badge={{ text: loss === 0 ? 'clean' : 'lossy', tone: loss === 0 ? 'ok' : 'warn' }}
          />
          <Metric
            label="Throughput"
            value={speedTested ? `${down} / ${up} Mbps` : '—'}
            sub={speedTested ? 'down / up · measured' : speed.running ? 'measuring…' : 'press “Run speed test”'}
            badge={{ text: speed.error ? 'failed' : speedTested ? 'measured' : 'idle', tone: speed.error ? 'danger' : speedTested ? 'ok' : 'neutral' }}
          />
          <Metric label="Gateway" value={gateway || '—'} sub="default route" />
          <Metric label="This node" value={selfIp || '—'} sub="primary interface" />
        </div>
      </div>

      <div className="internet-grid internet-grid--bottom">
        <div className="panel">
          <div className="panel__head">
            <h3 className="panel__title">Uplink path</h3>
          </div>
          <div className="internet-path">
            <div className="internet-path__seg">
              <span className="internet-path__seg-label">node</span>
              <span className="internet-path__seg-value">{selfIp || '—'}</span>
            </div>
            <span className={`internet-path__arrow ${routerOnline ? '' : 'internet-path__arrow--down'}`}>———▶</span>
            <div className="internet-path__seg">
              <span className="internet-path__seg-label">router</span>
              <span className="internet-path__seg-value">{gateway || '—'}</span>
            </div>
            <span className={`internet-path__arrow ${latencyOk ? '' : 'internet-path__arrow--down'}`}>———▶</span>
            <div className="internet-path__seg">
              <span className="internet-path__seg-label">internet</span>
              <span className="internet-path__seg-value">{latencyOk ? `${latency} ms` : 'unreachable'}</span>
            </div>
          </div>
        </div>

        <div className="panel">
          <div className="panel__head">
            <h3 className="panel__title">Latency · history</h3>
            <span className="k-mono k-tert" style={{ fontSize: 'var(--fs-2xs)' }}>
              {latHist.length} probes · 30s
            </span>
          </div>
          <div className="panel__body">
            {latHist.length > 1 ? (
              <Sparkline points={latHist.map((p) => ({ ts: p.ts, value: p.ms }))} color="var(--chart-5)" height={96} />
            ) : (
              <Empty title="Waiting for probes" hint="The WAN probe runs every 30 seconds; history accrues here." />
            )}
          </div>
        </div>

        <div className="panel">
          <div className="panel__head">
            <h3 className="panel__title">WAN probe</h3>
            <Badge tone={latencyOk ? 'ok' : 'neutral'} dot>{latencyOk ? 'online' : 'idle'}</Badge>
          </div>
          <div className="tel">
            <div className="metric-row">
              <span className="metric-row__label">Probed at</span>
              <span className="metric-row__value k-mono">{probedAt ? fmtDateTime(probedAt) : '—'}</span>
            </div>
            <div className="metric-row">
              <span className="metric-row__label">Targets</span>
              <span className="metric-row__value k-mono">3 public</span>
            </div>
            <div className="metric-row">
              <span className="metric-row__label">Best latency</span>
              <span className="metric-row__value k-mono">{latencyOk ? `${latency} ms` : '—'}</span>
            </div>
            <div className="metric-row">
              <span className="metric-row__label">Loss</span>
              <span className="metric-row__value k-mono">{lossPct.toFixed(1)}%</span>
            </div>
            <div className="metric-row">
              <span className="metric-row__label">Speed measured</span>
              <span className="metric-row__value k-mono">
                {speedTested ? fmtDateTime(new Date(speed.testedAt as string)) : '—'}
              </span>
            </div>
            {speed.error && (
              <div className="metric-row">
                <span className="metric-row__label">Speed error</span>
                <span className="metric-row__value k-mono" style={{ color: 'var(--danger)' }}>{speed.error}</span>
              </div>
            )}
            <div style={{ marginTop: '0.75rem' }}>
              <Button variant="primary" size="sm" disabled={speed.running} onClick={() => void startSpeedTest()}>
                {speed.running ? 'Measuring…' : 'Run speed test'}
              </Button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}