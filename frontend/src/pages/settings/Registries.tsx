import { startTransition, useEffect, useRef, useState } from 'react'
import {
  FaArrowRotateRight,
  FaCircleCheck,
  FaCircleXmark,
  FaClock,
  FaDatabase,
  FaEllipsisVertical,
  FaPlus,
  FaTrash,
} from 'react-icons/fa6'
import {
  ApiError,
  getPlatformSettings,
  syncRegistry,
  updatePlatformSettings,
  type OCIRegistry,
  type PlatformSettings,
} from '../../lib/api'
import AppShell from '../../components/AppShell'
import SlidingPanel from '../../components/SlidingPanel'
import '../PlatformSettings.css'

const REGISTRY_PROVIDERS = ['Zot', 'Generic OCI', 'Harbor', 'JFrog Artifactory', 'AWS ECR'] as const

export default function Registries() {
  const [savedSettings, setSavedSettings] = useState<PlatformSettings | null>(null)
  const [draft, setDraft] = useState<PlatformSettings | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [syncingId, setSyncingId] = useState('')
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  const [panelRegistry, setPanelRegistry] = useState<OCIRegistry | null>(null)
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

  const updatePanelRegistry = <K extends keyof OCIRegistry>(field: K, value: OCIRegistry[K]) => {
    if (!panelRegistry) return
    const updated = { ...panelRegistry, [field]: value }
    setPanelRegistry(updated)
    patchDraft((next) => {
      next.registries = next.registries.map((r) =>
        r.registryId === panelRegistry.registryId ? updated : r,
      )
    })
  }

  const openPanel = (registry: OCIRegistry) => {
    setPanelRegistry(structuredClone(registry))
    setPanelOpen(true)
  }

  const closePanel = () => {
    setPanelOpen(false)
    setTimeout(() => setPanelRegistry(null), 300)
  }

  const addRegistry = () => {
    if (!draft) return
    const registry = emptyRegistry(draft.registries.length + 1)
    patchDraft((next) => { next.registries.push(registry) })
    setPanelRegistry(registry)
    setPanelOpen(true)
  }

  const removeRegistry = (registryId: string) => {
    if (!draft || draft.registries.length === 1) return
    patchDraft((next) => { next.registries = next.registries.filter((r) => r.registryId !== registryId) })
    closePanel()
  }

  const syncSelected = async (registryId: string) => {
    setSyncingId(registryId)
    setMessage(null)
    try {
      const updated = await syncRegistry(registryId)
      startTransition(() => {
        setDraft(updated)
        setSavedSettings(updated)
        const refreshed = updated.registries.find((r) => r.registryId === registryId)
        if (refreshed) setPanelRegistry(structuredClone(refreshed))
        setMessage({ tone: 'success', text: 'Registry sync completed and catalog index refreshed.' })
      })
    } catch (reason) {
      setMessage({ tone: 'error', text: reason instanceof ApiError ? reason.message : 'Could not sync registry.' })
    } finally {
      setSyncingId('')
    }
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
        setMessage({ tone: 'success', text: 'OCI registries saved.' })
      })
    } catch (reason) {
      setMessage({ tone: 'error', text: reason instanceof ApiError ? reason.message : 'Could not save settings.' })
    } finally {
      setSaving(false)
    }
  }

  if (loading || !draft) {
    return (
      <AppShell section='Settings' title='OCI Registries' description=''>
        <div className='platform-page platform-page--loading'>
          <div className='platform-loading-card'>
            <p className='platform-loading-card__eyebrow'>Settings</p>
            <h1>Loading OCI registries…</h1>
          </div>
        </div>
      </AppShell>
    )
  }

  const dirty = JSON.stringify(draft) !== JSON.stringify(savedSettings)

  return (
    <AppShell
      section='Settings'
      sectionTo='/settings'
      title='OCI Registries'
      description='Control where BabelSuite discovers suites and native modules.'
      actions={(
        <>
          <button className='platform-button platform-button--secondary' onClick={addRegistry}>
            <FaPlus /> <span>Add Registry</span>
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
              <div className='bs-table-cell'>Provider</div>
              <div className='bs-table-cell bs-table-cell--wide'>URL</div>
              <div className='bs-table-cell'>Status</div>
              <div className='bs-table-cell'>Last Synced</div>
              <div className='bs-table-cell bs-table-cell--shrink' />
            </div>
          </div>

          {draft.registries.length === 0 && (
            <div className='bs-table-list__empty'>
              <FaDatabase className='bs-table-list__empty-icon' />
              <h4>No OCI registries</h4>
              <p>Connect a registry to discover suites and native modules from your catalog.</p>
              <button className='platform-button' onClick={addRegistry}><FaPlus /> Add Registry</button>
            </div>
          )}

          {draft.registries.map((registry) => (
            <div className='bs-table-list__row' key={registry.registryId} onClick={() => openPanel(registry)}>
              <div className='bs-table-row'>
                <div className='bs-table-cell bs-table-cell--shrink'>
                  {registry.syncStatus?.toLowerCase().includes('indexed')
                    ? <FaCircleCheck className='bs-status-icon bs-status-icon--ok' />
                    : registry.syncStatus?.toLowerCase().includes('pending')
                      ? <FaClock className='bs-status-icon bs-status-icon--warn' />
                      : <FaCircleXmark className='bs-status-icon bs-status-icon--off' />}
                </div>
                <div className='bs-table-cell'>
                  <strong>{registry.name}</strong>
                  <p className='bs-table-cell__sub'>{registry.registryId}</p>
                </div>
                <div className='bs-table-cell'>
                  <span className='bs-tag'>{registry.provider}</span>
                </div>
                <div className='bs-table-cell bs-table-cell--wide'>
                  <span className='bs-table-cell__mono'>{registry.registryUrl || '—'}</span>
                </div>
                <div className='bs-table-cell'>
                  <span className={`bs-status-badge bs-status-badge--${syncTone(registry.syncStatus)}`}>
                    {registry.syncStatus || 'Unknown'}
                  </span>
                </div>
                <div className='bs-table-cell'>
                  {registry.lastSyncedAt
                    ? <span>{new Date(registry.lastSyncedAt).toLocaleString()}</span>
                    : <span className='bs-table-cell__muted'>Never</span>}
                </div>
                <div className='bs-table-cell bs-table-cell--shrink'>
                  <RowMenu items={[
                    { label: 'Sync Now', action: () => void syncSelected(registry.registryId) },
                    { label: 'Delete', danger: true, action: () => removeRegistry(registry.registryId) },
                  ]} />
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
            {panelRegistry && (
              <button
                className='platform-button platform-button--secondary'
                onClick={() => void syncSelected(panelRegistry.registryId)}
                disabled={syncingId === panelRegistry.registryId}
              >
                <FaArrowRotateRight />
                <span>{syncingId === panelRegistry.registryId ? 'Syncing…' : 'Sync Now'}</span>
              </button>
            )}
            <button className='platform-button platform-button--secondary' onClick={closePanel}>Cancel</button>
            <button
              className='platform-icon-button'
              style={{ marginLeft: 'auto' }}
              onClick={() => panelRegistry && removeRegistry(panelRegistry.registryId)}
              disabled={draft.registries.length <= 1}
              title='Remove registry'
            >
              <FaTrash />
            </button>
          </>
        )}
      >
        {panelRegistry && (
          <>
            <div className='white-box'>
              <p className='white-box__section-header'>Identity</p>
              <div className='bs-form-row bs-form-row--two'>
                <div>
                  <label>Registry ID</label>
                  <input value={panelRegistry.registryId} onChange={(e) => updatePanelRegistry('registryId', e.target.value)} />
                </div>
                <div>
                  <label>Display Name</label>
                  <input value={panelRegistry.name} onChange={(e) => updatePanelRegistry('name', e.target.value)} />
                </div>
              </div>
            </div>

            <div className='white-box'>
              <p className='white-box__section-header'>Connection</p>
              <div className='bs-form-row bs-form-row--two'>
                <div>
                  <label>Provider</label>
                  <select value={panelRegistry.provider} onChange={(e) => updatePanelRegistry('provider', e.target.value)}>
                    {REGISTRY_PROVIDERS.map((p) => <option key={p} value={p}>{p}</option>)}
                  </select>
                </div>
                <div>
                  <label>Region <span style={{ fontWeight: 400, textTransform: 'none', letterSpacing: 0 }}>(cloud only)</span></label>
                  <input value={panelRegistry.region} onChange={(e) => updatePanelRegistry('region', e.target.value)} placeholder='eu-west-1' />
                </div>
              </div>
              <div className='bs-form-row'>
                <label>Registry URL</label>
                <input value={panelRegistry.registryUrl} onChange={(e) => updatePanelRegistry('registryUrl', e.target.value)} placeholder='http://localhost:5000' />
              </div>
              <div className='bs-form-row bs-form-row--two'>
                <div>
                  <label>Username / Service Account</label>
                  <input value={panelRegistry.username} onChange={(e) => updatePanelRegistry('username', e.target.value)} />
                </div>
                <div>
                  <label>Access Token / Password</label>
                  <input type='password' value={panelRegistry.secret} onChange={(e) => updatePanelRegistry('secret', e.target.value)} />
                </div>
              </div>
              <div className='bs-form-row'>
                <label>Repository Scope</label>
                <input value={panelRegistry.repositoryScope} onChange={(e) => updatePanelRegistry('repositoryScope', e.target.value)} placeholder='platform/catalog' />
              </div>
            </div>

            <div className='white-box'>
              <p className='white-box__section-header'>Sync Status</p>
              <div className='bs-form-row bs-form-row--two'>
                <div>
                  <label>Status</label>
                  <input value={panelRegistry.syncStatus} onChange={(e) => updatePanelRegistry('syncStatus', e.target.value)} />
                </div>
                <div>
                  <label>Last Indexed</label>
                  <input
                    readOnly
                    value={panelRegistry.lastSyncedAt ? new Date(panelRegistry.lastSyncedAt).toLocaleString() : 'Never'}
                    style={{ color: '#678094', cursor: 'default' }}
                  />
                </div>
              </div>
            </div>
          </>
        )}
      </SlidingPanel>
    </AppShell>
  )
}

function syncTone(status: string) {
  const s = (status || '').toLowerCase()
  if (s.includes('indexed') || s.includes('ready')) return 'ok'
  if (s.includes('pending')) return 'warn'
  return 'off'
}

function RowMenu({ items }: {
  items: Array<{ label: string; danger?: boolean; action: () => void }>
}) {
  const [open, setOpen] = useState(false)
  const [confirming, setConfirming] = useState<string | null>(null)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
        setConfirming(null)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  return (
    <div className='row-menu' ref={ref} onClick={(e) => e.stopPropagation()}>
      <button
        className='row-menu__trigger'
        onClick={(e) => { e.stopPropagation(); setOpen((v) => !v); setConfirming(null) }}
      >
        <FaEllipsisVertical />
      </button>
      {open && (
        <div className='row-menu__dropdown'>
          {items.map((item) => (
            confirming === item.label
              ? (
                <div key={item.label} className='row-menu__confirm'>
                  <span>Sure?</span>
                  <button onClick={() => { item.action(); setOpen(false); setConfirming(null) }}>Yes</button>
                  <button onClick={() => setConfirming(null)}>No</button>
                </div>
              )
              : (
                <button
                  key={item.label}
                  className={`row-menu__item${item.danger ? ' row-menu__item--danger' : ''}`}
                  onClick={() => item.danger ? setConfirming(item.label) : (item.action(), setOpen(false))}
                >
                  {item.label}
                </button>
              )
          ))}
        </div>
      )}
    </div>
  )
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
