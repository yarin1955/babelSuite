import { startTransition, useEffect, useState } from 'react'
import {
  FaAngleRight,
  FaArrowRotateRight,
  FaBoxArchive,
  FaCloudArrowUp,
  FaDocker,
  FaFloppyDisk,
  FaKey,
  FaPlus,
  FaServer,
  FaTrash,
} from 'react-icons/fa6'
import { useNavigate } from 'react-router-dom'
import {
  ApiError,
  getPlatformSettings,
  syncRegistry,
  updatePlatformSettings,
  type APISIXSidecarConfig,
  type ExecutionAgent,
  type OCIRegistry,
  type PlatformSettings,
} from '../lib/api'
import AppShell from '../components/AppShell'
import './PlatformSettings.css'

type Section = 'agents' | 'registries' | 'secrets'

const AGENT_TYPES = [
  { value: 'local', label: 'Local (Default)' },
  { value: 'remote-agent', label: 'Remote Worker' },
  { value: 'remote-docker', label: 'Remote Docker' },
  { value: 'kubernetes', label: 'Kubernetes' },
] as const

const REGISTRY_PROVIDERS = ['Zot', 'Generic OCI', 'Harbor', 'JFrog Artifactory', 'AWS ECR'] as const
const SECRET_PROVIDERS = [
  { value: 'none', label: 'No external manager' },
  { value: 'vault', label: 'HashiCorp Vault' },
  { value: 'aws-secrets-manager', label: 'AWS Secrets Manager' },
] as const

export default function PlatformSettings() {
  const navigate = useNavigate()
  const [savedSettings, setSavedSettings] = useState<PlatformSettings | null>(null)
  const [draft, setDraft] = useState<PlatformSettings | null>(null)
  const [activeSection, setActiveSection] = useState<Section | null>(null)
  const [selectedAgentId, setSelectedAgentId] = useState('')
  const [selectedRegistryId, setSelectedRegistryId] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [syncingRegistryId, setSyncingRegistryId] = useState('')
  const [message, setMessage] = useState<{ tone: 'success' | 'error' | 'info'; text: string } | null>(null)

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
          setSelectedAgentId(settings.agents[0]?.agentId ?? '')
          setSelectedRegistryId(settings.registries[0]?.registryId ?? '')
          setMessage(null)
        })
      } catch (reason) {
        if (cancelled) return
        setMessage({
          tone: 'error',
          text: reason instanceof ApiError ? reason.message : 'Could not load platform settings.',
        })
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

  const save = async () => {
    if (!draft) return

    setSaving(true)
    setMessage(null)
    try {
      const updated = await updatePlatformSettings(draft)
      startTransition(() => {
        setDraft(updated)
        setSavedSettings(updated)
        setSelectedAgentId(updated.agents[0]?.agentId ?? '')
        setSelectedRegistryId(updated.registries[0]?.registryId ?? '')
        setMessage({ tone: 'success', text: 'Platform settings saved to configuration.yaml.' })
      })
    } catch (reason) {
      setMessage({
        tone: 'error',
        text: reason instanceof ApiError ? reason.message : 'Could not save platform settings.',
      })
    } finally {
      setSaving(false)
    }
  }

  const syncSelectedRegistry = async (registryId: string) => {
    setSyncingRegistryId(registryId)
    setMessage(null)
    try {
      const updated = await syncRegistry(registryId)
      startTransition(() => {
        setDraft(updated)
        setSavedSettings(updated)
        setSelectedRegistryId(registryId)
        setMessage({ tone: 'success', text: 'Registry sync completed and catalog index refreshed.' })
      })
    } catch (reason) {
      setMessage({
        tone: 'error',
        text: reason instanceof ApiError ? reason.message : 'Could not sync registry.',
      })
    } finally {
      setSyncingRegistryId('')
    }
  }

  if (loading || !draft) {
    return (
      <AppShell
        section='Settings'
        title='Platform Settings'
        description='Configure execution agents, OCI registries, and global secrets.'
      >
        <div className='platform-page platform-page--loading'>
          <div className='platform-loading-card'>
            <p className='platform-loading-card__eyebrow'>Settings</p>
            <h2>Loading platform settings</h2>
            <p>Reading execution agents, OCI registries, and global secret controls.</p>
          </div>
        </div>
      </AppShell>
    )
  }

  const dirty = JSON.stringify(draft) !== JSON.stringify(savedSettings)
  const selectedAgent = draft.agents.find((a) => a.agentId === selectedAgentId) ?? draft.agents[0]
  const selectedRegistry = draft.registries.find((r) => r.registryId === selectedRegistryId) ?? draft.registries[0]

  const updateAgent = <K extends keyof ExecutionAgent>(field: K, value: ExecutionAgent[K]) => {
    if (!selectedAgent) return
    patchDraft((next) => {
      next.agents = next.agents.map((agent) => {
        if (agent.agentId !== selectedAgent.agentId) {
          if (field === 'default' && value === true) return { ...agent, default: false }
          return agent
        }
        return { ...agent, [field]: value }
      })
    })
  }

  const updateAgentSidecar = <K extends keyof APISIXSidecarConfig>(field: K, value: APISIXSidecarConfig[K]) => {
    if (!selectedAgent) return
    patchDraft((next) => {
      next.agents = next.agents.map((agent) =>
        agent.agentId === selectedAgent.agentId
          ? {
              ...agent,
              apisixSidecar: {
                ...agent.apisixSidecar,
                [field]: value,
              },
            }
          : agent,
      )
    })
  }

  const updateRegistry = <K extends keyof OCIRegistry>(field: K, value: OCIRegistry[K]) => {
    if (!selectedRegistry) return
    patchDraft((next) => {
      next.registries = next.registries.map((r) =>
        r.registryId === selectedRegistry.registryId ? { ...r, [field]: value } : r,
      )
    })
  }

  const addAgent = () => {
    const agent = emptyAgent(draft.agents.length + 1)
    patchDraft((next) => { next.agents.push(agent) })
    setSelectedAgentId(agent.agentId)
  }

  const removeSelectedAgent = () => {
    if (!selectedAgent || draft.agents.length === 1) return
    patchDraft((next) => { next.agents = next.agents.filter((a) => a.agentId !== selectedAgent.agentId) })
    const fallback = draft.agents.find((a) => a.agentId !== selectedAgent.agentId)
    setSelectedAgentId(fallback?.agentId ?? '')
  }

  const addRegistry = () => {
    const registry = emptyRegistry(draft.registries.length + 1)
    patchDraft((next) => { next.registries.push(registry) })
    setSelectedRegistryId(registry.registryId)
  }

  const removeSelectedRegistry = () => {
    if (!selectedRegistry || draft.registries.length === 1) return
    patchDraft((next) => { next.registries = next.registries.filter((r) => r.registryId !== selectedRegistry.registryId) })
    const fallback = draft.registries.find((r) => r.registryId !== selectedRegistry.registryId)
    setSelectedRegistryId(fallback?.registryId ?? '')
  }

  const addOverride = () => {
    patchDraft((next) => {
      next.secrets.globalOverrides.push({ key: '', value: '', description: '', sensitive: false })
    })
  }

  const removeOverride = (index: number) => {
    patchDraft((next) => { next.secrets.globalOverrides.splice(index, 1) })
  }

  // ── Overview ──────────────────────────────────────────────────────────────
  if (!activeSection) {
    return (
      <AppShell
        section='Settings'
        title='Platform Settings'
        description='Configure execution agents, OCI registries, and global secrets.'
      >
        <div className='platform-page'>
          {message && (
            <div className={`platform-alert platform-alert--${message.tone}`}>{message.text}</div>
          )}
          <div className='platform-settings-overview'>
            <div
              className='platform-redirect-panel'
              onClick={() => setActiveSection('agents')}
            >
              <div className='platform-redirect-panel__icon'><FaServer /></div>
              <div className='platform-redirect-panel__content'>
                <div className='platform-redirect-panel__title'>Execution Agents</div>
                <div className='platform-redirect-panel__description'>
                  Configure the runner pools where suites execute and where the APISIX sidecar is injected for API-facing suites. {draft.agents.length} agent{draft.agents.length !== 1 ? 's' : ''} configured.
                </div>
              </div>
              <div className='platform-redirect-panel__arrow'><FaAngleRight /></div>
            </div>

            <div
              className='platform-redirect-panel'
              onClick={() => setActiveSection('registries')}
            >
              <div className='platform-redirect-panel__icon'><FaBoxArchive /></div>
              <div className='platform-redirect-panel__content'>
                <div className='platform-redirect-panel__title'>OCI Registries</div>
                <div className='platform-redirect-panel__description'>
                  Control where BabelSuite discovers suites and native modules. Trigger syncs to refresh the catalog index. {draft.registries.length} registr{draft.registries.length !== 1 ? 'ies' : 'y'} configured.
                </div>
              </div>
              <div className='platform-redirect-panel__arrow'><FaAngleRight /></div>
            </div>

            <div
              className='platform-redirect-panel'
              onClick={() => setActiveSection('secrets')}
            >
              <div className='platform-redirect-panel__icon'><FaKey /></div>
              <div className='platform-redirect-panel__content'>
                <div className='platform-redirect-panel__title'>Global Secrets</div>
                <div className='platform-redirect-panel__description'>
                  Configure Vault or AWS Secrets Manager integration and define platform-wide overrides injected into every suite run. {draft.secrets.globalOverrides.length} override{draft.secrets.globalOverrides.length !== 1 ? 's' : ''} defined.
                </div>
              </div>
              <div className='platform-redirect-panel__arrow'><FaAngleRight /></div>
            </div>
          </div>
        </div>
      </AppShell>
    )
  }

  // ── Section shell (breadcrumb + save button shared across all sections) ───
  const sectionTitles: Record<Section, string> = {
    agents: 'Execution Agents',
    registries: 'OCI Registries',
    secrets: 'Global Secrets',
  }

  return (
    <AppShell
      section='Settings'
      title='Platform Settings'
      description='Configure execution agents, OCI registries, and global secrets.'
      actions={(
        <>
          <span className='platform-badge'>Local mode</span>
          <button className='platform-button platform-button--secondary' onClick={() => navigate('/')}>
            Dashboard
          </button>
          <button className='platform-button' onClick={save} disabled={!dirty || saving}>
            <FaFloppyDisk />
            <span>{saving ? 'Saving…' : dirty ? 'Save Changes' : 'Saved'}</span>
          </button>
        </>
      )}
    >
      <div className='platform-page'>
        {message && (
          <div className={`platform-alert platform-alert--${message.tone}`}>{message.text}</div>
        )}

        <nav className='platform-breadcrumb'>
          <button className='platform-breadcrumb__link' onClick={() => setActiveSection(null)}>
            Settings
          </button>
          <span className='platform-breadcrumb__sep'>/</span>
          <span className='platform-breadcrumb__current'>{sectionTitles[activeSection]}</span>
        </nav>

        {/* ── Agents ─────────────────────────────────────────────────────── */}
        {activeSection === 'agents' && (
          <section className='platform-section'>
            <div className='platform-section__header'>
              <div>
                <h2>Execution Agents</h2>
                <p>Configure the physical environments where suites execute, then tag them so heavier workloads and APISIX-fronted APIs route to the right pool.</p>
              </div>
              <button className='platform-button platform-button--secondary' onClick={addAgent}>
                <FaPlus />
                <span>Add Agent</span>
              </button>
            </div>

            <div className='platform-two-column'>
              <div className='platform-panel'>
                <div className='platform-list'>
                  {draft.agents.map((agent) => (
                    <button
                      key={agent.agentId}
                      className={`platform-list__row${agent.agentId === selectedAgent?.agentId ? ' platform-list__row--active' : ''}`}
                      onClick={() => setSelectedAgentId(agent.agentId)}
                    >
                      <div className='platform-list__main'>
                        <strong>{agent.name}</strong>
                        <p>{labelForAgentType(agent.type)}{agent.default ? ' · Default' : ''}</p>
                        {agent.lastHeartbeatAt && (
                          <p className='platform-list__sub'>Last heartbeat {new Date(agent.lastHeartbeatAt).toLocaleString()}</p>
                        )}
                      </div>
                      <div className='platform-list__meta'>
                        <span className={`platform-status platform-status--${statusTone(agent.status)}`}>{agent.status}</span>
                        <span>{agent.routingTags.join(', ') || 'No tags'}</span>
                      </div>
                    </button>
                  ))}
                </div>
              </div>

              {selectedAgent && (
                <div className='platform-panel platform-editor'>
                  <div className='platform-editor__top'>
                    <div>
                      <h3>{selectedAgent.name}</h3>
                      <p>{selectedAgent.description || 'Describe how this agent should be used by suite authors and operators.'}</p>
                    </div>
                    <button
                      className='platform-icon-button'
                      onClick={removeSelectedAgent}
                      disabled={draft.agents.length === 1}
                      title={draft.agents.length === 1 ? 'Keep at least one agent.' : 'Remove agent'}
                    >
                      <FaTrash />
                    </button>
                  </div>

                  <div className='platform-form-grid'>
                    <label className='platform-field'>
                      <span>Agent ID</span>
                      <input value={selectedAgent.agentId} onChange={(e) => updateAgent('agentId', e.target.value)} />
                    </label>
                    <label className='platform-field'>
                      <span>Display Name</span>
                      <input value={selectedAgent.name} onChange={(e) => updateAgent('name', e.target.value)} />
                    </label>
                    <label className='platform-field platform-field--full'>
                      <span>Description</span>
                      <textarea value={selectedAgent.description} onChange={(e) => updateAgent('description', e.target.value)} />
                    </label>
                    <label className='platform-field'>
                      <span>Agent Type</span>
                      <select value={selectedAgent.type} onChange={(e) => updateAgent('type', e.target.value as ExecutionAgent['type'])}>
                        {AGENT_TYPES.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
                      </select>
                    </label>
                    <label className='platform-field'>
                      <span>Status</span>
                      <input value={selectedAgent.status} onChange={(e) => updateAgent('status', e.target.value)} />
                    </label>
                    <label className='platform-field'>
                      <span>Last heartbeat</span>
                      <input value={selectedAgent.lastHeartbeatAt ? new Date(selectedAgent.lastHeartbeatAt).toLocaleString() : 'Never'} readOnly />
                    </label>
                    <label className='platform-field platform-field--full'>
                      <span>Routing Tags</span>
                      <input
                        value={selectedAgent.routingTags.join(', ')}
                        onChange={(e) => updateAgent('routingTags', splitList(e.target.value))}
                        placeholder='gpu-enabled, high-memory, ci-only'
                      />
                    </label>
                    <label className='platform-field platform-field--full'>
                      <span>Runtime Capabilities</span>
                      <input
                        value={selectedAgent.runtimeCapabilities.join(', ')}
                        onChange={(e) => updateAgent('runtimeCapabilities', splitList(e.target.value))}
                        placeholder='service, mock, task, test'
                      />
                    </label>
                  </div>

                  <div className='platform-checks'>
                    <label><input type='checkbox' checked={selectedAgent.enabled} onChange={(e) => updateAgent('enabled', e.target.checked)} /> Enabled</label>
                    <label><input type='checkbox' checked={selectedAgent.default} onChange={(e) => updateAgent('default', e.target.checked)} /> Default route</label>
                  </div>

                  {selectedAgent.type === 'local' && (
                    <div className='platform-detail-card'>
                      <div className='platform-detail-card__title'><FaDocker /> Local Docker</div>
                      <label className='platform-field platform-field--full'>
                        <span>Docker Socket</span>
                        <input value={selectedAgent.dockerSocket} onChange={(e) => updateAgent('dockerSocket', e.target.value)} />
                      </label>
                    </div>
                  )}

                  {selectedAgent.type === 'remote-agent' && (
                    <div className='platform-detail-card'>
                      <div className='platform-detail-card__title'><FaCloudArrowUp /> Remote Worker</div>
                      <div className='platform-form-grid'>
                        <label className='platform-field platform-field--full'>
                          <span>Worker URL</span>
                          <input value={selectedAgent.hostUrl} onChange={(e) => updateAgent('hostUrl', e.target.value)} />
                        </label>
                        <label className='platform-field'>
                          <span>TLS Cert</span>
                          <input value={selectedAgent.tlsCert} onChange={(e) => updateAgent('tlsCert', e.target.value)} />
                        </label>
                        <label className='platform-field'>
                          <span>TLS Key</span>
                          <input value={selectedAgent.tlsKey} onChange={(e) => updateAgent('tlsKey', e.target.value)} />
                        </label>
                      </div>
                    </div>
                  )}

                  {selectedAgent.type === 'remote-docker' && (
                    <div className='platform-detail-card'>
                      <div className='platform-detail-card__title'><FaCloudArrowUp /> Remote Docker</div>
                      <div className='platform-form-grid'>
                        <label className='platform-field platform-field--full'>
                          <span>TCP Host URL</span>
                          <input value={selectedAgent.hostUrl} onChange={(e) => updateAgent('hostUrl', e.target.value)} />
                        </label>
                        <label className='platform-field'>
                          <span>TLS Cert</span>
                          <input value={selectedAgent.tlsCert} onChange={(e) => updateAgent('tlsCert', e.target.value)} />
                        </label>
                        <label className='platform-field'>
                          <span>TLS Key</span>
                          <input value={selectedAgent.tlsKey} onChange={(e) => updateAgent('tlsKey', e.target.value)} />
                        </label>
                      </div>
                    </div>
                  )}

                  {selectedAgent.type === 'kubernetes' && (
                    <div className='platform-detail-card'>
                      <div className='platform-detail-card__title'><FaCloudArrowUp /> Kubernetes</div>
                      <div className='platform-form-grid'>
                        <label className='platform-field platform-field--full'>
                          <span>Kubeconfig File Path</span>
                          <input value={selectedAgent.kubeconfigPath} onChange={(e) => updateAgent('kubeconfigPath', e.target.value)} />
                        </label>
                        <label className='platform-field'>
                          <span>Target Namespace</span>
                          <input value={selectedAgent.targetNamespace} onChange={(e) => updateAgent('targetNamespace', e.target.value)} />
                        </label>
                        <label className='platform-field'>
                          <span>Service Account Token</span>
                          <input value={selectedAgent.serviceAccountToken} onChange={(e) => updateAgent('serviceAccountToken', e.target.value)} />
                        </label>
                      </div>
                    </div>
                  )}

                  <div className='platform-detail-card'>
                    <div className='platform-detail-card__title'>APISIX Sidecar</div>
                    <div className='platform-form-grid'>
                      <label className='platform-field platform-field--full'>
                        <span>Runtime Image</span>
                        <input value={selectedAgent.apisixSidecar.image} onChange={(e) => updateAgentSidecar('image', e.target.value)} />
                      </label>
                      <label className='platform-field platform-field--full'>
                        <span>Config Mount Path</span>
                        <input value={selectedAgent.apisixSidecar.configMountPath} onChange={(e) => updateAgentSidecar('configMountPath', e.target.value)} />
                      </label>
                      <label className='platform-field'>
                        <span>Listen Port</span>
                        <input
                          type='number'
                          value={selectedAgent.apisixSidecar.listenPort}
                          onChange={(e) => updateAgentSidecar('listenPort', parseNumberInput(e.target.value))}
                        />
                      </label>
                      <label className='platform-field'>
                        <span>Admin Port</span>
                        <input
                          type='number'
                          value={selectedAgent.apisixSidecar.adminPort}
                          onChange={(e) => updateAgentSidecar('adminPort', parseNumberInput(e.target.value))}
                        />
                      </label>
                      <label className='platform-field platform-field--full'>
                        <span>Capabilities</span>
                        <input
                          value={selectedAgent.apisixSidecar.capabilities.join(', ')}
                          onChange={(e) => updateAgentSidecar('capabilities', splitList(e.target.value))}
                          placeholder='rest, graphql, grpc, soap, kafka'
                        />
                      </label>
                    </div>
                  </div>
                </div>
              )}
            </div>
          </section>
        )}

        {/* ── Registries ─────────────────────────────────────────────────── */}
        {activeSection === 'registries' && (
          <section className='platform-section'>
            <div className='platform-section__header'>
              <div>
                <h2>OCI Registries</h2>
                <p>Control where BabelSuite discovers suites and native modules, then trigger syncs to refresh the catalog page.</p>
              </div>
              <div className='platform-section__actions'>
                {selectedRegistry && (
                  <button
                    className='platform-button platform-button--secondary'
                    onClick={() => void syncSelectedRegistry(selectedRegistry.registryId)}
                    disabled={syncingRegistryId === selectedRegistry.registryId}
                  >
                    <FaArrowRotateRight />
                    <span>{syncingRegistryId === selectedRegistry.registryId ? 'Syncing…' : 'Sync Selected'}</span>
                  </button>
                )}
                <button className='platform-button platform-button--secondary' onClick={addRegistry}>
                  <FaPlus />
                  <span>Add Registry</span>
                </button>
              </div>
            </div>

            <div className='platform-two-column'>
              <div className='platform-panel'>
                <div className='platform-list'>
                  {draft.registries.map((registry) => (
                    <button
                      key={registry.registryId}
                      className={`platform-list__row${registry.registryId === selectedRegistry?.registryId ? ' platform-list__row--active' : ''}`}
                      onClick={() => setSelectedRegistryId(registry.registryId)}
                    >
                      <div className='platform-list__main'>
                        <strong>{registry.name}</strong>
                        <p>{registry.provider}</p>
                      </div>
                      <div className='platform-list__meta'>
                        <span className={`platform-status platform-status--${statusTone(registry.syncStatus)}`}>{registry.syncStatus || 'Unknown'}</span>
                        <span>{registry.registryUrl}</span>
                      </div>
                    </button>
                  ))}
                </div>
              </div>

              {selectedRegistry && (
                <div className='platform-panel platform-editor'>
                  <div className='platform-editor__top'>
                    <div>
                      <h3>{selectedRegistry.name}</h3>
                      <p>Configure upstream registry connectivity, credentials, and manual index refresh for the catalog page.</p>
                    </div>
                    <button
                      className='platform-icon-button'
                      onClick={removeSelectedRegistry}
                      disabled={draft.registries.length === 1}
                      title={draft.registries.length === 1 ? 'Keep at least one registry.' : 'Remove registry'}
                    >
                      <FaTrash />
                    </button>
                  </div>

                  <div className='platform-form-grid'>
                    <label className='platform-field'>
                      <span>Registry ID</span>
                      <input value={selectedRegistry.registryId} onChange={(e) => updateRegistry('registryId', e.target.value)} />
                    </label>
                    <label className='platform-field'>
                      <span>Display Name</span>
                      <input value={selectedRegistry.name} onChange={(e) => updateRegistry('name', e.target.value)} />
                    </label>
                    <label className='platform-field'>
                      <span>Provider</span>
                      <select value={selectedRegistry.provider} onChange={(e) => updateRegistry('provider', e.target.value)}>
                        {REGISTRY_PROVIDERS.map((p) => <option key={p} value={p}>{p}</option>)}
                      </select>
                    </label>
                    <label className='platform-field'>
                      <span>Region</span>
                      <input value={selectedRegistry.region} onChange={(e) => updateRegistry('region', e.target.value)} placeholder='Only required for cloud registries' />
                    </label>
                    <label className='platform-field platform-field--full'>
                      <span>Registry URL</span>
                      <input value={selectedRegistry.registryUrl} onChange={(e) => updateRegistry('registryUrl', e.target.value)} placeholder='http://localhost:5000' />
                    </label>
                    <label className='platform-field'>
                      <span>Username / Service Account</span>
                      <input value={selectedRegistry.username} onChange={(e) => updateRegistry('username', e.target.value)} />
                    </label>
                    <label className='platform-field'>
                      <span>Access Token / Password</span>
                      <input type='password' value={selectedRegistry.secret} onChange={(e) => updateRegistry('secret', e.target.value)} />
                    </label>
                    <label className='platform-field platform-field--full'>
                      <span>Repository Scope</span>
                      <input value={selectedRegistry.repositoryScope} onChange={(e) => updateRegistry('repositoryScope', e.target.value)} placeholder='platform/catalog or modules' />
                    </label>
                  </div>

                  <div className='platform-sync-note'>
                    <strong>Last index refresh:</strong> {selectedRegistry.lastSyncedAt ? new Date(selectedRegistry.lastSyncedAt).toLocaleString() : 'Never synced'}
                  </div>
                </div>
              )}
            </div>
          </section>
        )}

        {/* ── Secrets ────────────────────────────────────────────────────── */}
        {activeSection === 'secrets' && (
          <section className='platform-section'>
            <div className='platform-section__header'>
              <div>
                <h2>Global Secrets</h2>
                <p>Configure Vault or AWS Secrets Manager, then define platform-wide overrides injected into every suite run.</p>
              </div>
            </div>

            <div className='platform-secrets-grid'>
              <div className='platform-panel platform-editor'>
                <h3>Secrets manager integration</h3>
                <div className='platform-form-grid'>
                  <label className='platform-field'>
                    <span>Provider</span>
                    <select
                      value={draft.secrets.provider}
                      onChange={(e) => patchDraft((next) => { next.secrets.provider = e.target.value as PlatformSettings['secrets']['provider'] })}
                    >
                      {SECRET_PROVIDERS.map((p) => <option key={p.value} value={p.value}>{p.label}</option>)}
                    </select>
                  </label>
                  <label className='platform-field'>
                    <span>AWS Region</span>
                    <input value={draft.secrets.awsRegion} onChange={(e) => patchDraft((next) => { next.secrets.awsRegion = e.target.value })} placeholder='us-east-1' />
                  </label>
                  <label className='platform-field'>
                    <span>Vault Address</span>
                    <input value={draft.secrets.vaultAddress} onChange={(e) => patchDraft((next) => { next.secrets.vaultAddress = e.target.value })} placeholder='https://vault.internal.company.com' />
                  </label>
                  <label className='platform-field'>
                    <span>Vault Namespace</span>
                    <input value={draft.secrets.vaultNamespace} onChange={(e) => patchDraft((next) => { next.secrets.vaultNamespace = e.target.value })} />
                  </label>
                  <label className='platform-field'>
                    <span>Vault Role</span>
                    <input value={draft.secrets.vaultRole} onChange={(e) => patchDraft((next) => { next.secrets.vaultRole = e.target.value })} />
                  </label>
                  <label className='platform-field'>
                    <span>Secret Prefix</span>
                    <input value={draft.secrets.secretPrefix} onChange={(e) => patchDraft((next) => { next.secrets.secretPrefix = e.target.value })} />
                  </label>
                </div>
              </div>

              <div className='platform-panel platform-editor'>
                <div className='platform-editor__top'>
                  <div>
                    <h3>Global overrides</h3>
                    <p>Key-value pairs injected into every suite run — proxies, root CAs, organization-wide runtime flags.</p>
                  </div>
                  <button className='platform-button platform-button--secondary' onClick={addOverride}>
                    <FaPlus />
                    <span>Add Override</span>
                  </button>
                </div>

                <div className='platform-overrides'>
                  {draft.secrets.globalOverrides.map((override, index) => (
                    <div key={`${override.key}-${index}`} className='platform-override'>
                      <div className='platform-form-grid'>
                        <label className='platform-field'>
                          <span>Key</span>
                          <input value={override.key} onChange={(e) => patchDraft((next) => { next.secrets.globalOverrides[index].key = e.target.value })} placeholder='HTTPS_PROXY' />
                        </label>
                        <label className='platform-field'>
                          <span>Description</span>
                          <input value={override.description} onChange={(e) => patchDraft((next) => { next.secrets.globalOverrides[index].description = e.target.value })} />
                        </label>
                        <label className='platform-field platform-field--full'>
                          <span>Value</span>
                          <textarea value={override.value} onChange={(e) => patchDraft((next) => { next.secrets.globalOverrides[index].value = e.target.value })} />
                        </label>
                      </div>
                      <div className='platform-checks'>
                        <label>
                          <input type='checkbox' checked={override.sensitive} onChange={(e) => patchDraft((next) => { next.secrets.globalOverrides[index].sensitive = e.target.checked })} />
                          Sensitive value
                        </label>
                        <button className='platform-icon-button' onClick={() => removeOverride(index)} title='Remove override'>
                          <FaTrash />
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </section>
        )}
      </div>
    </AppShell>
  )
}

function labelForAgentType(type: ExecutionAgent['type']) {
  switch (type) {
    case 'local': return 'Local Docker'
    case 'remote-agent': return 'Remote Worker'
    case 'remote-docker': return 'Remote Docker'
    case 'kubernetes': return 'Kubernetes'
    default: return type
  }
}

function statusTone(status: string) {
  const s = status.toLowerCase()
  if (s.includes('ready') || s.includes('indexed') || s.includes('online')) return 'success'
  if (s.includes('pending') || s.includes('standby')) return 'warning'
  if (s.includes('offline') || s.includes('disconnect')) return 'neutral'
  return 'neutral'
}

function splitList(value: string) {
  return value.split(',').map((s) => s.trim()).filter(Boolean)
}

function parseNumberInput(value: string) {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : 0
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
    runtimeCapabilities: [],
    dockerSocket: '/var/run/docker.sock',
    hostUrl: '',
    tlsCert: '',
    tlsKey: '',
    kubeconfigPath: '',
    targetNamespace: '',
    serviceAccountToken: '',
    apisixSidecar: defaultAPISIXSidecar(),
  }
}

function defaultAPISIXSidecar(): APISIXSidecarConfig {
  return {
    image: 'apache/apisix:latest',
    configMountPath: '/usr/local/apisix/conf/apisix.yaml',
    listenPort: 9080,
    adminPort: 9180,
    capabilities: ['rest', 'graphql', 'grpc', 'soap', 'kafka'],
  }
}

function emptyRegistry(index: number): OCIRegistry {
  return {
    registryId: `registry-${index}`,
    name: `Registry ${index}`,
    provider: 'Generic OCI',
    registryUrl: '',
    username: '',
    secret: '',
    repositoryScope: '',
    region: '',
    syncStatus: 'Pending',
  }
}
