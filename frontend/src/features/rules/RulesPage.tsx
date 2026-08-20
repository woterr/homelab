import { useMemo, useState } from 'react'
import { useRulesStore } from '../../state/useRulesStore'
import { useMetricsStore } from '../../state/useMetricsStore'
import { usePoll } from '../../lib/usePoll'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Field, Input, Select, Switch } from '../../components/ui/Inputs'
import { Modal } from '../../components/ui/Modal'
import { Empty } from '../../components/ui/Feedback'
import { PageHead } from '../../components/ui/PageHead'
import { IconPlus, IconTrash, IconAlert } from '../../components/ui/icons'
import { fmtDateTime, fmtRelative } from '../../lib/format'
import { useToastStore } from '../../state/useToastStore'
import type { Rule } from '../../lib/api'

const OPERATORS = [
  { value: 'gt', label: '> above', glyph: '>' },
  { value: 'gte', label: '≥ above or equal', glyph: '≥' },
  { value: 'lt', label: '< below', glyph: '<' },
  { value: 'lte', label: '≤ below or equal', glyph: '≤' },
  { value: 'eq', label: '= equal', glyph: '=' },
  { value: 'neq', label: '≠ not equal', glyph: '≠' },
]

const OP_GLYPH: Record<string, string> = Object.fromEntries(OPERATORS.map((o) => [o.value, o.glyph]))

interface Draft {
  name: string
  metric: string
  operator: string
  threshold: string
  duration_s: string
  notify: boolean
  message: string
}

const EMPTY: Draft = {
  name: '',
  metric: '',
  operator: 'gt',
  threshold: '80',
  duration_s: '60',
  notify: true,
  message: '',
}

export function RulesPage() {
  const rules = useRulesStore((s) => s.rules)
  const refresh = useRulesStore((s) => s.refresh)
  const create = useRulesStore((s) => s.create)
  const remove = useRulesStore((s) => s.remove)
  const series = useMetricsStore((s) => s.series)
  const refreshSeries = useMetricsStore((s) => s.refreshSeries)
  const push = useToastStore((s) => s.push)

  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState<Draft>(EMPTY)
  const [saving, setSaving] = useState(false)

  usePoll(() => Promise.all([refresh(), refreshSeries()]).then(() => undefined), 30_000)

  const metricOptions = useMemo(() => {
    const seen = new Set<string>()
    const out: { value: string; label: string }[] = []
    for (const s of series) {
      const name = typeof s?.name === 'string' ? s.name : ''
      if (!name) continue
      const base = name.split('{')[0]
      if (!seen.has(base)) {
        seen.add(base)
        out.push({ value: name, label: name })
      }
    }
    return out
  }, [series])

  const save = async () => {
    if (!draft.name.trim() || !draft.metric) {
      push({ kind: 'warn', title: 'Missing fields', message: 'Name and metric are required.' })
      return
    }
    const threshold = Number(draft.threshold)
    const duration_s = Number(draft.duration_s) || 60
    if (!Number.isFinite(threshold)) {
      push({ kind: 'warn', title: 'Bad threshold', message: 'Threshold must be a number.' })
      return
    }
    setSaving(true)
    try {
      await create({
        name: draft.name.trim(),
        metric: draft.metric,
        operator: draft.operator,
        threshold,
        duration_s,
        notify: draft.notify,
        message: draft.message.trim() || undefined,
      })
      setOpen(false)
      setDraft(EMPTY)
      push({ kind: 'ok', title: 'Rule created', message: `${draft.name.trim()} will be evaluated every tick.` })
    } catch (e) {
      push({ kind: 'danger', title: 'Could not create rule', message: e instanceof Error ? e.message : 'unknown error' })
    } finally {
      setSaving(false)
    }
  }

  const del = async (r: Rule) => {
    try {
      await remove(r.id)
      push({ kind: 'info', title: 'Rule removed', message: r.name })
    } catch (e) {
      push({ kind: 'danger', title: 'Could not remove rule', message: e instanceof Error ? e.message : 'unknown error' })
    }
  }

  return (
    <div className="stack">
      <PageHead
        index="08"
        title="Rules"
        meta={`${rules.length} AUTOMATIONS · EVALUATED EVERY TICK`}
        right={
          <Button variant="primary" icon={<IconPlus />} onClick={() => setOpen(true)}>
            New rule
          </Button>
        }
      />

      <div className="panel">
        <div className="panel__body--flush">
          {rules.length === 0 ? (
            <Empty
              title="No rules yet"
              hint="Create your first threshold rule — when a metric crosses it for the configured duration, an event fires and (optionally) a notification is sent."
              action={
                <Button variant="primary" size="sm" onClick={() => setOpen(true)}>
                  Create a rule
                </Button>
              }
            />
          ) : (
            rules.map((r) => (
              <div className="rule-row" key={r.id}>
                <span className="rule-row__when">WHEN</span>
                <div className="rule-row__cond">
                  <span className="metric">{r.metric}</span>
                  <span className="op">{OP_GLYPH[r.operator] ?? r.operator}</span>
                  <span className="val">{r.threshold}</span>
                  <span className="k-faint">for {r.duration_s}s</span>
                </div>
                <div className="rule-row__meta">
                  {r.notify && <IconAlert width={12} height={12} style={{ color: 'var(--warn)' }} />}
                  <Badge tone={r.notify ? 'accent' : 'neutral'} dot>
                    {r.notify ? 'notify' : 'event only'}
                  </Badge>
                  <span title={fmtDateTime(r.created_at)}>{fmtRelative(r.created_at)}</span>
                  <Button variant="ghost" size="sm" onClick={() => del(r)} aria-label="Delete rule">
                    <IconTrash />
                  </Button>
                </div>
              </div>
            ))
          )}
        </div>
      </div>

      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title="New rule"
        footer={
          <>
            <Button variant="ghost" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button variant="primary" onClick={save} disabled={saving}>
              {saving ? 'Saving…' : 'Create rule'}
            </Button>
          </>
        }
      >
        <Field label="Name">
          <Input
            value={draft.name}
            onChange={(e) => setDraft({ ...draft, name: e.target.value })}
            placeholder="e.g. CPU under sustained load"
          />
        </Field>
        <div className="rule-builder">
          <div className="rule-builder__row">
            <span className="rule-builder__tag rule-builder__tag--when">WHEN</span>
            <Field label="Metric">
              <Select value={draft.metric} onChange={(e) => setDraft({ ...draft, metric: e.target.value })}>
                <option value="">Select a metric…</option>
                {metricOptions.map((m) => (
                  <option key={m.value} value={m.value}>
                    {m.label}
                  </option>
                ))}
              </Select>
            </Field>
          </div>
          <div className="rule-builder__row">
            <span className="rule-builder__tag">THEN</span>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 'var(--sp-3)', width: '100%' }}>
              <Field label="Operator">
                <Select value={draft.operator} onChange={(e) => setDraft({ ...draft, operator: e.target.value })}>
                  {OPERATORS.map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </Select>
              </Field>
              <Field label="Threshold">
                <Input
                  type="number"
                  step="any"
                  value={draft.threshold}
                  onChange={(e) => setDraft({ ...draft, threshold: e.target.value })}
                />
              </Field>
            </div>
          </div>
          <div className="rule-builder__row">
            <span className="rule-builder__tag" style={{ visibility: 'hidden' }}>
              ·
            </span>
            <Field label="Must persist for" hint="The condition has to hold continuously for this long before it fires.">
              <Input
                type="number"
                min={1}
                value={draft.duration_s}
                onChange={(e) => setDraft({ ...draft, duration_s: e.target.value })}
              />
            </Field>
          </div>
          <div className="rule-builder__row">
            <span className="rule-builder__tag" style={{ visibility: 'hidden' }}>
              ·
            </span>
            <Switch checked={draft.notify} onChange={(v) => setDraft({ ...draft, notify: v })} label="Send notification when it fires" />
          </div>
          <div className="rule-builder__row">
            <span className="rule-builder__tag" style={{ visibility: 'hidden' }}>
              ·
            </span>
            <Field label="Message (optional)" hint="Overrides the default alert text.">
              <Input value={draft.message} onChange={(e) => setDraft({ ...draft, message: e.target.value })} placeholder="Server is hot!" />
            </Field>
          </div>
        </div>
      </Modal>
    </div>
  )
}