// api.ts — typed client for the node console backend REST API.

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
  })
  if (!res.ok) {
    let msg = res.statusText
    try {
      const body = await res.json()
      if (body?.error) msg = body.error
    } catch {
      /* ignore */
    }
    throw new ApiError(res.status, msg)
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

function qs(params: Record<string, string | number | boolean | undefined>): string {
  const parts = Object.entries(params)
    .filter(([, v]) => v !== undefined && v !== '')
    .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`)
  return parts.length ? `?${parts.join('&')}` : ''
}

// ---- types ----

export interface SystemInfo {
  hostname: string
  os: string
  kernel: string
  architecture: string
  model: string
  soc?: string
  cpu_model: string
  cores: number
  uptime_seconds: number
  boot_time: number
  server: string
}

export interface CPUSample {
  Total: number
  PerCore: number[]
  User: number
  System: number
  IOWait: number
  Load1: number
  Load5: number
  Load15: number
  FreqMHz: number[]
  CoreCount: number
}

export interface MemorySample {
  total: number
  used: number
  available: number
  cached: number
  buffers: number
  swap_total: number
  swap_used: number
  used_pct: number
}

export interface ThermalZone {
  name: string
  temp_c: number
  critical_c?: number
}

export interface PowerSample {
  available: boolean
  voltage_v?: number
  throttled?: string
  power_state: string
}

export interface NetworkInterface {
  name: string
  mac: string
  mtu: number
  ipv4: string[]
  ipv6: string[]
  speed_mbps: number
  operstate: string
  carrier: boolean
  up: boolean
  primary: boolean
  rx_bytes: number
  tx_bytes: number
  rx_packets: number
  tx_packets: number
  rx_errors: number
  rx_drops: number
  tx_errors: number
  tx_drops: number
  rx_bps: number
  tx_bps: number
}

export interface Device {
  mac: string
  ip: string
  hostname: string
  vendor: string
  device_type: string
  first_seen: string
  last_seen: string
  online: boolean
  metadata?: Record<string, string>
}

export interface TopologyNode {
  id: string
  label: string
  type: string
  ip?: string
  mac?: string
  vendor?: string
  online: boolean
  primary?: boolean
  rx_bps?: number
  tx_bps?: number
}

export interface TopologyLink {
  source: string
  target: string
  type: string
}

export interface Topology {
  nodes: TopologyNode[]
  links: TopologyLink[]
  gateway: string
  self_ip: string
}

export interface Filesystem {
  mountpoint: string
  device: string
  fs_type: string
  total: number
  used: number
  available: number
  used_pct: number
  read_only: boolean
}

export interface Partition {
  device: string
  name: string
  size: number
  mountpoint: string
  fs_type: string
  read_only: boolean
  used: number
  available: number
  used_pct: number
}

export interface Disk {
  device: string
  model?: string
  read_bps: number
  write_bps: number
  read_iops: number
  write_iops: number
  busy_pct: number
}

export interface Service {
  id: string
  name: string
  kind: string
  status: string
  image?: string
  started_at?: string
  restarts: number
  cpu: number
  memory: number
}

export interface Event {
  id: string
  ts: string
  type: string
  severity: string
  source: string
  message: string
  data?: Record<string, unknown>
}

export interface SeriesMeta {
  name: string
  samples: number
  first_ts: number
  last_ts: number
  min: number
  max: number
  labels: Record<string, string>
}

export interface MetricPoint {
  ts: string
  avg: number
  min?: number
  max?: number
  sum?: number
  count?: number
}

export interface QueryResult {
  series: string
  start: string
  end: string
  resolution: string
  aggregate: string
  points: MetricPoint[]
}

export interface Rule {
  id: string
  name: string
  enabled: boolean
  metric: string
  operator: string
  threshold: number
  duration_s: number
  notify: boolean
  channels?: string[]
  message?: string
  created_at: string
}

export interface NotifyProvider {
  name: string
  configured: boolean
}

export interface NotifyConfig {
  ntfy_server: string
  ntfy_topic: string
  discord_webhook: string
  smtp_host: string
  smtp_port: number
  smtp_user: string
  smtp_from: string
  smtp_to: string
  smtp_insecure: boolean
  smtp_pass_set: boolean
  phone_webhook: string
  phone_to: string
}

export interface NotifyConfigUpdate {
  ntfy_server?: string
  ntfy_topic?: string
  discord_webhook?: string
  smtp_host?: string
  smtp_port?: number
  smtp_user?: string
  smtp_pass?: string
  smtp_from?: string
  smtp_to?: string
  smtp_insecure?: boolean
  phone_webhook?: string
  phone_to?: string
}

export interface Settings {
  providers: { name: string; configured: boolean }[]
}

export interface LogEntry {
  time: string
  level: string
  message: string
  attrs?: Record<string, unknown>
}

export interface FullSystem {
  info: SystemInfo
  cpu: CPUSample
  memory: MemorySample
  temperature: ThermalZone[]
  power: PowerSample
  demo: boolean
}

// ---- endpoints ----

export const api = {
  system: () => request<FullSystem>('/api/v1/system'),
  cpu: () => request<CPUSample>('/api/v1/system/cpu'),
  memory: () => request<MemorySample>('/api/v1/system/memory'),
  temperature: () => request<ThermalZone[]>('/api/v1/system/temperature'),
  interfaces: () => request<{ interfaces: NetworkInterface[]; gateway: string }>('/api/v1/network/interfaces'),
  devices: () => request<Device[]>('/api/v1/network/devices'),
  topology: () => request<Topology>('/api/v1/network/topology'),
  storage: () => request<{ filesystems: Filesystem[]; disks: Disk[]; partitions: Partition[] }>('/api/v1/storage'),
  disks: () => request<Disk[]>('/api/v1/storage/disks'),
  services: () => request<Service[]>('/api/v1/services'),
  power: () => request<PowerSample>('/api/v1/power'),
  events: (limit = 200) => request<Event[]>(`/api/v1/events${qs({ limit })}`),
  logs: (limit = 200, level?: string) =>
    request<LogEntry[]>(`/api/v1/logs${qs({ limit, level })}`),
  series: () => request<SeriesMeta[]>('/api/v1/metrics/series'),
  query: (series: string, start: number, end: number, resolution = '30s', aggregate = 'avg') =>
    request<QueryResult>(
      `/api/v1/metrics/query${qs({ series, start, end, resolution, aggregate })}`,
    ),
  latest: (series: string) => request<Record<string, { ts: string; avg: number }>>(`/api/v1/metrics/latest${qs({ series })}`),
  rules: () => request<Rule[]>('/api/v1/rules'),
  createRule: (r: Omit<Rule, 'id' | 'created_at' | 'enabled'>) =>
    request<Rule>('/api/v1/rules', { method: 'POST', body: JSON.stringify(r) }),
  deleteRule: (id: string) => request<void>(`/api/v1/rules/${id}`, { method: 'DELETE' }),
  settings: () => request<Settings>('/api/v1/settings'),
  notifyProviders: () => request<NotifyProvider[]>('/api/v1/notify/providers'),
  notifyTest: () => request<{ status: string; channels?: number }>('/api/v1/notify/test', { method: 'POST' }),
  notifyConfig: () => request<NotifyConfig>('/api/v1/notify/config'),
  notifyConfigSave: (c: NotifyConfigUpdate) =>
    request<NotifyConfig>('/api/v1/notify/config/', { method: 'PUT', body: JSON.stringify(c) }),
  internet: () =>
    request<{ status: string; module: string; latency_ms?: number; loss?: number; download_mbps?: number; upload_mbps?: number; speed_tested_at?: string }>('/api/v1/internet'),
  internetSpeed: () =>
    request<{ download_mbps: number; upload_mbps: number; latency_ms: number; error?: string; tested_at: string }>('/api/v1/internet/speed', { method: 'POST' }),
}
