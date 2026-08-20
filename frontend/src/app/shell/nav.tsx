export interface NavLink {
  to: string
  num: string
  label: string
  meta?: string
}

export const NAV: NavLink[] = [
  { to: '/', num: '01', label: 'Overview' },
  { to: '/network', num: '02', label: 'Network' },
  { to: '/internet', num: '03', label: 'Internet' },
  { to: '/metrics', num: '04', label: 'Metrics' },
  { to: '/storage', num: '05', label: 'Storage' },
  { to: '/logs', num: '06', label: 'Logs' },
  { to: '/events', num: '07', label: 'Events' },
  { to: '/rules', num: '08', label: 'Rules' },
  { to: '/settings', num: '09', label: 'Settings' },
]