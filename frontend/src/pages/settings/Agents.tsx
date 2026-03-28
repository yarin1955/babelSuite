import { startTransition, useEffect, useState } from 'react'
import {
  FaCircleCheck,
  FaCircleXmark,
  FaCloudArrowUp,
  FaDocker,
  FaEllipsisVertical,
  FaPlus,
  FaTrash,
} from 'react-icons/fa6'
import { Link } from 'react-router-dom'
import {
  ApiError,
  getPlatformSettings,
  updatePlatformSettings,
  type ExecutionAgent,
  type PlatformSettings,
} from '../../lib/api'
import AppShell from '../../components/AppShell'
import SlidingPanel from '../../components/SlidingPanel'
import '../PlatformSettings.css'

const AGENT_TYPES = [
  { value: 'local', label: 'Local (Default)' },
  { value: 'remote-docker', label: 'Remote Docker' },
  { value: 'kubernetes', label: 'Kubernetes' },
] as const

export default function Agents() {
  const [savedSettings, setSavedSettings] = useState<PlatformSettings | null>(null)
  const [draft, setDraft] = useState<PlatformSettings | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  const [panelAgent, setPanelAgent] = useState<ExecutionAgent | null>(null)
  const [panelOpen, setPanelOpen] = useState(false)

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      setLoading(true)
      try {
        const settings = await getPlatformSettings()
        if (cancelled) return
        startTransition(() => {
          setSavedSettings(settings)
          setDraft(settings)
        })
      } catch (reason) {
        if (cancelled) return
        setMessage({ tone: 'error', text: reason instanceof ApiError ? reason.message : 'Could not load settings.' })
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => { cancelled = true }
  }, [])

  const patchDraft = (mutator: (next: PlatformSettings) => void) => {
    setDraft((current) => {
      if (!current) return current
      const next = structuredClone(current)
      mutator(next)
      return next
    })
  }

  const updatePanelAgent = <K extends keyof ExecutionAgent>(field: K, value: ExecutionAgent[K]) => {
    if (!panelAgent) return
    const updated = { ...panelAgent, [field]: value }
    // If setting default = true, unset others
    setPanelAgent(updated)
    patchDraft((next) => {
      next.agents = next.agents.map((a) => {
        if (a.agentId === panelAgent.agentId) return updated
        if (field === 'default' && value === true) return { ...a, default: false }
        return a
      })
    })
  }

  const openPanel = (agent: ExecutionAgent) => {
    setPanelAgent(structuredClone(agent))
    setPanelOpen(true)
  }

  const closePanel = () => {
    setPanelOpen(false)
    setTimeout(() => setPanelAgent(null), 300)
  }

  const addAgent = () => {
    if (!draft) return
    const agent = emptyAgent(draft.agents.length + 1)
    patchDraft((next) => { next.agents.push(agent) })
    setPanelAgent(agent)
    setPanelOpen(true)
  }

  const removeAgent = (agentId: string) => {
    if (!draft || draft.agents.length === 1) return
    patchDraft((next) => { next.agents = next.agents.filter((a) => a.agentId !== agentId) })
    closePanel()
  }

  const save = async () => {
    if (!draft) return
    setSaving(true)
    setMessage(null)
    try {
      const updated = await updatePlatformSettings(draft)
      startTransition(() => {
        setDraft(updated)
        setSavedSettings(updated)
        setMessage({ tone: 'success', text: 'Execution agents saved.' })
      })
    } catch (reason) {
      setMessage({ tone: 'error', text: reason instanceof ApiError ? reason.message : 'Could not save settings.' })
    } finally {
      setSaving(false)
    }
  }

  if (loading || !draft) {
    return (
      <AppShell section='Settings' title='Execution Agents' description=''>
        <div className='platform-page platform-page--loading'>
          <div className='platform-loading-card'>
            <p className='platform-loading-card__eyebrow'>Settings</p>
            <h1>Loading execution agents…</h1>
          </div>
        </div>
      </AppShell>
    )
  }

  const dirty = JSON.stringify(draft) !== JSON.stringify(savedSettings)

  return (
    <AppShell
      section='Settings'
      title='Execution Agents'
      description='Configure the physical environments where suites execute.'
      actions={(
        <>
          <Link to='/settings' className='platform-button platform-button--secondary'>← Settings</Link>
          <button className='platform-button platform-button--secondary' onClick={addAgent}>
            <FaPlus /> <span>Add Agent</span>
          </button>
          <button className='platform-button' onClick={save} disabled={!dirty || saving}>
            {saving ? 'Saving…' : dirty ? 'Save Changes' : 'Saved'}
          </button>
        </>
      )}
    >
      <div className='platform-page'>
        {message && <div className={`platform-alert platform-alert--${message.tone}`}>{message.text}</div>}

        <div className='bs-table-list bs-table-list--clickable'>
          <div className='bs-table-list__head'>
            <div className='bs-table-row'>
              <div className='bs-table-cell bs-table-cell--shrink' />
              <div className='bs-table-cell'>Name</div>
              <div className='bs-table-cell'>Type</div>
              <div className='bs-table-cell'>Status</div>
              <div className='bs-table-cell'>Tags</div>
              <div className='bs-table-cell bs-table-cell--shrink' />
            </div>
          </div>

          {draft.agents.length === 0 && (
            <div className='bs-table-list__empty'>
              No execution agents configured. <button className='bs-link' onClick={addAgent}>Add one.</button>
            </div>
          )}

          {draft.agents.map((agent) => (
            <div className='bs-table-list__row' key={agent.agentId} onClick={() => openPanel(agent)}>
              <div className='bs-table-row'>
                <div className='bs-table-cell bs-table-cell--shrink'>
                  {agent.enabled
                    ? <FaCircleCheck className='bs-status-icon bs-status-icon--ok' />
                    : <FaCircleXmark className='bs-status-icon bs-status-icon--off' />}
                </div>
                <div className='bs-table-cell'>
                  <strong>{agent.name}</strong>
                  {agent.default && <span className='bs-tag bs-tag--default'>default</span>}
                  {agent.description && <p className='bs-table-cell__sub'>{agent.description}</p>}
                </div>
                <div className='bs-table-cell'>
                  <span className='bs-tag'>{labelForAgentType(agent.type)}</span>
                </div>
                <div className='bs-table-cell'>
                  <span className={`bs-status-badge bs-status-badge--${statusTone(agent.status)}`}>
                    {agent.status}
                  </span>
                </div>
                <div className='bs-table-cell'>
                  {agent.routingTags.length > 0
                    ? agent.routingTags.map((tag) => <span key={tag} className='bs-tag'>{tag}</span>)
                    : <span className='bs-table-cell__muted'>—</span>}
                </div>
                <div className='bs-table-cell bs-table-cell--shrink'>
                  <FaEllipsisVertical className='bs-row-action' />
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>

      <SlidingPanel
        isOpen={panelOpen}
        onClose={closePanel}
        header={(
          <>
            <button className='platform-button' onClick={save} disabled={!dirty || saving}>
              {saving ? 'Saving…' : 'Save'}
            </button>
            <button className='platform-button platform-button--secondary' onClick={closePanel}>Cancel</button>
            <button
              className='platform-icon-button'
              style={{ marginLeft: 'auto' }}
              onClick={() => panelAgent && removeAgent(panelAgent.agentId)}
              disabled={draft.agents.length <= 1}
              title='Remove agent'
            >
              <FaTrash />
            </button>
          </>
        )}
      >
        {panelAgent && (
          <>
            <div className='white-box'>
              <p className='white-box__section-header'>Identity</p>
              <div className='bs-form-row'>
                <label>Agent ID</label>
                <input value={panelAgent.agentId} onChange={(e) => updatePanelAgent('agentId', e.target.value)} />
              </div>
              <div className='bs-form-row'>
                <label>Display Name</label>
                <input value={panelAgent.name} onChange={(e) => updatePanelAgent('name', e.target.value)} />
              </div>
              <div className='bs-form-row'>
                <label>Description</label>
                <textarea value={panelAgent.description} onChange={(e) => updatePanelAgent('description', e.target.value)} />
              </div>
            </div>

            <div className='white-box'>
              <p className='white-box__section-header'>Configuration</p>
              <div className='bs-form-row bs-form-row--two'>
                <div>
                  <label>Agent Type</label>
                  <select value={panelAgent.type} onChange={(e) => updatePanelAgent('type', e.target.value as ExecutionAgent['type'])}>
                    {AGENT_TYPES.map((opt) => (
                      <option key={opt.value} value={opt.value}>{opt.label}</option>
                    ))}
                  </select>
                </div>
                <div>
                  <label>Status</label>
                  <input value={panelAgent.status} onChange={(e) => updatePanelAgent('status', e.target.value)} />
                </div>
              </div>
              <div className='bs-form-row'>
                <label>Routing Tags <span style={{ fontWeight: 400, textTransform: 'none', letterSpacing: 0 }}>(comma-separated)</span></label>
                <input
                  value={panelAgent.routingTags.join(', ')}
                  onChange={(e) => updatePanelAgent('routingTags', splitList(e.target.value))}
                  placeholder='gpu-enabled, high-memory, ci-only'
                />
              </div>
              <div className='bs-form-checks'>
                <label>
                  <input type='checkbox' checked={panelAgent.enabled} onChange={(e) => updatePanelAgent('enabled', e.target.checked)} />
                  Enabled
                </label>
                <label>
                  <input type='checkbox' checked={panelAgent.default} onChange={(e) => updatePanelAgent('default', e.target.checked)} />
                  Default route
                </label>
              </div>
            </div>

            {panelAgent.type === 'local' && (
              <div className='white-box'>
                <p className='white-box__section-header'><FaDocker style={{ marginRight: 6 }} />Local Docker</p>
                <div className='bs-form-row'>
                  <label>Docker Socket</label>
                  <input value={panelAgent.dockerSocket} onChange={(e) => updatePanelAgent('dockerSocket', e.target.value)} />
                </div>
              </div>
            )}

            {panelAgent.type === 'remote-docker' && (
              <div className='white-box'>
                <p className='white-box__section-header'><FaCloudArrowUp style={{ marginRight: 6 }} />Remote Docker</p>
                <div className='bs-form-row'>
                  <label>TCP Host URL</label>
                  <input value={panelAgent.hostUrl} onChange={(e) => updatePanelAgent('hostUrl', e.target.value)} />
                </div>
                <div className='bs-form-row bs-form-row--two'>
                  <div>
                    <label>TLS Cert</label>
                    <input value={panelAgent.tlsCert} onChange={(e) => updatePanelAgent('tlsCert', e.target.value)} />
                  </div>
                  <div>
                    <label>TLS Key</label>
                    <input value={panelAgent.tlsKey} onChange={(e) => updatePanelAgent('tlsKey', e.target.value)} />
                  </div>
                </div>
              </div>
            )}

            {panelAgent.type === 'kubernetes' && (
              <div className='white-box'>
                <p className='white-box__section-header'><FaCloudArrowUp style={{ marginRight: 6 }} />Kubernetes</p>
                <div className='bs-form-row'>
                  <label>Kubeconfig File Path</label>
                  <input value={panelAgent.kubeconfigPath} onChange={(e) => updatePanelAgent('kubeconfigPath', e.target.value)} />
                </div>
                <div className='bs-form-row bs-form-row--two'>
                  <div>
                    <label>Target Namespace</label>
                    <input value={panelAgent.targetNamespace} onChange={(e) => updatePanelAgent('targetNamespace', e.target.value)} />
                  </div>
                  <div>
                    <label>Service Account Token</label>
                    <input value={panelAgent.serviceAccountToken} onChange={(e) => updatePanelAgent('serviceAccountToken', e.target.value)} />
                  </div>
                </div>
              </div>
            )}
          </>
        )}
      </SlidingPanel>
    </AppShell>
  )
}

function labelForAgentType(type: ExecutionAgent['type']) {
  switch (type) {
    case 'local': return 'Local Docker'
    case 'remote-docker': return 'Remote Docker'
    case 'kubernetes': return 'Kubernetes'
    default: return type
  }
}

function statusTone(status: string) {
  const s = status.toLowerCase()
  if (s.includes('ready') || s.includes('indexed')) return 'ok'
  if (s.includes('pending') || s.includes('standby')) return 'warn'
  if (s.includes('disconnect') || s.includes('error')) return 'err'
  return 'off'
}

function splitList(value: string) {
  return value.split(',').map((s) => s.trim()).filter(Boolean)
}

function emptyAgent(index: number): ExecutionAgent {
  return {
    agentId: `agent-${index}`,
    name: `New Agent ${index}`,
    type: 'local',
    description: '',
    enabled: true,
    default: false,
    status: 'Ready',
    routingTags: [],
    dockerSocket: '/var/run/docker.sock',
    hostUrl: '',
    tlsCert: '',
    tlsKey: '',
    kubeconfigPath: '',
    targetNamespace: '',
    serviceAccountToken: '',
  }
}
