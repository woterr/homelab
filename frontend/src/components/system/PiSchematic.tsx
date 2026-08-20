import { fmtBytes, fmtBytesRate, fmtPct, fmtSpeedMbps, fmtTemp } from '../../lib/format'

export interface TwinData {
  hostname: string
  model: string
  cpuTotal: number
  perCore: number[]
  coreCount: number
  load1: number
  load5: number
  memUsedPct: number
  memUsed: number
  memTotal: number
  temp?: { name: string; value: number }
  voltage?: number
  powerOk: boolean
  storagePct?: number
  storageUsed?: number
  storageTotal?: number
  rx?: number
  tx?: number
  iface?: string
  linkMbps?: number
  services?: { total: number; running: number }
  devices?: { total: number; online: number }
  gateway?: string
  selfIp?: string
  demo?: boolean
}

const RAM_CELLS = 48

export function PiSchematic({ data }: { data: TwinData }) {
  const {
    cpuTotal,
    memUsedPct,
    temp,
    storagePct,
    storageUsed,
    storageTotal,
    rx,
    tx,
    iface,
    linkMbps,
    services,
    model,
    hostname,
  } = data

  const netActive = (rx ?? 0) + (tx ?? 0) > 0
  const memPct = Math.round(Math.max(0, Math.min(100, memUsedPct)))

  const tempTone = temp ? (temp.value < 65 ? 'var(--ok)' : temp.value < 78 ? 'var(--warn)' : 'var(--danger)') : 'var(--text-faint)'
  const svcTone = services && services.total > 0 ? (services.running === services.total ? 'var(--ok)' : 'var(--warn)') : 'var(--text-faint)'
  const storageTone = (storagePct ?? 0) < 85 ? 'var(--ok)' : (storagePct ?? 0) < 95 ? 'var(--warn)' : 'var(--danger)'
  const netTone = 'var(--accent)'

  // elongated horizontal memory bar across the top edge of the board
  const ramX = 247
  const ramY = 182
  const ramW = 126
  const ramH = 16
  const ramGap = 0.5
  const ramCellW = (ramW - 58 - (RAM_CELLS - 1) * ramGap) / RAM_CELLS
  const filledCells = Math.round((memPct / 100) * RAM_CELLS)

  return (
    <svg
      viewBox="0 0 620 520"
      preserveAspectRatio="xMidYMid meet"
      style={{ width: '100%', height: '100%', display: 'block' }}
      role="img"
      aria-label={`Raspberry Pi schematic of ${hostname}`}
    >
      {/* ---- connection lines: thin, from board edges to ticks ---- */}
      {/* top: thermal */}
      <line x1={310} y1={170} x2={310} y2={82} stroke="var(--line)" strokeWidth={0.8} />
      <line x1={310} y1={170} x2={310} y2={82} stroke={tempTone} strokeWidth={0.9} opacity={0.5} />
      <circle cx={310} cy={170} r={2} fill={tempTone} fillOpacity={0.7} />
      <rect x={306} y={76} width={8} height={8} fill="none" stroke={tempTone} strokeOpacity={0.6} strokeWidth={0.8} />

      {/* left: services */}
      <line x1={235} y1={268} x2={128} y2={268} stroke="var(--line)" strokeWidth={0.8} />
      <line x1={235} y1={268} x2={128} y2={268} stroke={svcTone} strokeWidth={0.9} opacity={0.5} />
      <circle cx={235} cy={268} r={2} fill={svcTone} fillOpacity={0.7} />
      <rect x={122} y={265} width={6} height={6} fill="none" stroke={svcTone} strokeOpacity={0.6} strokeWidth={0.8} />

      {/* right: network */}
      <line x1={385} y1={268} x2={492} y2={268} stroke="var(--line)" strokeWidth={0.8} />
      <line x1={385} y1={268} x2={492} y2={268} stroke={netTone} strokeWidth={0.9} opacity={netActive ? 0.7 : 0.35} />
      <circle cx={385} cy={268} r={2} fill={netTone} fillOpacity={0.7} />
      <rect x={492} y={265} width={6} height={6} fill="none" stroke={netTone} strokeOpacity={0.6} strokeWidth={0.8} />

      {/* bottom: storage */}
      <line x1={310} y1={366} x2={310} y2={452} stroke="var(--line)" strokeWidth={0.8} />
      <line x1={310} y1={366} x2={310} y2={452} stroke={storageTone} strokeWidth={0.9} opacity={0.5} />
      <circle cx={310} cy={366} r={2} fill={storageTone} fillOpacity={0.7} />
      <rect x={306} y={452} width={8} height={8} fill="none" stroke={storageTone} strokeOpacity={0.6} strokeWidth={0.8} />

      {/* ---- endpoint annotations: compact technical labels ---- */}
      {/* thermal */}
      <text x={322} y={72} style={{ fontFamily: 'var(--font-mono)', fontSize: 8, letterSpacing: '0.14em' }} fill="var(--text-tertiary)">THERMAL</text>
      <text x={322} y={84} style={{ fontFamily: 'var(--font-mono)', fontSize: 10, fontWeight: 600, fontVariantNumeric: 'tabular-nums' }} fill="var(--text-primary)">
        {temp ? fmtTemp(temp.value) : '—'}
      </text>
      {/* services */}
      <text x={116} y={262} textAnchor="end" style={{ fontFamily: 'var(--font-mono)', fontSize: 8, letterSpacing: '0.14em' }} fill="var(--text-tertiary)">SERVICES</text>
      <text x={116} y={274} textAnchor="end" style={{ fontFamily: 'var(--font-mono)', fontSize: 10, fontWeight: 600, fontVariantNumeric: 'tabular-nums' }} fill="var(--text-primary)">
        {services ? `${services.running}/${services.total}` : '—'}
      </text>
      {/* network */}
      <text x={506} y={262} style={{ fontFamily: 'var(--font-mono)', fontSize: 8, letterSpacing: '0.14em' }} fill="var(--text-tertiary)">NETWORK</text>
      <text x={506} y={274} style={{ fontFamily: 'var(--font-mono)', fontSize: 10, fontWeight: 600, fontVariantNumeric: 'tabular-nums' }} fill="var(--text-primary)">
        {netActive ? fmtBytesRate(Math.max(rx ?? 0, tx ?? 0)) : '—'}
      </text>
      {iface && (
        <text x={506} y={287} style={{ fontFamily: 'var(--font-mono)', fontSize: 7, letterSpacing: '0.12em' }} fill="var(--text-tertiary)">
          {iface.toUpperCase()}{linkMbps ? ` · ${fmtSpeedMbps(linkMbps)}` : ''}
        </text>
      )}
      {/* storage */}
      <text x={310} y={478} textAnchor="middle" style={{ fontFamily: 'var(--font-mono)', fontSize: 8, letterSpacing: '0.14em' }} fill="var(--text-tertiary)">STORAGE</text>
      <text x={310} y={490} textAnchor="middle" style={{ fontFamily: 'var(--font-mono)', fontSize: 10, fontWeight: 600, fontVariantNumeric: 'tabular-nums' }} fill="var(--text-primary)">
        {storageUsed !== undefined ? fmtBytes(storageUsed) : '—'}
      </text>
      {storageTotal !== undefined && (
        <text x={310} y={502} textAnchor="middle" style={{ fontFamily: 'var(--font-mono)', fontSize: 7, letterSpacing: '0.12em' }} fill="var(--text-tertiary)">
          of {fmtBytes(storageTotal)}
        </text>
      )}

      {/* ---- Raspberry Pi board ---- */}
      <g>
        <rect
          x={235}
          y={170}
          width={150}
          height={196}
          rx={3}
          fill="var(--bg-panel)"
          fillOpacity={0.9}
          stroke="var(--line-strong)"
          strokeWidth={1}
          style={{ filter: 'drop-shadow(0 0 18px rgba(0, 0, 0, 0.5))' }}
        />
        {/* breathing glow along the board outline — the node is alive */}
        <rect
          x={234.5}
          y={169.5}
          width={151}
          height={197}
          rx={3.5}
          fill="none"
          stroke="var(--accent)"
          strokeWidth={1}
          className="schematic-breathe"
        />

        {/* elongated memory bar — top edge, segmented across the board width */}
        <text x={373} y={179} textAnchor="end" style={{ fontFamily: 'var(--font-mono)', fontSize: 8, fontWeight: 600, fontVariantNumeric: 'tabular-nums' }} fill="var(--accent)">
          {memPct}%
        </text>
        <rect x={ramX} y={ramY} width={ramW} height={ramH} rx={2} fill="var(--bg-glass)" stroke="var(--line-strong)" strokeWidth={0.8} />
        {Array.from({ length: RAM_CELLS }).map((_, i) => {
          const on = i < filledCells
          return (
            <rect
              key={i}
              x={ramX + 56 + i * (ramCellW + ramGap)}
              y={ramY + 2}
              width={ramCellW}
              height={ramH - 4}
              rx={0.5}
              fill={on ? 'var(--accent)' : 'transparent'}
              fillOpacity={on ? 0.75 : 0}
              stroke={on ? 'none' : 'var(--line-faint)'}
            />
          )
        })}
        <text x={ramX + 6} y={ramY + ramH - 5} style={{ fontFamily: 'var(--font-mono)', fontSize: 7, letterSpacing: '0.14em' }} fill="var(--text-tertiary)">
          MEM
        </text>

        {/* SoC — center, static */}
        {/* heartbeat ring, pulses out from the core */}
        <circle cx={310} cy={246} r={14} fill="none" stroke="var(--accent)" strokeWidth={1.2} className="schematic-beat" />
        <rect x={274} y={210} width={72} height={72} rx={2} fill="var(--bg-glass)" stroke="var(--accent)" strokeOpacity={0.4} strokeWidth={1} />
        <rect x={286} y={222} width={48} height={48} fill="none" stroke="var(--accent)" strokeOpacity={0.22} strokeWidth={0.8} />
        <text x={310} y={248} textAnchor="middle" style={{ fontFamily: 'var(--font-mono)', fontSize: 6.5, letterSpacing: '0.1em' }} fill="var(--text-secondary)">
          {model.toUpperCase()}
        </text>
        <text x={310} y={262} textAnchor="middle" style={{ fontFamily: 'var(--font-mono)', fontSize: 11, fontWeight: 600, fontVariantNumeric: 'tabular-nums' }} fill="var(--text-primary)">
          {fmtPct(cpuTotal, 0)}
        </text>

        {/* IO block — bottom left */}
        <rect x={247} y={288} width={46} height={16} fill="var(--bg-glass)" stroke="var(--line-strong)" strokeWidth={0.8} />
        <text x={270} y={299} textAnchor="middle" style={{ fontFamily: 'var(--font-mono)', fontSize: 6, letterSpacing: '0.16em' }} fill="var(--text-tertiary)">IO</text>

        {/* GPIO header — bottom left, small */}
        <rect x={247} y={318} width={22} height={22} fill="var(--bg-glass)" stroke="var(--line-strong)" strokeWidth={0.8} />
        {[0, 1, 2, 3].map((i) => (
          <circle key={i} cx={253 + (i % 2) * 10} cy={324 + Math.floor(i / 2) * 10} r={1.3} fill="var(--text-tertiary)" />
        ))}
        <text x={258} y={334} textAnchor="middle" style={{ fontFamily: 'var(--font-mono)', fontSize: 5.5, letterSpacing: '0.12em' }} fill="var(--text-faint)">GPIO</text>

        {/* NIC — bottom right, lights on traffic */}
        <circle cx={360} cy={297} r={9} fill="var(--bg-glass)" stroke="var(--line-strong)" strokeWidth={0.8} />
        <circle cx={360} cy={297} r={3.5} fill="var(--accent)" fillOpacity={netActive ? 0.85 : 0.25} className={netActive ? 'schematic-nic' : undefined} style={{ transition: 'fill-opacity 400ms ease' }} />
      </g>
    </svg>
  )
}