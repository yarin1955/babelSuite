import { useEffect, useMemo, useState } from 'react'
import {
  FaArrowRight, FaArrowsRotate, FaClock, FaCopy, FaPause,
  FaPlay, FaPlus, FaServer, FaTriangleExclamation,
} from 'react-icons/fa6'
import { useLocation, useNavigate } from 'react-router-dom'
import Layout from '../components/Layout'
import Page from '../components/Page'
import styles from './Agents.module.css'

const API = 'http://localhost:8090'

interface Agent {
  agent_id: string
  name: string
  desired_backend: string
  desired_platform: string
  desired_target_name: string
  desired_target_url: string
  platform: string
  backend: string
  target_name: string
  target_url: string
  capacity: number
  version: string
  labels: Record<string, string> | null
  last_contact: string
  last_work?: string | null
  no_schedule: boolean
  created_at: string
}

interface Draft {
  name: string
  desired_backend: string
  desired_platform: string
  desired_target_name: string
  desired_target_url: string
  capacity: number
  no_schedule: boolean
  labels: Array<{ k: string; v: string }>
}

type View = { kind: 'list' } | { kind: 'create' } | { kind: 'edit'; agent: Agent }

const BACKENDS = [
  { value: 'docker', label: 'Container', note: 'Available with the standard worker runtime today.' },
  { value: 'kubernetes', label: 'Orchestrated', note: 'Use this when the token is reserved for a managed runtime target.' },
  { value: 'local', label: 'Local', note: 'Useful for laptops and single-node validation hosts.' },
]

const authHeaders = () => ({
  'Content-Type': 'application/json',
  Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
})

const seedDraft = (agent?: Agent): Draft => {
  const labelRows = Object.entries(agent?.labels ?? {}).map(([k, v]) => ({ k, v }))
  return {
    name: agent?.name ?? '',
    desired_backend: agent?.desired_backend || 'docker',
    desired_platform: agent?.desired_platform ?? '',
    desired_target_name: agent?.desired_target_name ?? '',
    desired_target_url: agent?.desired_target_url ?? '',
    capacity: agent?.capacity && agent.capacity > 0 ? agent.capacity : 1,
    no_schedule: agent?.no_schedule ?? false,
    labels: labelRows.length > 0 ? labelRows : [{ k: '', v: '' }],
  }
}

const labelsFromDraft = (draft: Draft) => {
  const labels: Record<string, string> = {}
  draft.labels.forEach(({ k, v }) => {
    if (k.trim()) labels[k.trim()] = v.trim()
  })
  return labels
}

const timeAgo = (iso?: string | null) => {
  if (!iso) return 'never'
  const diff = Math.floor((Date.now() - new Date(iso).getTime()) / 1000)
  if (diff < 60) return `${diff}s ago`
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`
  return `${Math.floor(diff / 86400)}d ago`
}

const isRegistered = (agent: Agent) => Boolean(agent.backend || agent.platform || agent.version)
const isOnline = (agent: Agent) => isRegistered(agent) && Date.now() - new Date(agent.last_contact).getTime() < 60_000

function platformMatches(desired: string, actual: string) {
  if (!desired) return true
  if (!actual) return false
  return actual === desired || actual.startsWith(desired + '/')
}

function agentState(agent: Agent) {
  const desiredBackend = (agent.desired_backend || '').toLowerCase()
  const desiredPlatform = (agent.desired_platform || '').toLowerCase()
  const actualBackend = (agent.backend || '').toLowerCase()
  const actualPlatform = (agent.platform || '').toLowerCase()

  if (!isRegistered(agent)) {
    return { tone: 'pending', label: 'Awaiting registration', text: 'Token issued. Waiting for a worker to connect.' }
  }
  if ((desiredBackend && desiredBackend !== actualBackend) || (desiredPlatform && !platformMatches(desiredPlatform, actualPlatform))) {
    return { tone: 'warning', label: 'Runtime mismatch', text: 'Desired runtime does not match the registered worker.' }
  }
  if ((agent.desired_target_name && agent.desired_target_name !== agent.target_name) || (agent.desired_target_url && agent.desired_target_url !== agent.target_url)) {
    return { tone: 'warning', label: 'Target mismatch', text: 'The connected worker does not match the expected target name or URL.' }
  }
  if (agent.no_schedule) {
    return { tone: 'paused', label: 'Scheduling paused', text: 'Worker remains visible, but no new runs will be assigned.' }
  }
  if (isOnline(agent)) {
    return { tone: 'ready', label: 'Ready', text: 'Worker is online and eligible for scheduling.' }
  }
  return { tone: 'offline', label: 'Offline', text: 'Worker registered before, but has not heartbeated recently.' }
}

function bootstrap(draft: Draft, token: string) {
  if (!draft.desired_backend || draft.desired_backend === 'docker') {
    return {
      title: 'Bootstrap with the standard worker',
      note: 'This runtime matches the worker binary currently included with BabelSuite.',
      block: [
        'SERVER_URL=http://localhost:8090',
        `AGENT_TOKEN=${token}`,
        'BACKEND_ENGINE=docker',
        draft.desired_target_name ? `AGENT_TARGET_NAME=${draft.desired_target_name}` : null,
        draft.desired_target_url ? `AGENT_TARGET_URL=${draft.desired_target_url}` : null,
        `MAX_WORKFLOWS=${Math.max(1, draft.capacity)}`,
        'go run ./backend/cmd/agent',
      ].filter(Boolean).join('\n'),
    }
  }
  return {
    title: 'Reserved worker class',
    note: `Desired backend is ${draft.desired_backend}. Keep this token for a matching runtime class, or switch the desired backend back to the default container runtime before using the standard worker binary.`,
    block: [
        'SERVER_URL=http://localhost:8090',
        `AGENT_TOKEN=${token}`,
        `DESIRED_BACKEND=${draft.desired_backend}`,
        draft.desired_platform ? `DESIRED_PLATFORM=${draft.desired_platform}` : null,
        draft.desired_target_name ? `AGENT_TARGET_NAME=${draft.desired_target_name}` : null,
        draft.desired_target_url ? `AGENT_TARGET_URL=${draft.desired_target_url}` : null,
      ].filter(Boolean).join('\n'),
  }
}

export default function Agents() {
  const nav = useNavigate()
  const location = useLocation()
  const inSettingsFlow = location.pathname.startsWith('/settings/')
  const [agents, setAgents] = useState<Agent[]>([])
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState('')
  const [view, setView] = useState<View>({ kind: 'list' })
  const [saving, setSaving] = useState(false)
  const [newToken, setNewToken] = useState('')
  const [createdDraft, setCreatedDraft] = useState<Draft>(seedDraft())

  const metrics = useMemo(() => ({
    total: agents.length,
    online: agents.filter(isOnline).length,
    mismatched: agents.filter(agent => agentState(agent).tone === 'warning').length,
    paused: agents.filter(agent => agent.no_schedule).length,
  }), [agents])

  const load = async () => {
    setLoading(true)
    try {
      const res = await fetch(`${API}/api/agents`, { headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` } })
      const data = res.ok ? await res.json() : []
      setAgents(Array.isArray(data) ? data : [])
      setErr('')
    } catch {
      setErr('Failed to load agents')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [])

  const resetView = () => {
    setView({ kind: 'list' })
    setNewToken('')
    setCreatedDraft(seedDraft())
    setErr('')
  }

  const deleteAgent = async (id: string) => {
    if (!window.confirm('Delete this agent?')) return
    const res = await fetch(`${API}/api/agents/${id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` } })
    if (!res.ok) {
      const data = await res.json().catch(() => ({ error: 'Delete failed' }))
      setErr(data.error || 'Delete failed')
      return
    }
    void load()
  }

  const toolbar = view.kind === 'list'
    ? (
        <div className={styles.toolbarActions}>
          {inSettingsFlow && <button className='app-button app-button--secondary' onClick={() => nav('/settings')}>Back to Settings</button>}
          <button className='app-button app-button--primary' onClick={() => setView({ kind: 'create' })}><FaPlus />New agent</button>
        </div>
      )
    : <button className='app-button app-button--secondary' onClick={resetView}><FaArrowRight style={{ transform: 'rotate(180deg)' }} />Back</button>

  return (
    <Layout>
      <Page title='Agents' toolbar={toolbar}>
        {err && <div className='auth-error'>{err}</div>}

        <section className={styles.hero}>
          <div>
            <div className={styles.eyebrow}>Worker control plane</div>
            <h1>Set the runtime you want, then verify what actually connected.</h1>
            <p>Desired runtime and observed worker facts now live side by side, so it is easy to catch runtime mismatches before work lands on the wrong host.</p>
          </div>
            <div className={styles.metricGrid}>
              <Metric icon={<FaServer />} label='Registered agents' value={String(metrics.total)} />
              <Metric icon={<FaPlay />} label='Online workers' value={String(metrics.online)} />
              <Metric icon={<FaTriangleExclamation />} label='Mismatches' value={String(metrics.mismatched)} />
            <Metric icon={<FaPause />} label='Paused agents' value={String(metrics.paused)} />
          </div>
        </section>

        {view.kind === 'list' && (
          loading ? (
            <div className='panel-card'><div className='app-empty-state'>Loading agents...</div></div>
          ) : agents.length === 0 ? (
            <div className='panel-card'><div className='empty-state'><div className='empty-state__title'>No agents registered</div><div className='empty-state__desc'>Create an agent, choose the runtime class it belongs to, then attach the token to the worker host.</div></div></div>
          ) : (
            <div className={styles.agentGrid}>
              {agents.map(agent => (
                <AgentCard
                  key={agent.agent_id}
                  agent={agent}
                  onEdit={() => setView({ kind: 'edit', agent: { ...agent } })}
                  onDelete={() => void deleteAgent(agent.agent_id)}
                />
              ))}
            </div>
          )
        )}

        {view.kind === 'create' && (newToken
          ? <TokenReveal token={newToken} draft={createdDraft} onDone={resetView} />
          : <AgentForm
              title='Create agent'
              subtitle='Pick the worker class this token is meant for. Desired runtime is operator intent; observed runtime comes back from the worker after it connects.'
              draft={seedDraft()}
              saving={saving}
              onSubmit={async draft => {
                setSaving(true)
                try {
                  const body = {
                    name: draft.name.trim(),
                    desired_backend: draft.desired_backend,
                    desired_platform: draft.desired_platform.trim().toLowerCase(),
                    desired_target_name: draft.desired_target_name.trim(),
                    desired_target_url: draft.desired_target_url.trim(),
                    capacity: Math.max(1, draft.capacity),
                    no_schedule: draft.no_schedule,
                    labels: labelsFromDraft(draft),
                  }
                  const res = await fetch(`${API}/api/agents`, { method: 'POST', headers: authHeaders(), body: JSON.stringify(body) })
                  const data = await res.json().catch(() => ({}))
                  if (!res.ok) {
                    setErr(data.error || 'Create failed')
                    return
                  }
                  setCreatedDraft(draft)
                  setNewToken(data.token)
                  void load()
                } finally {
                  setSaving(false)
                }
              }}
            />
        )}

        {view.kind === 'edit' && (
          <AgentForm
            title='Edit agent'
            subtitle='Keep desired and observed runtime aligned so scheduling lands on the workers you expect.'
            draft={seedDraft(view.agent)}
            agent={view.agent}
            saving={saving}
            onSubmit={async draft => {
              setSaving(true)
              try {
                const body = {
                  name: draft.name.trim(),
                  desired_backend: draft.desired_backend,
                  desired_platform: draft.desired_platform.trim().toLowerCase(),
                  desired_target_name: draft.desired_target_name.trim(),
                  desired_target_url: draft.desired_target_url.trim(),
                  capacity: Math.max(1, draft.capacity),
                  no_schedule: draft.no_schedule,
                  labels: labelsFromDraft(draft),
                }
                const res = await fetch(`${API}/api/agents/${view.agent.agent_id}`, { method: 'PATCH', headers: authHeaders(), body: JSON.stringify(body) })
                const data = await res.json().catch(() => ({}))
                if (!res.ok) {
                  setErr(data.error || 'Save failed')
                  return
                }
                void load()
                resetView()
              } finally {
                setSaving(false)
              }
            }}
          />
        )}
      </Page>
    </Layout>
  )
}

function Metric({ icon, label, value }: { icon: React.ReactNode; label: string; value: string }) {
  return <div className={styles.metricCard}><div className={styles.metricIcon}>{icon}</div><div><div className={styles.metricLabel}>{label}</div><div className={styles.metricValue}>{value}</div></div></div>
}

function AgentCard({ agent, onEdit, onDelete }: { agent: Agent; onEdit: () => void; onDelete: () => void }) {
  const state = agentState(agent)
  return (
    <article className={styles.agentCard}>
      <div className={styles.agentHeader}>
        <div>
          <div className={styles.agentNameRow}>
            <strong>{agent.name}</strong>
            <span className={`${styles.statePill} ${styles[`statePill--${state.tone}`]}`}>{state.label}</span>
          </div>
          <p className={styles.agentSummary}>{state.text}</p>
        </div>
        <div className={styles.actions}>
          <button className='app-button app-button--secondary' onClick={onEdit}>Edit</button>
          <button className='app-button app-button--secondary' onClick={onDelete}>Delete</button>
        </div>
      </div>
      <div className={styles.runtimeGrid}>
        <RuntimeTile label='Desired backend' value={agent.desired_backend || 'not set'} accent />
        <RuntimeTile label='Observed backend' value={agent.backend || 'not reported'} />
        <RuntimeTile label='Desired platform' value={agent.desired_platform || 'not set'} accent />
        <RuntimeTile label='Observed platform' value={agent.platform || 'not reported'} />
        <RuntimeTile label='Desired target' value={agent.desired_target_name || agent.desired_target_url || 'not set'} accent />
        <RuntimeTile label='Observed target' value={agent.target_name || agent.target_url || 'not reported'} />
      </div>
      <div className={styles.metaRow}>
        <Meta icon={<FaServer />} label='capacity' value={String(agent.capacity || 1)} />
        <Meta icon={<FaClock />} label='last contact' value={timeAgo(agent.last_contact)} />
        <Meta icon={<FaArrowsRotate />} label='last work' value={timeAgo(agent.last_work)} />
        <Meta icon={<FaServer />} label='version' value={agent.version || 'pending'} />
      </div>
      {Object.keys(agent.labels ?? {}).length > 0 && <div className={styles.labelWrap}>{Object.entries(agent.labels ?? {}).map(([k, v]) => <span key={k} className={styles.labelChip}>{k}={v}</span>)}</div>}
    </article>
  )
}

function RuntimeTile({ label, value, accent }: { label: string; value: string; accent?: boolean }) {
  return <div className={`${styles.runtimeTile}${accent ? ` ${styles.runtimeTileAccent}` : ''}`}><span>{label}</span><strong>{value}</strong></div>
}

function Meta({ icon, label, value }: { icon: React.ReactNode; label: string; value: string }) {
  return <span className={styles.metaChip}>{icon}<span>{label}</span><strong>{value}</strong></span>
}

function AgentForm({
  title, subtitle, draft: initial, agent, saving, onSubmit,
}: {
  title: string
  subtitle: string
  draft: Draft
  agent?: Agent
  saving: boolean
  onSubmit: (draft: Draft) => Promise<void>
}) {
  const [draft, setDraft] = useState(initial)
  const state = agent ? agentState(agent) : null
  const needsKubernetesTarget = draft.desired_backend === 'kubernetes'
    && !draft.desired_target_name.trim()
    && !draft.desired_target_url.trim()

  return (
    <div className={styles.formShell}>
      <div className='panel-card'>
        <div className='panel-card__header panel-card__header--padded'><div><div className='panel-card__eyebrow'>{agent ? 'Agent controls' : 'Agent bootstrap'}</div><h3>{title}</h3></div></div>
        <div className={styles.formIntro}><p>{subtitle}</p></div>
        <form className={styles.form} onSubmit={async e => { e.preventDefault(); await onSubmit(draft) }}>
          <div className={styles.formGrid}>
            <Field label='Name'><input className={styles.fieldInput} value={draft.name} onChange={e => setDraft(curr => ({ ...curr, name: e.target.value }))} placeholder='validation-worker-emea-01' /></Field>
            <Field label='Desired backend' help='This is the runtime class you expect this token to be used for.'>
              <select className={styles.fieldInput} value={draft.desired_backend} onChange={e => setDraft(curr => ({ ...curr, desired_backend: e.target.value }))}>
                {BACKENDS.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            </Field>
            <Field label='Desired platform' help='Optional, for example linux/amd64 or linux/arm64.'><input className={styles.fieldInput} value={draft.desired_platform} onChange={e => setDraft(curr => ({ ...curr, desired_platform: e.target.value }))} placeholder='linux/amd64' /></Field>
            <Field label='Target name' help='Use a human-readable runtime target name.'><input className={styles.fieldInput} value={draft.desired_target_name} onChange={e => setDraft(curr => ({ ...curr, desired_target_name: e.target.value }))} placeholder='prod-eu-1' /></Field>
            <Field label='Target URL' help='Set the actual runtime server URL so multiple targets stay distinct.'><input className={styles.fieldInput} value={draft.desired_target_url} onChange={e => setDraft(curr => ({ ...curr, desired_target_url: e.target.value }))} placeholder='https://runtime-prod-eu-1.example.com' /></Field>
            <Field label='Capacity' help='How many workflows this worker should take at once.'><input className={styles.fieldInput} type='number' min={1} value={draft.capacity} onChange={e => setDraft(curr => ({ ...curr, capacity: Number(e.target.value) || 1 }))} /></Field>
          </div>
          <div className={styles.noteBox}>{BACKENDS.find(option => option.value === draft.desired_backend)?.note}</div>
          {needsKubernetesTarget && (
            <div className={styles.noteBox}>
              Set a target name or a target URL so multiple runtime targets stay distinct during scheduling.
            </div>
          )}
          <label className={styles.checkboxRow}><input type='checkbox' checked={draft.no_schedule} onChange={e => setDraft(curr => ({ ...curr, no_schedule: e.target.checked }))} /><span>Pause scheduling<small>Keep the worker visible, but block new runs from landing on it.</small></span></label>
          <div className={styles.labelsBlock}>
            <div className={styles.sectionLabel}>Labels</div>
            <LabelsEditor rows={draft.labels} onChange={rows => setDraft(curr => ({ ...curr, labels: rows }))} />
          </div>
          {agent && (
            <div className={styles.observedBox}>
              <div className={styles.sectionLabel}>Observed worker</div>
              <div className={styles.observedGrid}>
                <Observed label='State' value={state?.label || 'Unknown'} />
                <Observed label='Backend' value={agent.backend || 'not reported'} />
                <Observed label='Platform' value={agent.platform || 'not reported'} />
                <Observed label='Target name' value={agent.target_name || 'not reported'} />
                <Observed label='Target URL' value={agent.target_url || 'not reported'} />
                <Observed label='Version' value={agent.version || 'not reported'} />
                <Observed label='Last contact' value={agent.last_contact ? `${timeAgo(agent.last_contact)} (${new Date(agent.last_contact).toLocaleString()})` : 'never'} />
                <Observed label='Last work' value={agent.last_work ? `${timeAgo(agent.last_work)} (${new Date(agent.last_work).toLocaleString()})` : 'never'} />
              </div>
            </div>
          )}
          <div className={styles.formActions}><button className='app-button app-button--primary' type='submit' disabled={saving || !draft.name.trim() || needsKubernetesTarget}>{saving ? 'Saving...' : agent ? 'Save changes' : 'Create agent'}</button></div>
        </form>
      </div>
    </div>
  )
}

function Field({ label, help, children }: { label: string; help?: string; children: React.ReactNode }) {
  return <label className={styles.fieldGroup}><span className={styles.fieldLabel}>{label}</span>{children}{help && <span className={styles.fieldHelp}>{help}</span>}</label>
}

function Observed({ label, value }: { label: string; value: string }) {
  return <div className={styles.observedField}><span>{label}</span><strong>{value}</strong></div>
}

function LabelsEditor({ rows, onChange }: { rows: Array<{ k: string; v: string }>; onChange: (rows: Array<{ k: string; v: string }>) => void }) {
  const setValue = (index: number, field: 'k' | 'v', value: string) => {
    const next = [...rows]
    next[index] = { ...next[index], [field]: value }
    onChange(next)
  }
  return (
    <div className={styles.labelEditor}>
      {rows.map((row, index) => (
        <div key={`${index}-${row.k}`} className={styles.labelRow}>
          <input className={styles.fieldInput} value={row.k} onChange={e => setValue(index, 'k', e.target.value)} placeholder='region' />
          <input className={styles.fieldInput} value={row.v} onChange={e => setValue(index, 'v', e.target.value)} placeholder='emea' />
          <button className={styles.linkButton} type='button' onClick={() => onChange(rows.filter((_, i) => i !== index))}>Remove</button>
        </div>
      ))}
      <button className={styles.linkButton} type='button' onClick={() => onChange([...rows, { k: '', v: '' }])}>+ Add label</button>
    </div>
  )
}

function TokenReveal({ token, draft, onDone }: { token: string; draft: Draft; onDone: () => void }) {
  const [copied, setCopied] = useState(false)
  const info = bootstrap(draft, token)
  return (
    <div className={styles.formShell}>
      <div className='panel-card'>
        <div className='panel-card__header panel-card__header--padded'><div><div className='panel-card__eyebrow'>Agent created</div><h3>Store this token now</h3></div></div>
        <div className={styles.formIntro}>
          <p>This token is only returned once. Keep it with the worker host or secret that will register under the <strong>{draft.desired_backend}</strong> runtime class.</p>
          <div className={styles.tokenRow}>
            <code className={styles.tokenCode}>{token}</code>
            <button className='app-button app-button--secondary' onClick={async () => { try { await navigator.clipboard.writeText(token); setCopied(true); window.setTimeout(() => setCopied(false), 1500) } catch { setCopied(false) } }}><FaCopy />{copied ? 'Copied' : 'Copy'}</button>
          </div>
          <div className={styles.bootstrapBox}>
            <div className={styles.bootstrapTitle}>{info.title}</div>
            <p>{info.note}</p>
            <pre className={styles.bootstrapCode}>{info.block}</pre>
          </div>
          <div className={styles.formActions}><button className='app-button app-button--primary' onClick={onDone}>Done</button></div>
        </div>
      </div>
    </div>
  )
}
