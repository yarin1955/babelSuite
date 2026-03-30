import { startTransition, useEffect, useState } from 'react'
import { FaEllipsisVertical, FaPlus, FaTrash } from 'react-icons/fa6'
import { Link } from 'react-router-dom'
import {
  ApiError,
  getPlatformSettings,
  updatePlatformSettings,
  type GlobalOverride,
  type PlatformSettings,
} from '../../lib/api'
import AppShell from '../../components/AppShell'
import SlidingPanel from '../../components/SlidingPanel'
import '../PlatformSettings.css'

const SECRET_PROVIDERS = [
  { value: 'none', label: 'No external manager' },
  { value: 'vault', label: 'HashiCorp Vault' },
  { value: 'aws-secrets-manager', label: 'AWS Secrets Manager' },
] as const

export default function Secrets() {
  const [savedSettings, setSavedSettings] = useState<PlatformSettings | null>(null)
  const [draft, setDraft] = useState<PlatformSettings | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)
  const [panelOverride, setPanelOverride] = useState<{ index: number; override: GlobalOverride } | null>(null)
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

  const save = async () => {
    if (!draft) return
    setSaving(true)
    setMessage(null)
    try {
      const updated = await updatePlatformSettings(draft)
      startTransition(() => {
        setDraft(updated)
        setSavedSettings(updated)
        setMessage({ tone: 'success', text: 'Global secrets saved.' })
      })
    } catch (reason) {
      setMessage({ tone: 'error', text: reason instanceof ApiError ? reason.message : 'Could not save settings.' })
    } finally {
      setSaving(false)
    }
  }

  const openOverridePanel = (index: number, override: GlobalOverride) => {
    setPanelOverride({ index, override: structuredClone(override) })
    setPanelOpen(true)
  }

  const closePanel = () => {
    setPanelOpen(false)
    setTimeout(() => setPanelOverride(null), 300)
  }

  const updateOverrideField = <K extends keyof GlobalOverride>(field: K, value: GlobalOverride[K]) => {
    if (!panelOverride) return
    const updated = { ...panelOverride.override, [field]: value }
    setPanelOverride({ ...panelOverride, override: updated })
    patchDraft((next) => { next.secrets.globalOverrides[panelOverride.index] = updated })
  }

  const addOverride = () => {
    if (!draft) return
    const index = draft.secrets.globalOverrides.length
    const override: GlobalOverride = { key: '', value: '', description: '', sensitive: false }
    patchDraft((next) => { next.secrets.globalOverrides.push(override) })
    setPanelOverride({ index, override })
    setPanelOpen(true)
  }

  const removeOverride = (index: number) => {
    patchDraft((next) => { next.secrets.globalOverrides.splice(index, 1) })
    closePanel()
  }

  if (loading || !draft) {
    return (
      <AppShell section='Settings' title='Global Secrets' description=''>
        <div className='platform-page platform-page--loading'>
          <div className='platform-loading-card'>
            <p className='platform-loading-card__eyebrow'>Settings</p>
            <h1>Loading secrets configuration…</h1>
          </div>
        </div>
      </AppShell>
    )
  }

  const dirty = JSON.stringify(draft) !== JSON.stringify(savedSettings)

  return (
    <AppShell
      section='Settings'
      title='Global Secrets'
      description='Configure an external secrets manager and define platform-wide overrides injected into every suite run.'
      actions={(
        <>
          <Link to='/settings' className='platform-button platform-button--secondary'>← Settings</Link>
          <button className='platform-button' onClick={save} disabled={!dirty || saving}>
            {saving ? 'Saving…' : dirty ? 'Save Changes' : 'Saved'}
          </button>
        </>
      )}
    >
      <div className='platform-page'>
        {message && <div className={`platform-alert platform-alert--${message.tone}`}>{message.text}</div>}

        {/* ── Secrets manager integration (appearance-list panel style) ── */}
        <div className='bs-settings-panel'>
          <p className='bs-settings-panel__header'>Secrets Manager Integration</p>

          <div className='bs-settings-panel__row'>
            <div className='bs-settings-panel__label'>Provider</div>
            <div className='bs-settings-panel__control'>
              <select
                className='bs-select'
                value={draft.secrets.provider}
                onChange={(e) => patchDraft((next) => { next.secrets.provider = e.target.value as PlatformSettings['secrets']['provider'] })}
              >
                {SECRET_PROVIDERS.map((p) => <option key={p.value} value={p.value}>{p.label}</option>)}
              </select>
            </div>
          </div>

          {(draft.secrets.provider === 'vault' || draft.secrets.provider === 'aws-secrets-manager') && (
            <>
              {draft.secrets.provider === 'vault' && (
                <>
                  <div className='bs-settings-panel__row'>
                    <div className='bs-settings-panel__label'>Vault Address</div>
                    <div className='bs-settings-panel__control'>
                      <input
                        className='bs-field'
                        value={draft.secrets.vaultAddress}
                        onChange={(e) => patchDraft((next) => { next.secrets.vaultAddress = e.target.value })}
                        placeholder='https://vault.internal.company.com'
                      />
                    </div>
                  </div>
                  <div className='bs-settings-panel__row'>
                    <div className='bs-settings-panel__label'>Vault Namespace</div>
                    <div className='bs-settings-panel__control'>
                      <input
                        className='bs-field'
                        value={draft.secrets.vaultNamespace}
                        onChange={(e) => patchDraft((next) => { next.secrets.vaultNamespace = e.target.value })}
                      />
                    </div>
                  </div>
                  <div className='bs-settings-panel__row'>
                    <div className='bs-settings-panel__label'>Vault Role</div>
                    <div className='bs-settings-panel__control'>
                      <input
                        className='bs-field'
                        value={draft.secrets.vaultRole}
                        onChange={(e) => patchDraft((next) => { next.secrets.vaultRole = e.target.value })}
                      />
                    </div>
                  </div>
                </>
              )}
              {draft.secrets.provider === 'aws-secrets-manager' && (
                <div className='bs-settings-panel__row'>
                  <div className='bs-settings-panel__label'>AWS Region</div>
                  <div className='bs-settings-panel__control'>
                    <input
                      className='bs-field'
                      value={draft.secrets.awsRegion}
                      onChange={(e) => patchDraft((next) => { next.secrets.awsRegion = e.target.value })}
                      placeholder='us-east-1'
                    />
                  </div>
                </div>
              )}
              <div className='bs-settings-panel__row'>
                <div className='bs-settings-panel__label'>Secret Prefix</div>
                <div className='bs-settings-panel__control'>
                  <input
                    className='bs-field'
                    value={draft.secrets.secretPrefix}
                    onChange={(e) => patchDraft((next) => { next.secrets.secretPrefix = e.target.value })}
                    placeholder='kv/platform'
                  />
                </div>
              </div>
            </>
          )}
        </div>

        {/* ── Global overrides table ── */}
        <div className='bs-settings-section-title'>
          <span>Global Overrides</span>
          <button className='platform-button platform-button--secondary' onClick={addOverride}>
            <FaPlus /> <span>Add Override</span>
          </button>
        </div>
        <p className='bs-settings-section-desc'>
          Key-value pairs injected into every suite run — proxies, root CAs, or organization-wide runtime flags.
        </p>

        <div className='bs-table-list bs-table-list--clickable'>
          <div className='bs-table-list__head'>
            <div className='bs-table-row'>
              <div className='bs-table-cell'>Key</div>
              <div className='bs-table-cell'>Description</div>
              <div className='bs-table-cell'>Sensitive</div>
              <div className='bs-table-cell bs-table-cell--shrink' />
            </div>
          </div>

          {draft.secrets.globalOverrides.length === 0 && (
            <div className='bs-table-list__empty'>
              No global overrides defined. <button className='bs-link' onClick={addOverride}>Add one.</button>
            </div>
          )}

          {draft.secrets.globalOverrides.map((override, index) => (
            <div className='bs-table-list__row' key={`${override.key}-${index}`} onClick={() => openOverridePanel(index, override)}>
              <div className='bs-table-row'>
                <div className='bs-table-cell'>
                  <strong>{override.key || <span className='bs-table-cell__muted'>Unnamed</span>}</strong>
                </div>
                <div className='bs-table-cell'>
                  {override.description || <span className='bs-table-cell__muted'>—</span>}
                </div>
                <div className='bs-table-cell'>
                  {override.sensitive
                    ? <span className='bs-status-badge bs-status-badge--warn'>sensitive</span>
                    : <span className='bs-status-badge bs-status-badge--off'>plain</span>}
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
              onClick={() => panelOverride && removeOverride(panelOverride.index)}
              title='Remove override'
            >
              <FaTrash />
            </button>
          </>
        )}
      >
        {panelOverride && (
          <div className='white-box'>
            <p className='white-box__section-header'>Global Override</p>
            <div className='bs-form-row'>
              <label>Key</label>
              <input
                value={panelOverride.override.key}
                onChange={(e) => updateOverrideField('key', e.target.value)}
                placeholder='HTTPS_PROXY'
              />
            </div>
            <div className='bs-form-row'>
              <label>Description</label>
              <input
                value={panelOverride.override.description}
                onChange={(e) => updateOverrideField('description', e.target.value)}
              />
            </div>
            <div className='bs-form-row'>
              <label>Value</label>
              <textarea
                value={panelOverride.override.value}
                onChange={(e) => updateOverrideField('value', e.target.value)}
                rows={4}
              />
            </div>
            <div className='bs-form-checks'>
              <label>
                <input
                  type='checkbox'
                  checked={panelOverride.override.sensitive}
                  onChange={(e) => updateOverrideField('sensitive', e.target.checked)}
                />
                Sensitive value — mask in logs and UI
              </label>
            </div>
          </div>
        )}
      </SlidingPanel>
    </AppShell>
  )
}
