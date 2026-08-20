import { useEffect, useState } from 'react'
import { api, type Disk, type Filesystem, type Partition } from '../../lib/api'
import { usePoll } from '../../lib/usePoll'
import { Badge } from '../../components/ui/Badge'
import { Meter } from '../../components/ui/Meter'
import { Empty, SkeletonRows } from '../../components/ui/Feedback'
import { PageHead } from '../../components/ui/PageHead'
import { fmtBytes, fmtBytesRate } from '../../lib/format'
import { IconDrive } from '../../components/ui/icons'

export function StoragePage() {
  const [fs, setFs] = useState<Filesystem[]>([])
  const [disks, setDisks] = useState<Disk[]>([])
  const [parts, setParts] = useState<Partition[]>([])
  const [loading, setLoading] = useState(true)

  const refresh = async () => {
    try {
      const s = await api.storage()
      setFs(s.filesystems)
      setDisks(s.disks)
      setParts(s.partitions ?? [])
      setLoading(false)
    } catch {
      setLoading(false)
    }
  }
  usePoll(refresh, 30_000)

  useEffect(() => {
    void refresh()
  }, [])

  if (loading) {
    return (
      <div className="stack">
        <PageHead index="05" title="Storage" meta="MOUNTED PARTITIONS + BLOCK DEVICES" />
        <div className="panel">
          <SkeletonRows rows={5} />
        </div>
      </div>
    )
  }

  const totalCap = fs.reduce((a, f) => a + f.total, 0)
  const totalUsed = fs.reduce((a, f) => a + f.used, 0)
  const totalPct = totalCap > 0 ? (totalUsed / totalCap) * 100 : 0

  return (
    <div className="stack">
      <PageHead
        index="05"
        title="Storage"
        meta="MOUNTED PARTITIONS + BLOCK DEVICES"
        right={
          <span className="k-mono k-tert" style={{ fontSize: 'var(--fs-2xs)' }}>
            {fmtBytes(totalUsed)} / {fmtBytes(totalCap)}
          </span>
        }
      />

      <div className="panel">
        <div className="panel__head">
          <h3 className="panel__title">Capacity</h3>
          <span className="k-value">{fmtBytes(totalUsed)} / {fmtBytes(totalCap)}</span>
        </div>
        <div className="panel__body">
          <Meter value={totalPct} size="lg" tone={totalPct > 90 ? 'danger' : totalPct > 75 ? 'warn' : 'ok'} wave />
          <div className="tel__sub">
            <span>{fmtBytes(totalUsed)} allocated</span>
            <span>{fmtBytes(totalCap - totalUsed)} free</span>
          </div>
        </div>
      </div>

      <div className="grid grid--2">
        <div className="panel">
          <div className="panel__head">
            <h3 className="panel__title">Mounted partitions</h3>
            <span className="k-mono k-tert" style={{ fontSize: 'var(--fs-2xs)' }}>
              {parts.length} DRIVES · lsblk
            </span>
          </div>
          <div className="panel__body--flush">
            {parts.length === 0 ? (
              <Empty title="No partitions detected" hint="lsblk is used on the host to enumerate mounted drives." />
            ) : (
              parts.map((p) => (
                <div className="storage-fs" key={p.device}>
                  <div className="storage-fs__head">
                    <IconDrive width={14} height={14} style={{ color: 'var(--text-tertiary)', flexShrink: 0 }} />
                    <span className="storage-fs__mount">{p.mountpoint || p.device}</span>
                    <span className="storage-fs__dev">
                      <span className="k-mono">{p.device}</span>
                      <Badge tone="neutral">{p.fs_type || '—'}</Badge>
                      {p.read_only && <span className="k-faint" style={{ fontSize: 'var(--fs-2xs)' }}>ro</span>}
                    </span>
                    <span className="k-mono k-tert">{fmtBytes(p.size)}</span>
                  </div>
                  <Meter value={p.used_pct} tone={p.used_pct > 90 ? 'danger' : p.used_pct > 75 ? 'warn' : 'ok'} wave />
                  <div className="storage-fs__sub">
                    <span>{fmtBytes(p.used)} used</span>
                    <span>{fmtBytes(p.available)} free</span>
                    <span>{p.used_pct.toFixed(1)}%</span>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>

        <div className="panel">
          <div className="panel__head">
            <h3 className="panel__title">Disk I/O</h3>
            <IconDrive width={14} height={14} />
          </div>
          <div className="panel__body--flush">
            {disks.length === 0 ? (
              <Empty title="No block devices" hint="Disks are discovered from /sys/block." />
            ) : (
              <table className="table">
                <thead>
                  <tr>
                    <th>Device</th>
                    <th>Read</th>
                    <th>Write</th>
                    <th>IOPS</th>
                    <th style={{ width: '8rem' }}>Busy</th>
                  </tr>
                </thead>
                <tbody>
                  {disks.map((d) => (
                    <tr key={d.device}>
                      <td className="k-mono" style={{ fontWeight: 600 }}>
                        {d.device}
                        {d.model && <div className="k-faint" style={{ fontSize: 'var(--fs-2xs)' }}>{d.model}</div>}
                      </td>
                      <td className="k-mono">{fmtBytesRate(d.read_bps)}</td>
                      <td className="k-mono">{fmtBytesRate(d.write_bps)}</td>
                      <td className="k-mono">{d.read_iops.toFixed(0)} / {d.write_iops.toFixed(0)}</td>
                      <td>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                          <Meter value={d.busy_pct} />
                          <span className="k-mono" style={{ fontSize: 'var(--fs-xs)', width: '2.5rem' }}>{d.busy_pct.toFixed(0)}%</span>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>
      </div>

      <div className="panel" style={{ padding: 'var(--sp-3) var(--sp-4)' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--sp-3)' }}>
          <Badge tone="info">S.M.A.R.T.</Badge>
          <span className="k-sec" style={{ fontSize: 'var(--fs-sm)' }}>
            Health attributes are read when the node exposes them; on this deployment smartctl is not installed, so SMART degrades gracefully.
          </span>
        </div>
      </div>
    </div>
  )
}