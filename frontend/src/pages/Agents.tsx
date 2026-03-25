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
  runtime_target_id: string
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

interface RuntimeTarget {
  runtime_target_id: string
  name: string
  backend: string
  platform: string
  endpoint_url: string
  namespace: string
  insecure_skip_tls_verify?: boolean
  username?: string
  has_password?: boolean
  has_bearer_token?: boolean
  has_tls_ca_data?: boolean
  has_tls_cert_data?: boolean
  has_tls_key_data?: boolean
  labels: Record<string, string> | null
  created_at: string
  updated_at: string
  runner_backend?: string
  worker_bootstrap_supported?: boolean
  worker_bootstrap_reason?: string
}

interface Draft {
  name: string
  runtime_target_id: string
  capacity: number
  no_schedule: boolean
  labels: Array<{ k: string; v: string }>
}

interface TargetDraft {
  name: string
  backend: string
  platform: string
  endpoint_url: string
  namespace: string
  insecure_skip_tls_verify: boolean
  username: string
  password: string
  bearer_token: string
  tls_ca_data: string
  tls_cert_data: string
  tls_key_data: string
  clear_password: boolean
  clear_bearer_token: boolean
  clear_tls_ca_data: boolean
  clear_tls_cert_data: boolean
  clear_tls_key_data: boolean
  labels: Array<{ k: string; v: string }>
}

type View =
  | { kind: 'list' }
  | { kind: 'create-agent' }
  | { kind: 'edit-agent'; agent: Agent }
  | { kind: 'create-target' }
  | { kind: 'edit-target'; target: RuntimeTarget }

const TARGET_TYPES = [
  {
    value: 'docker',
    label: 'Container host',
    note: 'Bind this destination to one daemon or socket so worker tokens always land on the right host.',
    endpointLabel: 'Daemon URL or socket',
    endpointHelp: 'Examples: unix:///var/run/docker.sock, npipe:////./pipe/docker_engine, tcp://10.0.0.20:2376',
    endpointPlaceholder: 'unix:///var/run/docker.sock',
  },
  {
    value: 'kubernetes',
    label: 'Cluster',
    note: 'Record one cluster API server and namespace so managers know exactly which cluster this destination means.',
    endpointLabel: 'Cluster API server',
    endpointHelp: 'Example: https://cluster.example.internal',
    endpointPlaceholder: 'https://cluster.example.internal',
  },
] as const

const LEGACY_TARGET_TYPE = {
  value: 'local',
  label: 'Local host',
  note: 'Use the container runtime available on the worker machine itself.',
  endpointLabel: 'Daemon URL or socket',
  endpointHelp: 'Leave blank to use the local default runtime on the worker machine.',
  endpointPlaceholder: 'unix:///var/run/docker.sock',
} as const

const targetTypesByValue = new Map(
  [...TARGET_TYPES, LEGACY_TARGET_TYPE].map(option => [option.value, option]),
)

const authHeaders = () => ({
  'Content-Type': 'application/json',
  Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
})

const seedDraft = (agent?: Agent): Draft => {
  const labelRows = Object.entries(agent?.labels ?? {}).map(([k, v]) => ({ k, v }))
  return {
    name: agent?.name ?? '',
    runtime_target_id: agent?.runtime_target_id ?? '',
    capacity: agent?.capacity && agent.capacity > 0 ? agent.capacity : 1,
    no_schedule: agent?.no_schedule ?? false,
    labels: labelRows.length > 0 ? labelRows : [{ k: '', v: '' }],
  }
}

const seedTargetDraft = (target?: RuntimeTarget): TargetDraft => {
  const rows = Object.entries(target?.labels ?? {}).map(([k, v]) => ({ k, v }))
  return {
    name: target?.name ?? '',
    backend: target?.backend || 'docker',
    platform: target?.platform ?? '',
    endpoint_url: target?.endpoint_url ?? '',
    namespace: target?.namespace ?? '',
    insecure_skip_tls_verify: target?.insecure_skip_tls_verify ?? false,
    username: target?.username ?? '',
    password: '',
    bearer_token: '',
    tls_ca_data: '',
    tls_cert_data: '',
    tls_key_data: '',
    clear_password: false,
    clear_bearer_token: false,
    clear_tls_ca_data: false,
    clear_tls_cert_data: false,
    clear_tls_key_data: false,
    labels: rows.length > 0 ? rows : [{ k: '', v: '' }],
  }
}

const labelsFromDraft = (draft: Draft) => {
  const labels: Record<string, string> = {}
  draft.labels.forEach(({ k, v }) => {
    if (k.trim()) labels[k.trim()] = v.trim()
  })
  return labels
}

const labelsFromRows = (rows: Array<{ k: string; v: string }>) => {
  const labels: Record<string, string> = {}
  rows.forEach(({ k, v }) => {
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
    return { tone: 'warning', label: 'Runtime mismatch', text: 'The connected worker does not match the assigned runtime target.' }
  }
  if ((agent.desired_target_name && agent.desired_target_name !== agent.target_name) || (agent.desired_target_url && agent.desired_target_url !== agent.target_url)) {
    return { tone: 'warning', label: 'Target mismatch', text: 'The worker registered against a different target than expected.' }
  }
  if (agent.no_schedule) {
    return { tone: 'paused', label: 'Scheduling paused', text: 'Worker remains visible, but no new runs will be assigned.' }
  }
  if (isOnline(agent)) {
    return { tone: 'ready', label: 'Ready', text: 'Worker is online and eligible for scheduling.' }
  }
  return { tone: 'offline', label: 'Offline', text: 'Worker registered before, but has not heartbeated recently.' }
}

function targetType(value: string) {
  return targetTypesByValue.get(value as 'docker' | 'kubernetes' | 'local') || targetTypesByValue.get('docker')!
}

function targetSupportsWorkers(target: RuntimeTarget) {
  if (typeof target.worker_bootstrap_supported === 'boolean') {
    return target.worker_bootstrap_supported
  }
  return target.backend === 'docker' || target.backend === 'local'
}

function targetSupportReason(target: RuntimeTarget) {
  if (target.worker_bootstrap_reason) return target.worker_bootstrap_reason
  if (targetSupportsWorkers(target)) return ''
  return 'This destination is recorded for control-plane configuration only right now.'
}

function targetEndpointSummary(target: RuntimeTarget) {
  if (target.backend === 'kubernetes') {
    return target.endpoint_url || 'Cluster API server not recorded yet.'
  }
  return target.endpoint_url || 'Uses the daemon available on the worker machine.'
}

function targetSecuritySummary(target: RuntimeTarget) {
  const parts: string[] = []
  if (target.insecure_skip_tls_verify) parts.push('TLS verify disabled')
  if (target.username) parts.push(`Basic auth user ${target.username}`)
  if (target.has_password) parts.push('Password stored')
  if (target.has_bearer_token) parts.push('Bearer token stored')
  if (target.has_tls_ca_data) parts.push('Custom CA stored')
  if (target.has_tls_cert_data && target.has_tls_key_data) parts.push('Client certificate stored')
  if (parts.length === 0) return 'No TLS or credentials stored.'
  return parts.join(' · ')
}

function bootstrap(draft: Draft, target: RuntimeTarget | undefined, token: string) {
  if (!target) {
    return {
      title: 'Store this token now',
      note: 'Pick a runtime target before provisioning the worker that will use this token.',
      block: token,
    }
  }

  if (targetSupportsWorkers(target)) {
    const assignedDaemon = target.endpoint_url || 'worker local default'
    return {
      title: 'Bootstrap this worker',
      note: `This token is bound to ${target.name}. When the worker starts, it will connect to ${assignedDaemon} with the TLS and auth settings stored on that destination.`,
      block: [
        'SERVER_URL=http://localhost:8090',
        `AGENT_TOKEN=${token}`,
        `MAX_WORKFLOWS=${Math.max(1, draft.capacity)}`,
        'go run ./backend/cmd/agent',
      ].join('\n'),
    }
  }
  return {
    title: 'Destination saved',
    note: targetSupportReason(target),
    block: [
      'SERVER_URL=http://localhost:8090',
      `AGENT_TOKEN=${token}`,
      `MAX_WORKFLOWS=${Math.max(1, draft.capacity)}`,
    ].join('\n'),
  }
}

export default function Agents() {
  const nav = useNavigate()
  const location = useLocation()
  const inSettingsFlow = location.pathname.startsWith('/settings/')
  const [agents, setAgents] = useState<Agent[]>([])
  const [targets, setTargets] = useState<RuntimeTarget[]>([])
  const [loading, setLoading] = useState(true)
  const [err, setErr] = useState('')
  const [view, setView] = useState<View>({ kind: 'list' })
  const [saving, setSaving] = useState(false)
  const [newToken, setNewToken] = useState('')
  const [createdDraft, setCreatedDraft] = useState<Draft>(seedDraft())
  const targetsByID = useMemo(() => new Map(targets.map(target => [target.runtime_target_id, target])), [targets])
  const linkedCounts = useMemo(() => {
    const counts = new Map<string, number>()
    agents.forEach(agent => {
      if (!agent.runtime_target_id) return
      counts.set(agent.runtime_target_id, (counts.get(agent.runtime_target_id) || 0) + 1)
    })
    return counts
  }, [agents])

  const metrics = useMemo(() => ({
    total: agents.length,
    online: agents.filter(isOnline).length,
    mismatched: agents.filter(agent => agentState(agent).tone === 'warning').length,
    paused: agents.filter(agent => agent.no_schedule).length,
  }), [agents])

  const load = async () => {
    setLoading(true)
    try {
      const [agentsRes, targetsRes] = await Promise.all([
        fetch(`${API}/api/agents`, { headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` } }),
        fetch(`${API}/api/runtime-targets`, { headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` } }),
      ])
      const [agentsData, targetsData] = await Promise.all([
        agentsRes.ok ? agentsRes.json() : [],
        targetsRes.ok ? targetsRes.json() : [],
      ])
      setAgents(Array.isArray(agentsData) ? agentsData : [])
      setTargets(Array.isArray(targetsData) ? targetsData : [])
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

  const deleteTarget = async (id: string) => {
    if (!window.confirm('Delete this runtime target?')) return
    const res = await fetch(`${API}/api/runtime-targets/${id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` } })
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
          <button className='app-button app-button--secondary' onClick={() => setView({ kind: 'create-target' })}><FaPlus />New runtime target</button>
          <button className='app-button app-button--primary' onClick={() => setView({ kind: 'create-agent' })}><FaPlus />New agent</button>
        </div>
      )
    : <button className='app-button app-button--secondary' onClick={resetView}><FaArrowRight style={{ transform: 'rotate(180deg)' }} />Back</button>

  return (
    <Layout>
      <Page title='Agents' toolbar={toolbar}>
        {err && <div className='auth-error'>{err}</div>}

        <div className={styles.metricGrid}>
          <Metric icon={<FaServer />} label='Registered agents' value={String(metrics.total)} />
          <Metric icon={<FaPlay />} label='Online workers' value={String(metrics.online)} />
          <Metric icon={<FaTriangleExclamation />} label='Mismatches' value={String(metrics.mismatched)} />
          <Metric icon={<FaPause />} label='Paused agents' value={String(metrics.paused)} />
        </div>

        {view.kind === 'list' && (
          loading ? (
            <div className='panel-card'><div className='app-empty-state'>Loading agent controls...</div></div>
          ) : (
            <div className={styles.pageStack}>
              <section className={styles.sectionShell}>
                <div className={styles.sectionHeader}>
                  <div>
                    <div className={styles.sectionLabel}>Runtime targets</div>
                    <h2>Named destinations</h2>
                    <p>Define container hosts and cluster records once, then reuse them across worker tokens and scheduling rules.</p>
                  </div>
                  <button className='app-button app-button--secondary' onClick={() => setView({ kind: 'create-target' })}><FaPlus />New runtime target</button>
                </div>
                {targets.length === 0 ? (
                  <div className={styles.emptyCard}><div className='empty-state'><div className='empty-state__title'>No runtime targets yet</div><div className='empty-state__desc'>Create a named destination first so teams can point workers to one daemon or record one exact cluster.</div></div></div>
                ) : (
                  <div className={styles.targetGrid}>
                    {targets.map(target => (
                      <TargetCard
                        key={target.runtime_target_id}
                        target={target}
                        linkedAgents={linkedCounts.get(target.runtime_target_id) || 0}
                        onEdit={() => setView({ kind: 'edit-target', target: { ...target } })}
                        onDelete={() => void deleteTarget(target.runtime_target_id)}
                      />
                    ))}
                  </div>
                )}
              </section>
              <section className={styles.sectionShell}>
                <div className={styles.sectionHeader}>
                  <div>
                    <div className={styles.sectionLabel}>Worker tokens</div>
                    <h2>Assigned agents</h2>
                    <p>Issue tokens against container hosts, then verify the daemon and platform each worker reports after it connects.</p>
                  </div>
                  <button className='app-button app-button--primary' onClick={() => setView({ kind: 'create-agent' })}><FaPlus />New agent</button>
                </div>
                {agents.length === 0 ? (
                  <div className={styles.emptyCard}><div className='empty-state'><div className='empty-state__title'>No agents registered</div><div className='empty-state__desc'>Create an agent token, bind it to a container host, and attach it to the worker machine that should run suite steps.</div></div></div>
                ) : (
                  <div className={styles.agentGrid}>
                    {agents.map(agent => (
                      <AgentCard
                        key={agent.agent_id}
                        agent={agent}
                        target={agent.runtime_target_id ? targetsByID.get(agent.runtime_target_id) : undefined}
                        onEdit={() => setView({ kind: 'edit-agent', agent: { ...agent } })}
                        onDelete={() => void deleteAgent(agent.agent_id)}
                      />
                    ))}
                  </div>
                )}
              </section>
            </div>
          )
        )}

        {view.kind === 'create-target' && (
          <TargetForm
            title='Create runtime target'
            subtitle='Define one container host or one cluster record, then reuse it across the workspace.'
            draft={seedTargetDraft()}
            target={undefined}
            saving={saving}
            onSubmit={async draft => {
              setSaving(true)
              try {
                const res = await fetch(`${API}/api/runtime-targets`, {
                  method: 'POST',
                  headers: authHeaders(),
                  body: JSON.stringify({
                    name: draft.name.trim(),
                    backend: draft.backend,
                    platform: draft.platform.trim().toLowerCase(),
                    endpoint_url: draft.endpoint_url.trim(),
                    namespace: draft.namespace.trim(),
                    insecure_skip_tls_verify: draft.insecure_skip_tls_verify,
                    username: draft.username.trim(),
                    password: draft.password,
                    bearer_token: draft.bearer_token,
                    tls_ca_data: draft.tls_ca_data,
                    tls_cert_data: draft.tls_cert_data,
                    tls_key_data: draft.tls_key_data,
                    clear_password: draft.clear_password,
                    clear_bearer_token: draft.clear_bearer_token,
                    clear_tls_ca_data: draft.clear_tls_ca_data,
                    clear_tls_cert_data: draft.clear_tls_cert_data,
                    clear_tls_key_data: draft.clear_tls_key_data,
                    labels: labelsFromRows(draft.labels),
                  }),
                })
                const data = await res.json().catch(() => ({}))
                if (!res.ok) {
                  setErr(data.error || 'Create failed')
                  return
                }
                await load()
                resetView()
              } finally {
                setSaving(false)
              }
            }}
          />
        )}

        {view.kind === 'edit-target' && (
          <TargetForm
            title='Edit runtime target'
            subtitle='Update the destination definition managers expect workers and schedulers to reference.'
            draft={seedTargetDraft(view.target)}
            target={view.target}
            saving={saving}
            onSubmit={async draft => {
              setSaving(true)
              try {
                const res = await fetch(`${API}/api/runtime-targets/${view.target.runtime_target_id}`, {
                  method: 'PUT',
                  headers: authHeaders(),
                  body: JSON.stringify({
                    name: draft.name.trim(),
                    backend: draft.backend,
                    platform: draft.platform.trim().toLowerCase(),
                    endpoint_url: draft.endpoint_url.trim(),
                    namespace: draft.namespace.trim(),
                    insecure_skip_tls_verify: draft.insecure_skip_tls_verify,
                    username: draft.username.trim(),
                    password: draft.password,
                    bearer_token: draft.bearer_token,
                    tls_ca_data: draft.tls_ca_data,
                    tls_cert_data: draft.tls_cert_data,
                    tls_key_data: draft.tls_key_data,
                    clear_password: draft.clear_password,
                    clear_bearer_token: draft.clear_bearer_token,
                    clear_tls_ca_data: draft.clear_tls_ca_data,
                    clear_tls_cert_data: draft.clear_tls_cert_data,
                    clear_tls_key_data: draft.clear_tls_key_data,
                    labels: labelsFromRows(draft.labels),
                  }),
                })
                const data = await res.json().catch(() => ({}))
                if (!res.ok) {
                  setErr(data.error || 'Save failed')
                  return
                }
                await load()
                resetView()
              } finally {
                setSaving(false)
              }
            }}
          />
        )}

        {view.kind === 'create-agent' && (newToken
          ? <TokenReveal token={newToken} draft={createdDraft} target={targetsByID.get(createdDraft.runtime_target_id)} onDone={resetView} />
          : <AgentForm
              title='Create agent'
              subtitle='Bind this token to one named container host. The worker will report the daemon and platform it actually connects to after startup.'
              draft={seedDraft()}
              targets={targets}
              saving={saving}
              onSubmit={async draft => {
                setSaving(true)
                try {
                  const body = {
                    name: draft.name.trim(),
                    runtime_target_id: draft.runtime_target_id,
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

        {view.kind === 'edit-agent' && (
          <AgentForm
            title='Edit agent'
            subtitle='Keep each worker bound to the right container host so scheduling lands on the daemon you expect.'
            draft={seedDraft(view.agent)}
            agent={view.agent}
            targets={targets}
            saving={saving}
            onSubmit={async draft => {
              setSaving(true)
              try {
                const body = {
                  name: draft.name.trim(),
                  runtime_target_id: draft.runtime_target_id,
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

function TargetCard({
  target, linkedAgents, onEdit, onDelete,
}: {
  target: RuntimeTarget
  linkedAgents: number
  onEdit: () => void
  onDelete: () => void
}) {
  const type = targetType(target.backend)
  const supportsWorkers = targetSupportsWorkers(target)
  return (
    <article className={styles.targetCard}>
      <div className={styles.agentHeader}>
        <div>
          <div className={styles.agentNameRow}>
            <strong>{target.name}</strong>
            <span className={`${styles.statePill} ${styles[supportsWorkers ? 'statePill--ready' : 'statePill--planned']}`}>{type.label}</span>
          </div>
          <p className={styles.agentSummary}>{targetEndpointSummary(target)}</p>
        </div>
        <div className={styles.actions}>
          <button className='app-button app-button--secondary' onClick={onEdit}>Edit</button>
          <button className='app-button app-button--secondary' onClick={onDelete}>Delete</button>
        </div>
      </div>
      <div className={styles.runtimeGrid}>
        <RuntimeTile label='Type' value={type.label} accent />
        <RuntimeTile label='Runner' value={target.runner_backend || target.backend} />
        <RuntimeTile label='Platform' value={target.platform || 'not set'} />
        <RuntimeTile label={target.backend === 'kubernetes' ? 'Cluster API server' : 'Daemon'} value={target.endpoint_url || 'worker default'} accent />
        <RuntimeTile label='Namespace' value={target.namespace || 'not set'} />
      </div>
      <div className={styles.noteBox}>{targetSecuritySummary(target)}</div>
      {!supportsWorkers && <div className={styles.noteBox}>{targetSupportReason(target)}</div>}
      <div className={styles.metaRow}>
        <Meta icon={<FaServer />} label='linked agents' value={String(linkedAgents)} />
        <Meta icon={<FaArrowsRotate />} label='updated' value={timeAgo(target.updated_at)} />
      </div>
      {Object.keys(target.labels ?? {}).length > 0 && <div className={styles.labelWrap}>{Object.entries(target.labels ?? {}).map(([k, v]) => <span key={k} className={styles.labelChip}>{k}={v}</span>)}</div>}
    </article>
  )
}

function AgentCard({
  agent, target, onEdit, onDelete,
}: {
  agent: Agent
  target?: RuntimeTarget
  onEdit: () => void
  onDelete: () => void
}) {
  const state = agentState(agent)
  const assignedTarget = target?.name || agent.desired_target_name || agent.desired_target_url || 'not set'
  const observedTarget = agent.target_name || agent.target_url || 'not reported'
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
        <RuntimeTile label='Assigned target' value={assignedTarget} accent />
        <RuntimeTile label='Observed target' value={observedTarget} />
        <RuntimeTile label='Expected runner' value={target?.runner_backend || agent.desired_backend || 'not set'} accent />
        <RuntimeTile label='Observed backend' value={agent.backend || 'not reported'} />
        <RuntimeTile label='Desired platform' value={agent.desired_platform || 'not set'} accent />
        <RuntimeTile label='Observed platform' value={agent.platform || 'not reported'} />
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
  title, subtitle, draft: initial, agent, targets, saving, onSubmit,
}: {
  title: string
  subtitle: string
  draft: Draft
  agent?: Agent
  targets: RuntimeTarget[]
  saving: boolean
  onSubmit: (draft: Draft) => Promise<void>
}) {
  const [draft, setDraft] = useState(initial)
  const selectableTargets = useMemo(() => {
    return targets.filter(item => targetSupportsWorkers(item) || item.runtime_target_id === draft.runtime_target_id)
  }, [targets, draft.runtime_target_id])
  const target = targets.find(item => item.runtime_target_id === draft.runtime_target_id)
  const state = agent ? agentState(agent) : null
  const targetRequired = !draft.runtime_target_id.trim()
  const cannotSave = saving || !draft.name.trim() || targetRequired || selectableTargets.length === 0 || (target ? !targetSupportsWorkers(target) : false)
  const clusterTargets = targets.filter(item => !targetSupportsWorkers(item))

  return (
    <div className={styles.formShell}>
      <div className='panel-card'>
        <div className='panel-card__header panel-card__header--padded'><div><div className='panel-card__eyebrow'>{agent ? 'Agent controls' : 'Agent bootstrap'}</div><h3>{title}</h3></div></div>
        <div className={styles.formIntro}><p>{subtitle}</p></div>
        <form className={styles.form} onSubmit={async e => { e.preventDefault(); await onSubmit(draft) }}>
          <div className={styles.formGrid}>
            <Field label='Name'><input className={styles.fieldInput} value={draft.name} onChange={e => setDraft(curr => ({ ...curr, name: e.target.value }))} placeholder='validation-worker-emea-01' /></Field>
            <Field label='Runtime target' help='Choose the named container host this token belongs to.'>
              <select className={styles.fieldInput} value={draft.runtime_target_id} onChange={e => setDraft(curr => ({ ...curr, runtime_target_id: e.target.value }))}>
                <option value=''>Select a runtime target</option>
                {selectableTargets.map(item => <option key={item.runtime_target_id} value={item.runtime_target_id}>{item.name}</option>)}
              </select>
            </Field>
            <Field label='Capacity' help='How many workflows this worker should take at once.'><input className={styles.fieldInput} type='number' min={1} value={draft.capacity} onChange={e => setDraft(curr => ({ ...curr, capacity: Number(e.target.value) || 1 }))} /></Field>
          </div>
          {selectableTargets.length === 0 && <div className={styles.noteBox}>Create a container host destination before creating agent tokens so workers can be assigned to a specific daemon.</div>}
          {clusterTargets.length > 0 && <div className={styles.noteBox}>Cluster destinations are recorded here too, but worker tokens can only be issued for container hosts right now.</div>}
          {targetRequired && selectableTargets.length > 0 && <div className={styles.noteBox}>Select a runtime target so the token is bound to one specific daemon.</div>}
          {target && !targetSupportsWorkers(target) && <div className={styles.noteBox}>{targetSupportReason(target)}</div>}
          {target && <TargetSummary target={target} />}
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
          <div className={styles.formActions}><button className='app-button app-button--primary' type='submit' disabled={cannotSave}>{saving ? 'Saving...' : agent ? 'Save changes' : 'Create agent'}</button></div>
        </form>
      </div>
    </div>
  )
}

function TargetForm({
  title, subtitle, draft: initial, target, saving, onSubmit,
}: {
  title: string
  subtitle: string
  draft: TargetDraft
  target?: RuntimeTarget
  saving: boolean
  onSubmit: (draft: TargetDraft) => Promise<void>
}) {
  const [draft, setDraft] = useState(initial)
  const options = draft.backend === LEGACY_TARGET_TYPE.value ? [...TARGET_TYPES, LEGACY_TARGET_TYPE] : TARGET_TYPES
  const type = targetType(draft.backend)
  const needsEndpoint = draft.backend === 'kubernetes' && !draft.endpoint_url.trim()
  const hasStoredClientCert = Boolean(target?.has_tls_cert_data && target?.has_tls_key_data)
  return (
    <div className={styles.formShell}>
      <div className='panel-card'>
        <div className='panel-card__header panel-card__header--padded'><div><div className='panel-card__eyebrow'>Runtime target</div><h3>{title}</h3></div></div>
        <div className={styles.formIntro}><p>{subtitle}</p></div>
        <form className={styles.form} onSubmit={async e => { e.preventDefault(); await onSubmit(draft) }}>
          <div className={styles.formGrid}>
            <Field label='Name'><input className={styles.fieldInput} value={draft.name} onChange={e => setDraft(curr => ({ ...curr, name: e.target.value }))} placeholder='k8s-prod-eu' /></Field>
            <Field label='Destination type' help='Choose whether this record points to one container host or one cluster.'>
              <select className={styles.fieldInput} value={draft.backend} onChange={e => setDraft(curr => ({ ...curr, backend: e.target.value }))}>
                {options.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            </Field>
            <Field label='Platform' help='Optional, for example linux/amd64 or linux/arm64.'><input className={styles.fieldInput} value={draft.platform} onChange={e => setDraft(curr => ({ ...curr, platform: e.target.value }))} placeholder='linux/amd64' /></Field>
            <Field label={type.endpointLabel} help={type.endpointHelp}><input className={styles.fieldInput} value={draft.endpoint_url} onChange={e => setDraft(curr => ({ ...curr, endpoint_url: e.target.value }))} placeholder={type.endpointPlaceholder} /></Field>
            <Field label='Namespace' help='Optional scope for shared cluster targets.'><input className={styles.fieldInput} value={draft.namespace} onChange={e => setDraft(curr => ({ ...curr, namespace: e.target.value }))} placeholder='validation-runs' /></Field>
          </div>
          <div className={styles.noteBox}>{type.note}</div>
          {draft.backend === 'kubernetes' && <div className={styles.noteBox}>Cluster destinations are recorded here for routing and visibility, but worker execution is not available for them yet.</div>}
          {needsEndpoint && <div className={styles.noteBox}>Cluster destinations need an API server URL so teams can tell exactly which cluster this record points to.</div>}
          <div className={styles.labelsBlock}>
            <div className={styles.sectionLabel}>Connection security</div>
            <div className={styles.formGrid}>
              <label className={styles.checkboxRow}>
                <input type='checkbox' checked={draft.insecure_skip_tls_verify} onChange={e => setDraft(curr => ({ ...curr, insecure_skip_tls_verify: e.target.checked }))} />
                <span>Skip TLS verification<small>Use only when the endpoint uses a self-signed or otherwise untrusted certificate.</small></span>
              </label>
              <Field label='Basic auth username' help='Optional. Pair this with a stored password if the endpoint is behind basic auth.'>
                <input className={styles.fieldInput} value={draft.username} onChange={e => setDraft(curr => ({ ...curr, username: e.target.value }))} placeholder='runtime-user' />
              </Field>
              <Field label='Password' help={target?.has_password ? 'Leave blank to keep the stored password, or enter a new value to replace it.' : 'Optional. Stored only for the worker bootstrap path.'}>
                <input className={styles.fieldInput} type='password' value={draft.password} onChange={e => setDraft(curr => ({ ...curr, password: e.target.value, clear_password: false }))} placeholder={target?.has_password ? 'Stored separately' : 'Optional'} />
              </Field>
              {target?.has_password && (
                <label className={styles.checkboxRow}>
                  <input type='checkbox' checked={draft.clear_password} onChange={e => setDraft(curr => ({ ...curr, clear_password: e.target.checked, password: e.target.checked ? '' : curr.password }))} />
                  <span>Clear stored password<small>Remove the saved password from this destination.</small></span>
                </label>
              )}
              <Field label='Bearer token' help={target?.has_bearer_token ? 'Leave blank to keep the stored token, or enter a new value to replace it.' : 'Optional. Use this instead of basic auth when the endpoint expects a bearer token.'}>
                <textarea className={`${styles.fieldInput} ${styles.fieldTextarea}`} value={draft.bearer_token} onChange={e => setDraft(curr => ({ ...curr, bearer_token: e.target.value, clear_bearer_token: false }))} placeholder={target?.has_bearer_token ? 'Stored separately' : 'Optional'} rows={3} />
              </Field>
              {target?.has_bearer_token && (
                <label className={styles.checkboxRow}>
                  <input type='checkbox' checked={draft.clear_bearer_token} onChange={e => setDraft(curr => ({ ...curr, clear_bearer_token: e.target.checked, bearer_token: e.target.checked ? '' : curr.bearer_token }))} />
                  <span>Clear stored bearer token<small>Remove the saved bearer token from this destination.</small></span>
                </label>
              )}
              <Field label='CA certificate' help={target?.has_tls_ca_data ? 'Leave blank to keep the stored CA bundle, or enter new PEM data to replace it.' : 'Optional PEM bundle used to trust a custom certificate authority.'}>
                <textarea className={`${styles.fieldInput} ${styles.fieldTextarea}`} value={draft.tls_ca_data} onChange={e => setDraft(curr => ({ ...curr, tls_ca_data: e.target.value, clear_tls_ca_data: false }))} placeholder={target?.has_tls_ca_data ? 'Stored PEM data' : '-----BEGIN CERTIFICATE-----'} rows={5} />
              </Field>
              {target?.has_tls_ca_data && (
                <label className={styles.checkboxRow}>
                  <input type='checkbox' checked={draft.clear_tls_ca_data} onChange={e => setDraft(curr => ({ ...curr, clear_tls_ca_data: e.target.checked, tls_ca_data: e.target.checked ? '' : curr.tls_ca_data }))} />
                  <span>Clear stored CA bundle<small>Remove the saved CA certificates from this destination.</small></span>
                </label>
              )}
              <Field label='Client certificate' help={hasStoredClientCert ? 'Leave blank to keep the stored client certificate, or enter new PEM data to replace it.' : 'Optional PEM certificate for mutual TLS.'}>
                <textarea className={`${styles.fieldInput} ${styles.fieldTextarea}`} value={draft.tls_cert_data} onChange={e => setDraft(curr => ({ ...curr, tls_cert_data: e.target.value, clear_tls_cert_data: false }))} placeholder={hasStoredClientCert ? 'Stored PEM data' : '-----BEGIN CERTIFICATE-----'} rows={5} />
              </Field>
              <Field label='Client key' help={hasStoredClientCert ? 'Leave blank to keep the stored client key, or enter new PEM data to replace it.' : 'Optional PEM private key paired with the client certificate.'}>
                <textarea className={`${styles.fieldInput} ${styles.fieldTextarea}`} value={draft.tls_key_data} onChange={e => setDraft(curr => ({ ...curr, tls_key_data: e.target.value, clear_tls_key_data: false }))} placeholder={hasStoredClientCert ? 'Stored PEM key' : '-----BEGIN PRIVATE KEY-----'} rows={5} />
              </Field>
              {hasStoredClientCert && (
                <label className={styles.checkboxRow}>
                  <input type='checkbox' checked={draft.clear_tls_cert_data || draft.clear_tls_key_data} onChange={e => setDraft(curr => ({ ...curr, clear_tls_cert_data: e.target.checked, clear_tls_key_data: e.target.checked, tls_cert_data: e.target.checked ? '' : curr.tls_cert_data, tls_key_data: e.target.checked ? '' : curr.tls_key_data }))} />
                  <span>Clear stored client certificate pair<small>Remove both the saved certificate and private key from this destination.</small></span>
                </label>
              )}
            </div>
          </div>
          <div className={styles.labelsBlock}>
            <div className={styles.sectionLabel}>Labels</div>
            <LabelsEditor rows={draft.labels} onChange={rows => setDraft(curr => ({ ...curr, labels: rows }))} />
          </div>
          <div className={styles.formActions}><button className='app-button app-button--primary' type='submit' disabled={saving || !draft.name.trim() || needsEndpoint}>{saving ? 'Saving...' : 'Save runtime target'}</button></div>
        </form>
      </div>
    </div>
  )
}

function TargetSummary({ target }: { target: RuntimeTarget }) {
  const type = targetType(target.backend)
  return (
    <div className={styles.summaryBox}>
      <div className={styles.sectionLabel}>Target summary</div>
      <div className={styles.summaryGrid}>
        <RuntimeTile label='Type' value={type.label} accent />
        <RuntimeTile label='Runner' value={target.runner_backend || target.backend} />
        <RuntimeTile label='Platform' value={target.platform || 'not set'} />
        <RuntimeTile label={target.backend === 'kubernetes' ? 'Cluster API server' : 'Daemon'} value={target.endpoint_url || 'worker default'} accent />
        <RuntimeTile label='Namespace' value={target.namespace || 'not set'} />
      </div>
      <div className={styles.noteBox}>{targetSecuritySummary(target)}</div>
      {!targetSupportsWorkers(target) && <div className={styles.noteBox}>{targetSupportReason(target)}</div>}
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

function TokenReveal({
  token, draft, target, onDone,
}: {
  token: string
  draft: Draft
  target?: RuntimeTarget
  onDone: () => void
}) {
  const [copied, setCopied] = useState(false)
  const info = bootstrap(draft, target, token)
  return (
    <div className={styles.formShell}>
      <div className='panel-card'>
        <div className='panel-card__header panel-card__header--padded'><div><div className='panel-card__eyebrow'>Agent created</div><h3>Store this token now</h3></div></div>
        <div className={styles.formIntro}>
          <p>This token is only returned once. Keep it with the worker host that should register against <strong>{target?.name || 'the selected runtime target'}</strong>.</p>
          <div className={styles.tokenRow}>
            <code className={styles.tokenCode}>{token}</code>
            <button className='app-button app-button--secondary' onClick={async () => { try { await navigator.clipboard.writeText(token); setCopied(true); window.setTimeout(() => setCopied(false), 1500) } catch { setCopied(false) } }}><FaCopy />{copied ? 'Copied' : 'Copy'}</button>
          </div>
          {target && <TargetSummary target={target} />}
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
