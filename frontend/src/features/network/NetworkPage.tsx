import { useMemo, useState } from 'react'
import { useNetworkStore } from '../../state/useNetworkStore'
import { usePoll } from '../../lib/usePoll'
import { Badge } from '../../components/ui/Badge'
import { statusTone } from '../../lib/status'
import { Empty } from '../../components/ui/Feedback'
import { Input } from '../../components/ui/Inputs'
import { PageHead } from '../../components/ui/PageHead'
import { fmtRelative } from '../../lib/format'
import { IconDevice, IconSearch } from '../../components/ui/icons'
import { TopologyView } from '../../components/topology/TopologyView'

export function NetworkPage() {
  const topology = useNetworkStore((s) => s.topology)
  const devices = useNetworkStore((s) => s.devices)
  const refresh = useNetworkStore((s) => s.refresh)
  const [q, setQ] = useState('')

  usePoll(() => refresh(), 60_000)

  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase()
    if (!needle) return devices
    return devices.filter((d) =>
      [d.ip, d.mac, d.hostname, d.vendor, d.device_type].some((v) => v?.toLowerCase().includes(needle)),
    )
  }, [devices, q])

  const online = devices.filter((d) => d.online).length

  return (
    <div className="stack">
      <PageHead
        index="02"
        title="Network"
        meta={`${online}/${devices.length} ONLINE · ${topology?.nodes.length ?? 0} NODES`}
        right={
          <div style={{ position: 'relative', width: '16rem' }}>
            <Input placeholder="filter ip · mac · host" value={q} onChange={(e) => setQ(e.target.value)} />
            <span style={{ position: 'absolute', right: 10, top: '50%', transform: 'translateY(-50%)', color: 'var(--text-tertiary)' }}>
              <IconSearch width={14} height={14} />
            </span>
          </div>
        }
      />

      <div className="panel">
        <div className="panel__head">
          <h3 className="panel__title">Topology</h3>
          {topology?.gateway && (
            <span className="k-mono k-tert" style={{ fontSize: 'var(--fs-2xs)' }}>
              gw {topology.gateway}
            </span>
          )}
        </div>
        <div className="panel__body panel__body--flush">
          {topology ? (
            <TopologyView topology={topology} />
          ) : (
            <Empty title="No neighbors detected" hint="Passive ARP/mDNS discovery runs continuously; the ICMP probe is gentle and respects the configured window." />
          )}
        </div>
      </div>

      <div className="panel">
        <div className="panel__head">
          <h3 className="panel__title">Devices</h3>
          <span className="k-mono k-tert" style={{ fontSize: 'var(--fs-2xs)' }}>
            {filtered.length} shown
          </span>
        </div>
        <div className="panel__body">
          {devices.length === 0 ? (
            <Empty title="No devices yet" hint="Discovery is passive first — devices show up as they appear in ARP and mDNS." />
          ) : filtered.length === 0 ? (
            <Empty title="No matches" hint="Try a different filter." />
          ) : (
            <div className="grid grid--3">
              {filtered.map((d) => (
                <div className={`panel device-card ${d.online ? 'device-card--online' : 'device-card--offline'}`} key={d.mac || d.ip}>
                  <div className="device-card__head">
                    <span className="device-card__glyph">
                      <IconDevice width={16} height={16} />
                    </span>
                    <Badge tone={statusTone(d.online ? 'online' : 'offline')} dot>
                      {d.online ? 'online' : 'offline'}
                    </Badge>
                  </div>
                  <div className="device-card__host k-truncate">{d.hostname || 'unknown'}</div>
                  <div className="k-mono k-tert device-card__ip">{d.ip || '—'}</div>
                  <div className="device-card__meta">
                    <span className="k-mono">{d.mac}</span>
                    <span>{d.vendor || 'unknown'}</span>
                    <span>{d.device_type || 'unknown'}</span>
                    <span className="k-faint">seen {fmtRelative(d.last_seen)}</span>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}