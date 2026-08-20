import { useEffect, useState } from 'react'
import { api, type NotifyConfig, type NotifyConfigUpdate, type NotifyProvider } from '../../lib/api'
import { useSettingsStore } from '../../state/useSettingsStore'
import { useToastStore } from '../../state/useToastStore'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Field, Input, Select, Switch } from '../../components/ui/Inputs'
import { Empty } from '../../components/ui/Feedback'
import { PageHead } from '../../components/ui/PageHead'
import { IconBell, IconSettings } from '../../components/ui/icons'

const ACCENTS = ['#ffb4a8', '#f0a093', '#e6b45f', '#b6bdea']

const CHANNEL_GLYPH: Record<string, string> = {
  ntfy: 'ntfy',
  discord: 'disc',
  email: 'mail',
  phone: 'sms',
  telegram: 'tg',
}

interface Draft {
  ntfy_server: string
  ntfy_topic: string
  discord_webhook: string
  smtp_host: string
  smtp_port: number
  smtp_user: string
  smtp_pass: string
  smtp_from: string
  smtp_to: string
  smtp_insecure: boolean
  phone_webhook: string
  phone_to: string
}

function fromConfig(c: NotifyConfig | null): Draft {
  return {
    ntfy_server: c?.ntfy_server ?? '',
    ntfy_topic: c?.ntfy_topic ?? '',
    discord_webhook: c?.discord_webhook ?? '',
    smtp_host: c?.smtp_host ?? '',
    smtp_port: c?.smtp_port ?? 587,
    smtp_user: c?.smtp_user ?? '',
    smtp_pass: '',
    smtp_from: c?.smtp_from ?? '',
    smtp_to: c?.smtp_to ?? '',
    smtp_insecure: c?.smtp_insecure ?? false,
    phone_webhook: c?.phone_webhook ?? '',
    phone_to: c?.phone_to ?? '',
  }
}

export function SettingsPage() {
  const motion = useSettingsStore((s) => s.motion)
  const setMotion = useSettingsStore((s) => s.setMotion)
  const accent = useSettingsStore((s) => s.accent)
  const setAccent = useSettingsStore((s) => s.setAccent)
  const livePoll = useSettingsStore((s) => s.livePoll)
  const setLivePoll = useSettingsStore((s) => s.setLivePoll)

  const push = useToastStore((s) => s.push)
  const [providers, setProviders] = useState<NotifyProvider[]>([])
  const [testing, setTesting] = useState(false)
  const [loading, setLoading] = useState(true)
  const [cfg, setCfg] = useState<NotifyConfig | null>(null)
  const [draft, setDraft] = useState<Draft>(fromConfig(null))
  const [saving, setSaving] = useState(false)

  const refreshProviders = () => {
    void api
      .notifyProviders()
      .then(setProviders)
      .catch(() => undefined)
  }

  useEffect(() => {
    void api
      .notifyProviders()
      .then((p) => {
        setProviders(p)
        setLoading(false)
      })
      .catch(() => setLoading(false))
    void api
      .notifyConfig()
      .then((c) => {
        setCfg(c)
        setDraft(fromConfig(c))
      })
      .catch(() => undefined)
  }, [])

  const set = <K extends keyof Draft>(key: K, value: Draft[K]) =>
    setDraft((d) => ({ ...d, [key]: value }))

  const saveConfig = async () => {
    setSaving(true)
    const body: NotifyConfigUpdate = {
      ntfy_server: draft.ntfy_server,
      ntfy_topic: draft.ntfy_topic,
      discord_webhook: draft.discord_webhook,
      smtp_host: draft.smtp_host,
      smtp_port: draft.smtp_port,
      smtp_user: draft.smtp_user,
      smtp_from: draft.smtp_from,
      smtp_to: draft.smtp_to,
      smtp_insecure: draft.smtp_insecure,
      phone_webhook: draft.phone_webhook,
      phone_to: draft.phone_to,
    }
    if (draft.smtp_pass !== '') body.smtp_pass = draft.smtp_pass
    try {
      const saved = await api.notifyConfigSave(body)
      setCfg(saved)
      setDraft({ ...fromConfig(saved), smtp_pass: '' })
      refreshProviders()
      push({ kind: 'ok', title: 'Notification config saved', message: 'Channels reloaded; rules will alert on every configured channel.' })
    } catch (e) {
      push({ kind: 'danger', title: 'Save failed', message: e instanceof Error ? e.message : 'unknown error' })
    } finally {
      setSaving(false)
    }
  }

  const sendTest = async () => {
    setTesting(true)
    try {
      await api.notifyTest()
      push({ kind: 'ok', title: 'Test notification sent', message: 'All configured channels were pinged.' })
    } catch (e) {
      push({ kind: 'danger', title: 'Test failed', message: e instanceof Error ? e.message : 'no channels configured' })
    } finally {
      setTesting(false)
    }
  }

  return (
    <div className="stack">
      <PageHead index="09" title="Settings" meta="INTERFACE TWEAKS + NOTIFICATION CHANNELS" />

      <div className="grid grid--2">
        {/* interface tweaks */}
        <div className="panel">
          <div className="panel__head">
            <h3 className="panel__title">
              <IconSettings width={14} height={14} />
              Interface tweaks
            </h3>
          </div>
          <div className="panel__body">
            <div className="rule-builder" style={{ gap: 'var(--sp-4)' }}>
              <div className="tweak-row">
                <div>
                  <div className="k-label">Motion</div>
                  <div className="k-sec" style={{ fontSize: 'var(--fs-xs)', marginTop: 2 }}>
                    Full keeps live status pulses and one-shot draw effects; reduced kills animation entirely for snappier updates.
                  </div>
                </div>
                <Select
                  value={motion}
                  onChange={(e) => setMotion(e.target.value === 'reduced' ? 'reduced' : 'full')}
                  style={{ width: '10rem' }}
                >
                  <option value="full">Full</option>
                  <option value="reduced">Reduced</option>
                </Select>
              </div>

              <div className="tweak-row">
                <div>
                  <div className="k-label">Live polling</div>
                  <div className="k-sec" style={{ fontSize: 'var(--fs-xs)', marginTop: 2 }}>
                    When off, dashboards stop auto-refreshing and update only on manual refresh.
                  </div>
                </div>
                <Switch checked={livePoll} onChange={setLivePoll} label="" />
              </div>

              <div>
                <div className="k-label">Accent tone</div>
                <div className="k-sec" style={{ fontSize: 'var(--fs-xs)', marginTop: 2 }}>
                  Retint the primary accent across the whole console.
                </div>
                <div className="accent-row" style={{ display: 'flex', gap: 'var(--sp-2)', marginTop: 'var(--sp-3)' }}>
                  {ACCENTS.map((hex) => (
                    <button
                      key={hex}
                      className="accent-swatch"
                      style={{
                        background: hex,
                        boxShadow: accent === hex ? `0 0 0 2px var(--bg-panel), 0 0 0 4px ${hex}` : 'none',
                      }}
                      onClick={() => setAccent(hex)}
                      aria-label={`Accent ${hex}`}
                    />
                  ))}
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* notification channels */}
        <div className="panel">
          <div className="panel__head">
            <h3 className="panel__title">
              <IconBell width={14} height={14} />
              Notification channels
            </h3>
            <span className="k-mono k-tert" style={{ fontSize: 'var(--fs-2xs)' }}>
              {providers.filter((p) => p.configured).length}/{providers.length} READY
            </span>
          </div>
          <div className="panel__body--flush">
            {loading ? (
              <Empty title="Loading providers…" />
            ) : providers.length === 0 ? (
              <Empty title="No providers" hint="The backend registered none." />
            ) : (
              providers.map((p) => (
                <div key={p.name} className="rule-row">
                  <span className="notify-channel">
                    <span className="notify-channel__glyph">{CHANNEL_GLYPH[p.name] ?? p.name.slice(0, 3)}</span>
                    <span className="k-mono" style={{ fontWeight: 600, textTransform: 'uppercase' }}>
                      {p.name}
                    </span>
                  </span>
                  <span className="k-sec" style={{ fontSize: 'var(--fs-xs)', flex: 1 }}>
                    {p.configured ? 'Ready to send alerts' : 'Not configured — alerts are logged to the event feed only'}
                  </span>
                  {p.configured ? <Badge tone="ok" dot>configured</Badge> : <Badge tone="neutral">unconfigured</Badge>}
                </div>
              ))
            )}
            <div className="rule-row">
              <Button variant="ghost" size="sm" onClick={sendTest} disabled={testing}>
                {testing ? 'Sending…' : 'Send test notification'}
              </Button>
            </div>
          </div>
        </div>
      </div>

      {/* notification config editor */}
      <div className="panel">
        <div className="panel__head">
          <h3 className="panel__title">
            <IconBell width={14} height={14} />
            Channel configuration
          </h3>
          <span className="k-mono k-tert" style={{ fontSize: 'var(--fs-2xs)' }}>
            saved on the backend · secrets never leave it
          </span>
        </div>
        <div className="panel__body">
          <div className="notify-form">
            <div className="notify-form__col">
              <div className="k-label">ntfy</div>
              <Field label="Server" hint="e.g. https://ntfy.sh or your self-hosted URL">
                <Input value={draft.ntfy_server} placeholder="https://ntfy.sh" onChange={(e) => set('ntfy_server', e.target.value)} />
              </Field>
              <Field label="Topic">
                <Input value={draft.ntfy_topic} placeholder="homelab-alerts" onChange={(e) => set('ntfy_topic', e.target.value)} />
              </Field>

              <div className="k-label" style={{ marginTop: 'var(--sp-4)' }}>discord</div>
              <Field label="Webhook URL">
                <Input value={draft.discord_webhook} placeholder="https://discord.com/api/webhooks/…" onChange={(e) => set('discord_webhook', e.target.value)} />
              </Field>

              <div className="k-label" style={{ marginTop: 'var(--sp-4)' }}>phone</div>
              <Field label="Gateway webhook" hint={'receives {"to", "text"}'}>
                <Input value={draft.phone_webhook} placeholder="https://…" onChange={(e) => set('phone_webhook', e.target.value)} />
              </Field>
              <Field label="Destination number">
                <Input value={draft.phone_to} placeholder="+15551234567" onChange={(e) => set('phone_to', e.target.value)} />
              </Field>
            </div>

            <div className="notify-form__col">
              <div className="k-label">email (SMTP)</div>
              <div className="grid" style={{ gridTemplateColumns: '2fr 1fr', gap: 'var(--sp-2)' }}>
                <Field label="Host">
                  <Input value={draft.smtp_host} placeholder="smtp.example.com" onChange={(e) => set('smtp_host', e.target.value)} />
                </Field>
                <Field label="Port">
                  <Input
                    type="number"
                    value={draft.smtp_port}
                    onChange={(e) => set('smtp_port', Number(e.target.value) || 0)}
                  />
                </Field>
              </div>
              <div className="grid" style={{ gridTemplateColumns: '1fr 1fr', gap: 'var(--sp-2)' }}>
                <Field label="User">
                  <Input value={draft.smtp_user} autoComplete="off" onChange={(e) => set('smtp_user', e.target.value)} />
                </Field>
                <Field label="Password" hint={cfg?.smtp_pass_set ? 'stored · leave blank to keep' : undefined}>
                  <Input
                    type="password"
                    value={draft.smtp_pass}
                    autoComplete="new-password"
                    placeholder={cfg?.smtp_pass_set ? '••••••••' : ''}
                    onChange={(e) => set('smtp_pass', e.target.value)}
                  />
                </Field>
              </div>
              <div className="grid" style={{ gridTemplateColumns: '1fr 1fr', gap: 'var(--sp-2)' }}>
                <Field label="From">
                  <Input value={draft.smtp_from} placeholder="homelab@example.com" onChange={(e) => set('smtp_from', e.target.value)} />
                </Field>
                <Field label="To" hint="comma-separated">
                  <Input value={draft.smtp_to} placeholder="me@example.com" onChange={(e) => set('smtp_to', e.target.value)} />
                </Field>
              </div>
              <div className="tweak-row" style={{ marginTop: 'var(--sp-2)' }}>
                <div>
                  <div className="k-label">Allow insecure TLS</div>
                  <div className="k-sec" style={{ fontSize: 'var(--fs-xs)', marginTop: 2 }}>
                    Only needed for self-signed certificates on port 465.
                  </div>
                </div>
                <Switch checked={draft.smtp_insecure} onChange={(v) => set('smtp_insecure', v)} label="" />
              </div>
            </div>
          </div>

          <div className="rule-row" style={{ marginTop: 'var(--sp-3)', paddingLeft: 0, paddingRight: 0 }}>
            <Button variant="primary" size="sm" disabled={saving} onClick={() => void saveConfig()}>
              {saving ? 'Saving…' : 'Save channel config'}
            </Button>
            <span className="k-sec" style={{ fontSize: 'var(--fs-xs)' }}>
              Applies live — rules start alerting on every configured channel.
            </span>
          </div>
        </div>
      </div>

      <div className="panel" style={{ padding: 'var(--sp-3) var(--sp-4)' }}>
        <span className="k-sec" style={{ fontSize: 'var(--fs-sm)' }}>
          Channel values set here are stored on the backend and layered over any
          environment defaults (<span className="k-mono">NTFY_TOPIC</span>,{' '}
          <span className="k-mono">DISCORD_WEBHOOK</span>, <span className="k-mono">SMTP_HOST</span>…,{' '}
          <span className="k-mono">PHONE_WEBHOOK</span>). Secrets are never echoed back to the browser.
        </span>
      </div>
    </div>
  )
}