import { useMemo, useRef, useState } from 'react'
import type { Topology, TopologyNode, TopologyLink } from '../../lib/api'
import { fmtBytesRate } from '../../lib/format'
import { IconX } from '../ui/icons'

const TYPE_COLORS: Record<string, string> = {
  internet: 'var(--chart-3)',
  router: 'var(--chart-2)',
  server: 'var(--chart-0)',
  nas: 'var(--chart-1)',
  laptop: 'var(--chart-5)',
  mobile: 'var(--chart-4)',
  device: 'var(--chart-7)',
  docker: 'var(--chart-6)',
  self: 'var(--accent)',
}

const TYPE_GLYPHS: Record<string, string> = {
  internet: '◎',
  router: '◈',
  server: '▣',
  nas: '▤',
  laptop: '◰',
  mobile: '◱',
  device: '●',
  docker: '◇',
  self: 'π',
}

interface Pos {
  x: number
  y: number
}

interface Placed extends TopologyNode {
  x: number
  y: number
}

interface PlacedLink extends TopologyLink {
  source: string
  target: string
}

function colorFor(node: TopologyNode): string {
  return TYPE_COLORS[node.type] ?? 'var(--text-tertiary)'
}

const glyph = (node: TopologyNode) => TYPE_GLYPHS[node.type] ?? '●'

/**
 * Deterministic radial layout: router (or this node) at the centre, the
 * internet uplink pinned to the top and this node pinned to the right, with
 * every other device distributed on rings that scale with the container size.
 * The innermost ring leaves clean gaps next to the pinned anchors so labels
 * never overlap. There is no force simulation, so the graph never drifts,
 * clusters or "zooms" — it stays exactly where the layout put it and glides
 * smoothly when data changes.
 */
function layout(size: { width: number; height: number }, nodes: TopologyNode[]): Map<string, Pos> {
  const out = new Map<string, Pos>()
  if (nodes.length === 0) return out
  const cx = size.width / 2
  const cy = size.height / 2

  const router = nodes.find((n) => n.type === 'router' || n.primary) ?? null
  const self = nodes.find((n) => n.type === 'self' || n.primary)
  const center = router ?? self ?? nodes[0]
  const internet = nodes.find((n) => n.type === 'internet')

  // Outer radius bounded by the smallest dimension so nothing clips at the edges.
  const Rmax = Math.max(78, Math.min(size.width, size.height) / 2 - 22)

  if (center) out.set(center.id, { x: cx, y: cy })
  if (internet && internet.id !== center.id) out.set(internet.id, { x: cx, y: cy - Rmax })
  if (self && self.id !== center.id && self.id !== internet?.id) {
    out.set(self.id, { x: cx + Rmax, y: cy })
  }

  const placed = new Set(out.keys())
  const rest = nodes.filter((n) => !placed.has(n.id))

  // Angular spacing keeps glyphs and their labels apart; scales with the radius.
  const spacing = Math.min(58, Math.max(46, Rmax * 0.5))

  // Ring radii that fill the available area without exceeding it.
  const radii: number[] = []
  for (let r = Rmax * 0.6; r <= Rmax - 10; r += Rmax * 0.34) {
    radii.push(r)
  }
  if (radii.length === 0) radii.push(Math.max(60, Rmax * 0.6))

  let i = 0
  for (let ring = 0; ring < radii.length && i < rest.length; ring++) {
    const rr = radii[ring]
    const count = Math.max(4, Math.min(rest.length - i, Math.floor((2 * Math.PI * rr) / spacing)))
    const step = (Math.PI * 2) / count
    // Only rings that reach the pinned anchors need to reserve their spokes.
    const skip = Rmax - rr < 34 ? [0, -Math.PI / 2] : []
    const margin = step * 0.42
    for (let k = 0; k < count && i < rest.length; k++) {
      const a = (((k + ring * 0.5) * step) % (Math.PI * 2) + Math.PI * 2) % (Math.PI * 2)
      const near = skip.some((s) => {
        const d = Math.min(Math.abs(a - s), Math.PI * 2 - Math.abs(a - s))
        return d < margin
      })
      if (near) continue
      out.set(rest[i].id, { x: cx + Math.cos(a) * rr, y: cy + Math.sin(a) * rr })
      i++
    }
  }

  // Any leftovers spread evenly on the outermost ring, keeping anchor spokes clear.
  if (i < rest.length) {
    const rr = Math.min(radii[radii.length - 1], Rmax - 14)
    const n = rest.length - i
    const step = (Math.PI * 2) / n
    const margin = step * 0.42
    let k = 0
    let attempts = 0
    const maxAttempts = n * 3
    while (i < rest.length && attempts < maxAttempts) {
      const a = (((k + 0.5) * step) % (Math.PI * 2) + Math.PI * 2) % (Math.PI * 2)
      k++
      attempts++
      const near = [0, -Math.PI / 2].some((s) => {
        const d = Math.min(Math.abs(a - s), Math.PI * 2 - Math.abs(a - s))
        return d < margin
      })
      if (near) continue
      out.set(rest[i].id, { x: cx + Math.cos(a) * rr, y: cy + Math.sin(a) * rr })
      i++
    }
  }
  return out
}

export function TopologyView({ topology }: { topology: Topology }) {
  const wrapRef = useRef<HTMLDivElement>(null)
  const [size, setSize] = useState({ width: 0, height: 0 })
  const [selected, setSelected] = useState<Placed | null>(null)
  const [hovered, setHovered] = useState<string | null>(null)

  // observe container size (no rAF hot loop)
  const roRef = useRef<ResizeObserver | null>(null)
  const initRo = () => {
    if (!wrapRef.current || roRef.current) return
    const ro = new ResizeObserver((entries) => {
      const r = entries[0].contentRect
      setSize({ width: r.width, height: r.height })
    })
    ro.observe(wrapRef.current)
    roRef.current = ro
  }

  const positions = useMemo(
    () => layout(size, topology.nodes),
    [size, topology],
  )

  const placed: Placed[] = useMemo(
    () =>
      topology.nodes.map((n) => {
        const p = positions.get(n.id) ?? { x: size.width / 2, y: size.height / 2 }
        return { ...n, x: p.x, y: p.y }
      }),
    [topology, positions, size],
  )

  const links: PlacedLink[] = useMemo(() => {
    const ids = new Set(topology.nodes.map((n) => n.id))
    const byId = new Map(topology.nodes.map((n) => [n.id, n]))
    const out: PlacedLink[] = []
    const seen = new Set<string>()
    const add = (a: string, b: string, type: string) => {
      const key = a < b ? `${a}|${b}` : `${b}|${a}`
      if (seen.has(key)) return
      seen.add(key)
      out.push({ source: a, target: b, type })
    }

    for (const l of topology.links) {
      if (ids.has(l.source) && ids.has(l.target)) add(l.source, l.target, l.type)
    }

    // ensure every device hangs off the router / this node
    const router = topology.nodes.find((n) => n.type === 'router' || n.primary)
    const self = topology.nodes.find((n) => n.type === 'self' || n.primary)
    const hub = router ?? self
    const internet = topology.nodes.find((n) => n.type === 'internet')
    if (hub) {
      for (const n of topology.nodes) {
        if (n.id === hub.id) continue
        if (byId.get(n.id)?.type === 'internet' && hub.type !== 'router') continue
        add(hub.id, n.id, n.type === 'internet' ? 'wan' : 'lan')
      }
    }
    if (internet && hub && internet.id !== hub.id) add(hub.id, internet.id, 'wan')
    return out
  }, [topology])

  const nodeById = useMemo(() => new Map(placed.map((n) => [n.id, n])), [placed])

  const hoverLinks = (id: string | null) => {
    if (!id) return new Set<string>()
    return new Set(links.filter((l) => l.source === id || l.target === id).map((l) => `${l.source}|${l.target}`))
  }
  const activeLinks = useMemo(() => hoverLinks(hovered), [hovered, links]) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="topology-wrap" ref={(el) => {
      wrapRef.current = el
      initRo()
    }}>
      <svg width={size.width} height={size.height} style={{ display: 'block' }}>
        {/* links */}
        {links.map((l) => {
          const a = nodeById.get(l.source)
          const b = nodeById.get(l.target)
          if (!a || !b) return null
          const live = a.online && b.online
          const dimmed = activeLinks.size > 0 && !activeLinks.has(`${l.source}|${l.target}`) && !activeLinks.has(`${l.target}|${l.source}`)
          const key = `${l.source}|${l.target}`
          return (
            <g key={key} opacity={dimmed ? 0.12 : 1} style={{ transition: 'opacity 200ms ease' }}>
              <line
                x1={a.x}
                y1={a.y}
                x2={b.x}
                y2={b.y}
                stroke={live ? 'var(--line-cyan)' : 'var(--line-faint)'}
                strokeWidth={1.2}
                opacity={0.7}
              />
            </g>
          )
        })}

        {/* nodes */}
        {placed.map((n) => {
          const color = colorFor(n)
          const isHover = hovered === n.id
          return (
            <g
              key={n.id}
              className="topology-node"
              transform={`translate(${n.x}, ${n.y})`}
              style={{ transition: 'transform 600ms cubic-bezier(0.4, 0, 0.2, 1)', cursor: 'pointer' }}
              onMouseEnter={() => setHovered(n.id)}
              onMouseLeave={() => setHovered(null)}
              onClick={() => setSelected(n)}
            >
              <circle
                r={isHover ? 26 : 20}
                fill={color}
                fillOpacity={n.online ? 0.1 : 0.03}
                stroke={n.online ? color : 'var(--line)'}
                strokeWidth={1.4}
                strokeDasharray={n.online ? undefined : '3 3'}
                style={{ transition: 'r 200ms ease, fill-opacity 200ms ease' }}
              />
              <circle cx={12} cy={-12} r={3} fill={n.online ? 'var(--up)' : 'var(--down)'} stroke="var(--bg-panel)" strokeWidth={1} />
              <text textAnchor="middle" dy="0.35em" fontSize={15} fill={n.online ? color : 'var(--text-faint)'}>
                {glyph(n)}
              </text>
              <text textAnchor="middle" y={34} fontFamily="var(--font-mono)" fontSize={10} fontWeight={600} fill="var(--text-primary)">
                {n.label.toUpperCase()}
              </text>
              <text textAnchor="middle" y={46} fontFamily="var(--font-mono)" fontSize={8.5} fill="var(--text-tertiary)">
                {n.ip ?? n.type.toUpperCase()}
              </text>
            </g>
          )
        })}
      </svg>

      <div className="topology-legend" style={{ position: 'absolute', left: 12, top: 12, display: 'flex', gap: 6, flexWrap: 'wrap' }}>
        {Object.entries(TYPE_COLORS)
          .filter(([t]) => topology.nodes.some((n) => n.type === t))
          .map(([t, c]) => (
            <span key={t} className="status" style={{ background: 'var(--bg-layer)', border: '1px solid var(--line)', borderRadius: 'var(--r-xs)', padding: '2px 8px' }}>
              <span style={{ width: 8, height: 8, borderRadius: 2, background: c, display: 'inline-block' }} />
              {t}
            </span>
          ))}
      </div>

      {selected && (
        <div className="panel" style={{ position: 'absolute', right: 12, top: 12, width: 240 }}>
          <div className="panel__head" style={{ padding: 'var(--sp-3)' }}>
            <span className="panel__title">{selected.label}</span>
            <button className="btn btn--ghost btn--sm" onClick={() => setSelected(null)} aria-label="Close">
              <IconX width={14} height={14} />
            </button>
          </div>
          <div className="panel__body" style={{ padding: 'var(--sp-3)' }}>
            <div className="kv">
              <span className="kv__k">Type</span>
              <span className="kv__v">{selected.type}</span>
              {selected.ip && (
                <>
                  <span className="kv__k">IP</span>
                  <span className="kv__v k-mono">{selected.ip}</span>
                </>
              )}
              {selected.mac && (
                <>
                  <span className="kv__k">MAC</span>
                  <span className="kv__v k-mono">{selected.mac}</span>
                </>
              )}
              {selected.vendor && (
                <>
                  <span className="kv__k">Vendor</span>
                  <span className="kv__v">{selected.vendor}</span>
                </>
              )}
              {(selected.rx_bps ?? 0) > 0 && (
                <>
                  <span className="kv__k">RX</span>
                  <span className="kv__v">{fmtBytesRate(selected.rx_bps ?? 0)}</span>
                </>
              )}
              {(selected.tx_bps ?? 0) > 0 && (
                <>
                  <span className="kv__k">TX</span>
                  <span className="kv__v">{fmtBytesRate(selected.tx_bps ?? 0)}</span>
                </>
              )}
              <span className="kv__k">Status</span>
              <span className={`kv__v ${selected.online ? 'k-ok' : 'k-danger'}`}>
                {selected.online ? 'online' : 'offline'}
              </span>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}