import { startTransition, useEffect, useState } from 'react'
import { FaFloppyDisk } from 'react-icons/fa6'
import {
  ApiError,
  getPlatformSettings,
  updatePlatformSettings,
  type PlatformSettings,
} from '../../lib/api'
import AppShell from '../../components/AppShell'
import '../PlatformSettings.css'

const MODE_OPTIONS = [
  { value: 'local', label: 'Local', desc: 'Runs workloads on the host machine via the local Docker socket.' },
  { value: 'remote', label: 'Remote', desc: 'Delegates all workloads to a remote Docker or Kubernetes pool.' },
  { value: 'hybrid', label: 'Hybrid', desc: 'Routes suites by routing tag across local and remote agents.' },
] as const

export default function General() {
  const [savedSettings, setSavedSettings] = useState<PlatformSettings | null>(null)
  const [draft, setDraft] = useState<PlatformSettings | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState<{ tone: 'success' | 'error'; text: string } | null>(null)

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

  const save = async () => {
    if (!draft) return
    setSaving(true)
    setMessage(null)
    try {
      const updated = await updatePlatformSettings(draft)
      startTransition(() => {
        setDraft(updated)
        setSavedSettings(updated)
        setMessage({ tone: 'success', text: 'General settings saved.' })
      })
    } catch (reason) {
      setMessage({ tone: 'error', text: reason instanceof ApiError ? reason.message : 'Could not save settings.' })
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <AppShell section='Settings' title='General' description=''>
        <div className='platform-page platform-page--loading'>
          <div className='platform-loading-card'>
            <p className='platform-loading-card__eyebrow'>Settings</p>
            <h1>Loading general settings…</h1>
          </div>
        </div>
      </AppShell>
    )
  }

  if (!draft) {
    return (
      <AppShell section='Settings' title='General' description=''>
        <div className='platform-page'>
          {message && <div className={`platform-alert platform-alert--${message.tone}`}>{message.text}</div>}
          <div className='platform-loading-card'>
            <p className='platform-loading-card__eyebrow'>Settings</p>
            <h1>General settings unavailable</h1>
            <p>The frontend could not load platform settings from the backend.</p>
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
      title='General'
      description='Platform-level identity and deployment mode for this BabelSuite instance.'
      actions={(
        <>
          <button className='platform-button' onClick={save} disabled={!dirty || saving}>
            <FaFloppyDisk />
            <span>{saving ? 'Saving…' : dirty ? 'Save Changes' : 'Saved'}</span>
          </button>
        </>
      )}
    >
      <div className='platform-page'>
        {message && <div className={`platform-alert platform-alert--${message.tone}`}>{message.text}</div>}

        {/* ── Deployment Mode (appearance-list panel style) ── */}
        <div className='bs-settings-panel'>
          <p className='bs-settings-panel__header'>Deployment Mode</p>

          {MODE_OPTIONS.map((opt) => (
            <div
              key={opt.value}
              className={`bs-settings-panel__row bs-settings-panel__row--selectable${draft.mode === opt.value ? ' bs-settings-panel__row--selected' : ''}`}
              onClick={() => setDraft({ ...draft, mode: opt.value })}
            >
              <div className='bs-settings-panel__label'>
                <input
                  type='radio'
                  name='mode'
                  value={opt.value}
                  checked={draft.mode === opt.value}
                  onChange={() => setDraft({ ...draft, mode: opt.value })}
                  className='bs-radio'
                />
                {opt.label}
              </div>
              <div className='bs-settings-panel__control bs-settings-panel__description'>
                {opt.desc}
              </div>
            </div>
          ))}
        </div>

        {/* ── Instance Description (appearance-list panel style) ── */}
        <div className='bs-settings-panel'>
          <p className='bs-settings-panel__header'>Instance Description</p>
          <div className='bs-settings-panel__row'>
            <div className='bs-settings-panel__label'>Description</div>
            <div className='bs-settings-panel__control'>
              <textarea
                className='bs-field'
                rows={1}
                value={draft.description}
                onChange={(e) => setDraft({ ...draft, description: e.target.value })}
                placeholder='Physical-layer configuration for BabelSuite execution agents, OCI catalog sources, and shared platform credentials.'
              />
            </div>
          </div>
        </div>

        {/* ── Instance Metadata (read-only) ── */}
        <div className='bs-settings-panel'>
          <p className='bs-settings-panel__header'>Instance Metadata</p>
          <div className='bs-settings-panel__row'>
            <div className='bs-settings-panel__label'>Mode</div>
            <div className='bs-settings-panel__control'>
              <span className='platform-badge'>{draft.mode}</span>
            </div>
          </div>
          <div className='bs-settings-panel__row'>
            <div className='bs-settings-panel__label'>Agents configured</div>
            <div className='bs-settings-panel__control'>{draft.agents.length}</div>
          </div>
          <div className='bs-settings-panel__row'>
            <div className='bs-settings-panel__label'>Registries configured</div>
            <div className='bs-settings-panel__control'>{draft.registries.length}</div>
          </div>
          <div className='bs-settings-panel__row'>
            <div className='bs-settings-panel__label'>Global overrides</div>
            <div className='bs-settings-panel__control'>{draft.secrets.globalOverrides.length}</div>
          </div>
          <div className='bs-settings-panel__row'>
            <div className='bs-settings-panel__label'>Last saved</div>
            <div className='bs-settings-panel__control'>
              {draft.updatedAt ? new Date(draft.updatedAt).toLocaleString() : 'Never'}
            </div>
          </div>
        </div>
      </div>
    </AppShell>
  )
}
