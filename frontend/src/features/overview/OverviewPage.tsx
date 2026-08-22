import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useSystemStore } from '../../state/useSystemStore'
import { useNetworkStore } from '../../state/useNetworkStore'
import { useServicesStore } from '../../state/useServicesStore'
import { useEventsStore } from '../../state/useEventsStore'
import { useLive } from '../../lib/useLive'
import { usePoll } from '../../lib/usePoll'
import { useSeriesHistoryMulti } from '../../lib/useSeriesHistory'
import { useTelemetryStore } from '../../state/useTelemetryStore'
import { api, type Filesystem, type NetworkInterface } from '../../lib/api'
import { Button } from '../../components/ui/Button'
import { SkeletonRows, Empty } from '../../components/ui/Feedback'
import { LineChart, Sparkline } from '../../components/charts/LineChart'
import { PiSchematic, type TwinData } from '../../components/system/PiSchematic'
import { ReadinessPanel } from '../../components/system/ReadinessPanel'
import { fmtBytes, fmtBytesRate, fmtPct, fmtSpeedMbps, fmtTemp, fmtUptime, fmtRelative, severityTone } from '../../lib/format'
import { seriesColor } from '../../lib/series'
import { IconCpu, IconMemory, IconThermo, IconWifi, IconBolt, IconArrowUp, IconArrowDown, IconRefresh, IconPlus } from '../../components/ui/icons'

function throttledFlags(maskHex?: string): string[] {
  if (!maskHex) return []
  const bits = parseInt(maskHex.replace(/^0x/i, ''), 16)
  if (!Number.isFinite(bits)) return []
  const table: Array<[number, string]> = [
    [0, 'under-voltage'],
    [1, 'arm-freq capped'],
    [2, 'throttled'],
    [3, 'soft-temp limit'],
    [16, 'under-voltage (seen)'],
    [17, 'arm-freq capped (seen)'],
    [18, 'throttled (seen)'],
    [19, 'soft-temp limit (seen)'],
  ]
  return table.filter(([bit]) => bits & (1 << bit)).map(([, label]) => label)
}

export function OverviewPage() {
  const system = useSystemStore((s) => s.system)
  const refreshSystem = useSystemStore((s) => s.refresh)
  const topology = useNetworkStore((s) => s.topology)
  const refreshNetwork = useNetworkStore((s) => s.refresh)
  const services = useServicesStore((s) => s.services)
  const refreshServices = useServicesStore((s) => s.refresh)
  const events = useEventsStore((s) => s.events)
  const refreshEvents = useEventsStore((s) => s.refresh)
  const [storage, setStorage] = useState<{ filesystems: Filesystem[] } | null>(null)
  const [iface, setIface] = useState<NetworkInterface | null>(null)
  const [refreshing, setRefreshing] = useState(false)
  const navigate = useNavigate()
  const addMetric = useTelemetryStore((s) => s.add)

  const pinToTelemetry = (base: string) => {
    addMetric(base)
    navigate('/metrics')
  }

  const refreshAll = async () => {
    setRefreshing(true)
    try {
      await Promise.all([
        refreshSystem(),
        refreshNetwork(),
        refreshServices(),
        refreshEvents(30),
        api.storage().then(setStorage),
        api.interfaces().then(({ interfaces }) => setIface(interfaces.find((i) => i.primary) ?? interfaces[0] ?? null)),
      ])
    } finally {
      setRefreshing(false)
    }
  }

  useLive([])
  usePoll(refreshAll, 15000)

  const windowMs = 15 * 60_000
  const [cpuHist, tempHist, memHist, rxHist, txHist] = useSeriesHistoryMulti(
    ['cpu.utilization', 'temperature{sensor="cpu-thermal"}', 'memory.used_percent', 'network.primary_rx_bps', 'network.primary_tx_bps'],
    windowMs,
    '15s',
  )

  if (!system) {
    return (
      <div className="stack">
        <div className="sec-head">
          <span className="sec-head__index">01</span>
          <h1 className="sec-head__title">Overview</h1>
          <span className="sec-head__rule" />
          <span className="sec-head__meta">SYSTEM</span>
        </div>
        <div className="panel">
          <SkeletonRows rows={8} />
        </div>
      </div>
    )
  }

  const { info, cpu, memory, temperature, power } = system
  const temp = temperature?.[0]
  const recentEvents = events.slice(0, 8)

  const self = topology?.nodes.find((n) => n.type === 'self' || n.primary) ?? topology?.nodes.find((n) => n.ip === topology.self_ip)
  const rx = iface?.rx_bps ?? self?.rx_bps ?? 0
  const tx = iface?.tx_bps ?? self?.tx_bps ?? 0
  const devicesOnline = topology?.nodes.filter((n) => n.type !== 'internet' && n.type !== 'self' && n.online).length ?? 0
  const devicesTotal = topology?.nodes.filter((n) => n.type !== 'internet' && n.type !== 'self').length ?? 0
  const fsRoot = storage?.filesystems.find((f) => f.mountpoint === '/')

  const twin: TwinData = {
    hostname: info.hostname,
    model: info.soc || info.cpu_model || 'arm',
    cpuTotal: cpu.Total,
    perCore: cpu.PerCore,
    coreCount: cpu.CoreCount,
    load1: cpu.Load1,
    load5: cpu.Load5,
    memUsedPct: memory.used_pct,
    memUsed: memory.used,
    memTotal: memory.total,
    temp: temp ? { name: temp.name, value: temp.temp_c } : undefined,
    voltage: power.voltage_v,
    powerOk: power.available,
    storagePct: fsRoot?.used_pct,
    storageUsed: fsRoot?.used,
    storageTotal: fsRoot?.total,
    rx,
    tx,
    iface: iface?.name,
    linkMbps: iface?.speed_mbps,
    services: { total: services.length, running: services.filter((s) => s.status === 'running').length },
    devices: { total: devicesTotal, online: devicesOnline },
    gateway: topology?.gateway,
    selfIp: topology?.self_ip,
    demo: system.demo,
  }

  const subs = [
    {
      label: 'CPU',
      score: cpu.Total < 80 ? 1 : cpu.Total < 95 ? 0.6 : 0.2,
      note: fmtPct(cpu.Total, 0),
    },
    {
      label: 'Memory',
      score: memory.used_pct < 85 ? 1 : memory.used_pct < 95 ? 0.6 : 0.2,
      note: fmtPct(memory.used_pct, 0),
    },
    {
      label: 'Thermal',
      score: temp ? (temp.temp_c < 65 ? 1 : temp.temp_c < 78 ? 0.6 : 0.2) : 0.5,
      note: temp ? fmtTemp(temp.temp_c) : 'no sensor',
    },
    {
      label: 'Storage',
      score: fsRoot ? (fsRoot.used_pct < 85 ? 1 : fsRoot.used_pct < 95 ? 0.6 : 0.2) : 0.5,
      note: fsRoot ? `${fmtBytes(fsRoot.used)} / ${fmtBytes(fsRoot.total)}` : '—',
    },
    {
      label: 'Power',
      score: power.available ? 1 : 0.2,
      note: power.available ? 'nominal' : 'unavailable',
    },
    {
      label: 'Services',
      score:
        services.length > 0
          ? services.filter((s) => s.status === 'running').length / services.length
          : 0.5,
      note: services.length ? `${services.filter((s) => s.status === 'running').length}/${services.length} up` : 'none',
    },
  ]

  const nominal = power.available && temp && temp.temp_c < 65 && cpu.Total < 80 && (fsRoot?.used_pct ?? 0) < 85
  const thrFlags = throttledFlags(power.throttled)

  return (
    <div className="overview">
      {/* ---- header ---- */}
      <div className="overview-head">
        <div>
          <div className="overview-head__title">System overview</div>
          <div className="overview-head__sub">
            {info.hostname} · {info.model} · UP {fmtUptime(info.uptime_seconds)}
          </div>
          <div className="overview-head__meta">
            {info.os} · {info.kernel} · {info.architecture} · booted {fmtRelative(new Date(info.boot_time * 1000))} · {info.cores} cores
          </div>
        </div>
        <div className="overview-head__right">
          <span className={`nominal-pill ${nominal ? 'nominal-pill--ok' : 'nominal-pill--warn'}`}>
            <span className="nominal-pill__dot" />
            <span className="nominal-pill__label">{nominal ? 'All systems operational' : 'Attention needed'}</span>
          </span>
          <Button variant="primary" icon={<IconRefresh />} onClick={() => void refreshAll()} disabled={refreshing}>
            {refreshing ? 'Sampling…' : 'Execute refresh'}
          </Button>
        </div>
      </div>

      {/* ---- bento grid ---- */}
      <div className="overview-grid">
        {/* left column */}
        <div className="ov-col">
          <div className="glass-card">
            <div className="glass-card__title">
              <IconThermo width={16} height={16} />
              Temp-core
              <button className="glass-card__pin" title="Add to Telemetry" onClick={() => pinToTelemetry('temperature')}>
                <IconPlus width={12} height={12} />
              </button>
            </div>
            <div className="stat-display">
              <span className="stat-display__value">{temp ? temp.temp_c.toFixed(1) : '—'}</span>
              <span className="stat-display__unit">°C</span>
            </div>
            <div className="metric-row">
              <span className="metric-row__label">Sensor</span>
              <span className="metric-row__value k-mono">{temp?.name ?? 'no sensor'}</span>
            </div>
            <div className="metric-row">
              <span className="metric-row__label">Critical</span>
              <span className="metric-row__value k-mono">{temp?.critical_c ? fmtTemp(temp.critical_c) : '—'}</span>
            </div>
            <div className="stack" style={{ marginTop: 'var(--sp-3)', gap: 'var(--sp-2)' }}>
              <span className="k-label">TREND · 15M</span>
              <Sparkline points={tempHist} color="var(--warn)" height={40} unit="°C" />
            </div>
          </div>

          <div className="glass-card glass-card--grow">
            <div className="glass-card__title">
              <IconMemory width={16} height={16} />
              Memory
              <button className="glass-card__pin" title="Add to Telemetry" onClick={() => pinToTelemetry('memory.used_percent')}>
                <IconPlus width={12} height={12} />
              </button>
            </div>
            <div className="stat-display">
              <span className="stat-display__value">{fmtPct(memory.used_pct, 0)}</span>
            </div>
            <div className="wave-bar" role="progressbar" aria-valuenow={Math.round(memory.used_pct)} aria-valuemin={0} aria-valuemax={100}>
              <div className="wave-bar__fill" style={{ width: `${memory.used_pct}%` }} />
            </div>
            <div className="stack" style={{ marginTop: 'var(--sp-4)', gap: 0 }}>
              <div className="metric-row">
                <span className="metric-row__label">Used</span>
                <span className="metric-row__value">{fmtBytes(memory.used)} / {fmtBytes(memory.total)}</span>
              </div>
              <div className="metric-row">
                <span className="metric-row__label">Cached</span>
                <span className="metric-row__value">{fmtBytes(memory.cached)}</span>
              </div>
              <div className="metric-row">
                <span className="metric-row__label">Swap</span>
                <span className="metric-row__value">{fmtBytes(memory.swap_used)} / {fmtBytes(memory.swap_total)}</span>
              </div>
            </div>
            <div className="stack" style={{ marginTop: 'var(--sp-3)', gap: 'var(--sp-2)' }}>
              <span className="k-label">MEMORY · 15M</span>
              <LineChart
                series={[{ key: 'mem', label: 'memory.used_percent', color: seriesColor('mem'), points: memHist }]}
                height={70}
                unit="%"
              />
            </div>
          </div>
        </div>

        {/* center — schematic */}
        <div className="ov-schematic">
          <span className="ov-schematic__corner ov-schematic__corner--tl label-caps">
            <span className="ov-schematic__dot" />
            Schema: Alpha
          </span>
          <span className="ov-schematic__corner ov-schematic__corner--tr label-caps k-tert">
            HW-ID: {info.model.toUpperCase()}
          </span>
          <div className="ov-schematic__stage">
            <PiSchematic data={twin} />
          </div>
          <div className="ov-schematic__corner ov-schematic__corner--bl">
            <span className="ov-chip">CPU-{cpu.CoreCount}C</span>
            <span className="ov-chip">NET-TX {fmtBytesRate(tx)}</span>
            <span className="ov-chip">GW {topology?.gateway ?? '—'}</span>
            <span className="ov-chip">LAN {topology?.self_ip ?? '—'}</span>
            <span className="ov-chip">{services.filter((s) => s.status === 'running').length}/{services.length} SVC</span>
            <span className="ov-chip">{devicesOnline}/{devicesTotal} DEV</span>
          </div>
        </div>

        {/* right column */}
        <div className="ov-col">
          <div className="glass-card">
            <div className="glass-card__title">
              <IconCpu width={16} height={16} />
              Processor
              <button className="glass-card__pin" title="Add to Telemetry" onClick={() => pinToTelemetry('cpu.utilization')}>
                <IconPlus width={12} height={12} />
              </button>
            </div>
            <div className="stat-display">
              <span className="stat-display__value">{fmtPct(cpu.Total, 0)}</span>
            </div>
            <div className="core-bars">
              {cpu.PerCore.map((v, i) => (
                <div className="core-bar" key={i}>
                  <span className="core-bar__label">{i.toString().padStart(2, '0')}</span>
                  <span className="core-bar__track">
                    <span className="core-bar__fill" style={{ width: `${v}%` }} />
                  </span>
                  <span className="k-mono" style={{ width: '2.4rem', fontSize: 10, textAlign: 'right', color: 'var(--text-tertiary)' }}>
                    {Math.round(v)}%
                  </span>
                </div>
              ))}
            </div>
            <div className="stack" style={{ marginTop: 'var(--sp-4)', gap: 0 }}>
              <div className="metric-row">
                <span className="metric-row__label">Load 1m</span>
                <span className="metric-row__value">{cpu.Load1.toFixed(2)}</span>
              </div>
              <div className="metric-row">
                <span className="metric-row__label">Load 5m</span>
                <span className="metric-row__value">{cpu.Load5.toFixed(2)}</span>
              </div>
            </div>
            <div className="stack" style={{ marginTop: 'var(--sp-3)', gap: 'var(--sp-2)' }}>
              <span className="k-label">CPU · 15M</span>
              <LineChart
                series={[{ key: 'cpu', label: 'cpu.utilization', color: seriesColor('cpu'), points: cpuHist }]}
                thresholds={[{ value: 85, label: 'threshold', color: 'var(--danger)' }]}
                height={120}
                unit="%"
              />
            </div>
          </div>

          <div className="glass-card glass-card--grow">
            <div className="glass-card__title">
              <IconWifi width={16} height={16} />
              Network
              <button className="glass-card__pin" title="Add to Telemetry" onClick={() => pinToTelemetry('network.primary_tx_bps')}>
                <IconPlus width={12} height={12} />
              </button>
            </div>
            <div className="stat-display">
              <span className="stat-display__value">{fmtBytesRate(rx + tx)}</span>
            </div>
            <div className="stack" style={{ gap: 'var(--sp-2)' }}>
              <div className="metric-row">
                <span className="metric-row__label" style={{ display: 'inline-flex', gap: 6, alignItems: 'center' }}>
                  <IconArrowDown width={12} height={12} style={{ color: 'var(--ok)' }} /> RX
                </span>
                <span className="metric-row__value" style={{ color: 'var(--ok)' }}>{fmtBytesRate(rx)}</span>
              </div>
              <div className="metric-row">
                <span className="metric-row__label" style={{ display: 'inline-flex', gap: 6, alignItems: 'center' }}>
                  <IconArrowUp width={12} height={12} style={{ color: 'var(--warn)' }} /> TX
                </span>
                <span className="metric-row__value" style={{ color: 'var(--warn)' }}>{fmtBytesRate(tx)}</span>
              </div>
              <div className="metric-row">
                <span className="metric-row__label">Interface</span>
                <span className="metric-row__value k-mono">{iface?.name ?? '—'}</span>
              </div>
              <div className="metric-row">
                <span className="metric-row__label">Link</span>
                <span className="metric-row__value k-mono">
                  {iface?.up ? fmtSpeedMbps(iface.speed_mbps) : iface?.name ? 'down' : '—'}
                </span>
              </div>
            </div>
            <div className="stack" style={{ marginTop: 'var(--sp-3)', gap: 'var(--sp-2)' }}>
              <span className="k-label">THROUGHPUT · 15M</span>
              <LineChart
                series={[
                  { key: 'rx', label: 'rx', color: seriesColor('rx'), points: rxHist },
                  { key: 'tx', label: 'tx', color: seriesColor('tx'), points: txHist },
                ]}
                height={120}
                unit=""
              />
            </div>
          </div>
        </div>

        {/* bottom row — three independent columns */}
        <div className="ov-row">
          <div className="glass-card ov-events">
            <div className="glass-card__title">Event log · tail</div>
            {recentEvents.length === 0 ? (
              <Empty title="No events yet" hint="Events appear here as the node notices changes." />
            ) : (
              recentEvents.map((e) => (
                <div className="log-row" key={e.id}>
                  <span className="log-row__time">{fmtRelative(e.ts)}</span>
                  <span className="log-row__body">{e.message}</span>
                  <span className={`sev-badge sev-badge--${severityTone(e.severity)}`}>{e.severity}</span>
                </div>
              ))
            )}
          </div>

          <div className="glass-card">
            <div className="glass-card__title">System readiness</div>
            <ReadinessPanel subs={subs} />
          </div>

          <div className="glass-card">
            <div className="glass-card__title">
              <IconBolt width={16} height={16} />
              Power
            </div>
            <div className="stack" style={{ gap: 0 }}>
              <div className="metric-row">
                <span className="metric-row__label">Status</span>
                <span className={`metric-row__value ${power.available ? 'k-ok' : 'k-warn'}`}>
                  {power.available ? 'nominal' : 'unavailable'}
                </span>
              </div>
              <div className="metric-row">
                <span className="metric-row__label">Rail</span>
                <span className="metric-row__value k-mono">{power.voltage_v ? `${power.voltage_v.toFixed(2)} V` : '—'}</span>
              </div>
              {power.voltage_v !== undefined && (
                <div className="stack" style={{ gap: 'var(--sp-1)', padding: 'var(--sp-2) 0 var(--sp-3)' }}>
                  <div className="wave-bar" role="progressbar" aria-valuemin={430} aria-valuemax={550} aria-valuenow={Math.round(power.voltage_v * 100)}>
                    <div className="wave-bar__fill" style={{ width: `${Math.max(0, Math.min(100, ((power.voltage_v - 4.3) / 1.2) * 100))}%` }} />
                  </div>
                  <span className="k-faint" style={{ fontSize: 'var(--fs-2xs)' }}>RAIL 4.3V — 5.5V · NOMINAL 5.1V</span>
                </div>
              )}
              <div className="metric-row">
                <span className="metric-row__label">Throttling</span>
                <span className={`metric-row__value ${thrFlags.length ? 'k-warn' : 'k-ok'}`}>
                  {thrFlags.length ? thrFlags.join(', ') : 'none detected'}
                </span>
              </div>
              <div className="metric-row">
                <span className="metric-row__label">Source</span>
                <span className="metric-row__value k-mono">{power.available ? 'vcgencmd' : 'unavailable'}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}