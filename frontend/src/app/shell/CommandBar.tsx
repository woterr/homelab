import { NavLink } from 'react-router-dom'
import { useConnectionStore } from '../../state/useConnectionStore'
import { useSystemStore } from '../../state/useSystemStore'
import { fmtUptime } from '../../lib/format'
import { NAV } from './nav'

function Logo() {
  return (
    <svg width={20} height={20} viewBox="0 0 24 24" aria-hidden>
      <rect x="3" y="3" width="18" height="18" rx="4" fill="var(--bg-panel)" stroke="var(--accent)" strokeWidth="1.5" />
      <circle cx="12" cy="12" r="3.5" fill="var(--accent)" />
    </svg>
  )
}

function Clock() {
  const now = new Date()
  const hh = String(now.getHours()).padStart(2, '0')
  const mm = String(now.getMinutes()).padStart(2, '0')
  const ss = String(now.getSeconds()).padStart(2, '0')
  return (
    <span className="cmd-tick">
      <span>{hh}:{mm}:{ss}</span>
    </span>
  )
}

export function CommandBar() {
  const connected = useConnectionStore((s) => s.connected)
  const system = useSystemStore((s) => s.system)
  const hostname = system?.info.hostname?.toUpperCase() || 'NODE'

  return (
    <header className="cmdbar">
      <div className="cmdbar__brand">
        <Logo />
        <span className="cmdbar__wordmark">{hostname}</span>
      </div>
      <span className="cmdbar__divider" />
      <nav className="cmdbar__nav">
        {NAV.map((entry) => (
          <NavLink
            key={entry.to}
            to={entry.to}
            end={entry.to === '/'}
            className={({ isActive }) => `cmdlink ${isActive ? 'cmdlink--active' : ''}`}
          >
            <span className="cmdlink__num">{entry.num}</span>
            {entry.label}
          </NavLink>
        ))}
      </nav>
      <div className="cmdbar__right">
        <span className="cmdbar__status" title={system ? `up ${fmtUptime(system.info.uptime_seconds)}` : undefined}>
          <span className={`cmd-tick__dot ${connected ? 'cmd-tick__dot--on' : 'cmd-tick__dot--off'}`} />
          <span className="k-mono" style={{ fontSize: 10, letterSpacing: '0.12em', fontWeight: 600, color: 'var(--text-secondary)' }}>
            system: {connected ? 'online' : 'offline'}
          </span>
        </span>
        <Clock />
      </div>
    </header>
  )
}