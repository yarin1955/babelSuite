import { useEffect, useMemo, useRef, useState } from 'react'
import Editor from '@monaco-editor/react'
import { useLocation, useNavigate } from 'react-router-dom'
import {
  FaFloppyDisk, FaPenToSquare, FaPlus, FaTrash,
} from 'react-icons/fa6'
import Layout from '../components/Layout'
import Page from '../components/Page'
import '../lib/monaco'
import styles from './Profiles.module.css'

const API = 'http://localhost:8090'
const PANEL_ANIMATION_MS = 240

interface Profile {
  profile_id: string
  org_id: string
  name: string
  description: string
  format: 'yaml' | 'json'
  content: string
  revision: number
  created_by: string
  created_by_name: string
  updated_by: string
  updated_by_name: string
  created_at: string
  updated_at: string
}

interface DraftProfile extends Profile {
  isNew: boolean
}

type PanelMode = 'create' | 'edit'

const authHeaders = () => ({ 'Content-Type': 'application/json', Authorization: `Bearer ${localStorage.getItem('token') || ''}` })

const yamlTemplate = `services:
  api:
    env:
      FEATURE_FLAG: "enabled"
      REGION: "emea"

tests:
  env:
    BASE_URL: "https://example.internal"
`

const jsonTemplate = JSON.stringify({
  services: {
    api: {
      env: {
        FEATURE_FLAG: 'enabled',
        REGION: 'emea',
      },
    },
  },
  tests: {
    env: {
      BASE_URL: 'https://example.internal',
    },
  },
}, null, 2)

const blankDraft = (format: 'yaml' | 'json' = 'yaml'): DraftProfile => ({
  profile_id: '',
  org_id: '',
  name: '',
  description: '',
  format,
  content: format === 'json' ? jsonTemplate : yamlTemplate,
  revision: 0,
  created_by: '',
  created_by_name: '',
  updated_by: '',
  updated_by_name: '',
  created_at: '',
  updated_at: '',
  isNew: true,
})

const serializeDraft = (profile: DraftProfile | null) => profile
  ? JSON.stringify({ name: profile.name, description: profile.description, format: profile.format, content: profile.content })
  : ''

const timeAgo = (iso?: string) => {
  if (!iso) return 'never'
  const diff = Math.floor((Date.now() - new Date(iso).getTime()) / 1000)
  if (diff < 60) return `${diff}s ago`
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`
  return `${Math.floor(diff / 86400)}d ago`
}

export default function Profiles() {
  const nav = useNavigate()
  const location = useLocation()
  const inSettingsFlow = location.pathname.startsWith('/settings/')
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [loading, setLoading] = useState(true)
  const [pageErr, setPageErr] = useState('')
  const [search, setSearch] = useState('')

  const [panelMode, setPanelMode] = useState<PanelMode | null>(null)
  const [panelMounted, setPanelMounted] = useState(false)
  const [panelVisible, setPanelVisible] = useState(false)
  const [panelLoading, setPanelLoading] = useState(false)
  const [panelSaving, setPanelSaving] = useState(false)
  const [panelErr, setPanelErr] = useState('')
  const [panelDraft, setPanelDraft] = useState<DraftProfile | null>(null)
  const [panelSnapshot, setPanelSnapshot] = useState('')
  const closeTimerRef = useRef<number | null>(null)

  const filteredProfiles = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return profiles
    return profiles.filter(profile =>
      [profile.name, profile.description, profile.updated_by_name, profile.created_by_name].join(' ').toLowerCase().includes(q),
    )
  }, [profiles, search])

  const panelDirty = panelSnapshot !== serializeDraft(panelDraft)
  const panelIsCreate = panelMode === 'create'
  const panelIsEdit = panelMode === 'edit'

  const loadProfiles = async () => {
    setLoading(true)
    try {
      const res = await fetch(`${API}/api/profiles`, { headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` } })
      const data = await res.json().catch(() => null)
      if (!res.ok) {
        setPageErr((data as { error?: string } | null)?.error || 'Failed to load profiles')
        return
      }
      const list = Array.isArray(data) ? data : []
      setProfiles(list)
      setPageErr('')
    } catch {
      setPageErr('Failed to load profiles')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void loadProfiles() }, [])
  useEffect(() => () => {
    if (closeTimerRef.current != null) {
      window.clearTimeout(closeTimerRef.current)
    }
  }, [])

  const openPanel = (mode: PanelMode, draft: DraftProfile | null, loadingState = false) => {
    if (closeTimerRef.current != null) {
      window.clearTimeout(closeTimerRef.current)
      closeTimerRef.current = null
    }
    setPanelMode(mode)
    setPanelDraft(draft)
    setPanelSnapshot(serializeDraft(draft))
    setPanelLoading(loadingState)
    setPanelSaving(false)
    setPanelErr('')
    setPanelMounted(true)
    window.requestAnimationFrame(() => setPanelVisible(true))
  }

  const confirmPanelLeave = () => !panelDirty || window.confirm('Discard unsaved changes?')

  const finishClosePanel = () => {
    setPanelMode(null)
    setPanelMounted(false)
    setPanelVisible(false)
    setPanelLoading(false)
    setPanelSaving(false)
    setPanelErr('')
    setPanelDraft(null)
    setPanelSnapshot('')
  }

  const closePanel = (force = false) => {
    if (!force && !confirmPanelLeave()) return
    setPanelVisible(false)
    if (closeTimerRef.current != null) {
      window.clearTimeout(closeTimerRef.current)
    }
    closeTimerRef.current = window.setTimeout(() => {
      finishClosePanel()
      closeTimerRef.current = null
    }, PANEL_ANIMATION_MS)
  }

  const openCreate = () => {
    if (!confirmPanelLeave()) return
    openPanel('create', blankDraft())
  }

  const openEdit = async (profile: Profile) => {
    if (!confirmPanelLeave()) return
    openPanel('edit', null, true)
    try {
      const res = await fetch(`${API}/api/profiles/${profile.profile_id}`, { headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` } })
      const data = await res.json().catch(() => null)
      if (!res.ok || !data) {
        setPanelErr((data as { error?: string } | null)?.error || 'Failed to load profile')
        setPanelLoading(false)
        return
      }
      const next = { ...data, isNew: false } as DraftProfile
      setPanelDraft(next)
      setPanelSnapshot(serializeDraft(next))
      setPanelErr('')
    } catch {
      setPanelErr('Failed to load profile')
    } finally {
      setPanelLoading(false)
    }
  }

  const setDraftFormat = (format: 'yaml' | 'json') => {
    setPanelDraft(current => {
      if (!current) return current
      const shouldSwapTemplate = current.content === yamlTemplate || current.content === jsonTemplate
      return {
        ...current,
        format,
        content: shouldSwapTemplate ? (format === 'json' ? jsonTemplate : yamlTemplate) : current.content,
      }
    })
  }

  const savePanel = async () => {
    if (!panelDraft) return
    if (!panelDraft.name.trim()) {
      setPanelErr('Name is required')
      return
    }
    if (panelDraft.format === 'json') {
      try {
        JSON.parse(panelDraft.content)
      } catch {
        setPanelErr('JSON content is not valid')
        return
      }
    }

    setPanelSaving(true)
    try {
      const payload = {
        name: panelDraft.name.trim(),
        description: panelDraft.description.trim(),
        format: panelDraft.format,
        content: panelDraft.content,
        base_updated_at: panelIsEdit ? panelDraft.updated_at || undefined : undefined,
      }
      const method = panelIsCreate ? 'POST' : 'PUT'
      const path = panelIsCreate ? '/api/profiles' : `/api/profiles/${panelDraft.profile_id}`
      const res = await fetch(`${API}${path}`, { method, headers: authHeaders(), body: JSON.stringify(payload) })
      const data = await res.json().catch(() => ({}))
      if (!res.ok) {
        setPanelErr(data.error || (panelIsCreate ? 'Create failed' : 'Save failed'))
        return
      }

      if (panelIsCreate) {
        closePanel(true)
      } else {
        const next = { ...data, isNew: false } as DraftProfile
        setPanelDraft(next)
        setPanelSnapshot(serializeDraft(next))
      }
      setPanelErr('')
      await loadProfiles()
    } finally {
      setPanelSaving(false)
    }
  }

  const deleteProfile = async () => {
    if (!panelDraft || !panelIsEdit) return
    if (!window.confirm(`Delete profile "${panelDraft.name}"?`)) return
    const res = await fetch(`${API}/api/profiles/${panelDraft.profile_id}`, { method: 'DELETE', headers: { Authorization: `Bearer ${localStorage.getItem('token') || ''}` } })
    if (!res.ok) {
      const data = await res.json().catch(() => ({ error: 'Delete failed' }))
      setPanelErr(data.error || 'Delete failed')
      return
    }
    closePanel(true)
    await loadProfiles()
  }

  return (
    <Layout>
      <Page
        title='Profiles'
        toolbar={
          <div className={styles.toolbarActions}>
            {inSettingsFlow && <button className='app-button app-button--secondary' onClick={() => nav('/settings')}>Back to Settings</button>}
            <button className='app-button app-button--primary' onClick={openCreate}>
              <FaPlus />
              New profile
            </button>
          </div>
        }
      >
        {pageErr && <div className='auth-error'>{pageErr}</div>}

        <section className={styles.listShell}>
          <div className={styles.listToolbar}>
            <div>
              <div className='panel-card__eyebrow'>Profiles</div>
              <h3>Shared environment profiles</h3>
            </div>
            <div className={styles.listTools}>
              <input className={styles.searchInput} value={search} onChange={e => setSearch(e.target.value)} placeholder='Filter profiles...' />
              <span className={styles.listCount}>{filteredProfiles.length} shown</span>
            </div>
          </div>

          <div className='panel-card panel-card--flush'>
            {loading ? (
              <div className='app-empty-state'>Loading profiles...</div>
            ) : filteredProfiles.length === 0 ? (
              <div className='app-empty-state'>
                <div className='app-empty-state__title'>{profiles.length === 0 ? 'No profiles yet' : 'No matching profiles'}</div>
                <div className='app-empty-state__desc'>
                  {profiles.length === 0
                    ? 'Create a shared profile to start managing suite environment configuration.'
                    : 'Try a broader search or clear the current filter.'}
                </div>
                {profiles.length === 0 && (
                  <button className='app-button app-button--primary' onClick={openCreate}>
                    <FaPlus />
                    Create profile
                  </button>
                )}
              </div>
            ) : (
              <div className={styles.profileList}>
                {filteredProfiles.map(profile => (
                  <button key={profile.profile_id} className={styles.profileRow} onClick={() => void openEdit(profile)}>
                    <div className={styles.profileMain}>
                      <div className={styles.profileTitleRow}>
                        <strong>{profile.name}</strong>
                        <span className='app-chip'>{profile.format}</span>
                        <span className='app-chip'>v{profile.revision}</span>
                      </div>
                      <div className={styles.profileDescription}>{profile.description || 'No description'}</div>
                    </div>
                    <div className={styles.profileSide}>
                      <span>{profile.updated_by_name || 'unknown'}</span>
                      <span>{timeAgo(profile.updated_at)}</span>
                      <span className={styles.profileAction}><FaPenToSquare />Open</span>
                    </div>
                  </button>
                ))}
              </div>
            )}
          </div>
        </section>
      </Page>

      {panelMounted && (
        <div className={`${styles.panelOverlay} ${panelVisible ? styles.panelOverlayVisible : ''}`} onClick={() => closePanel()}>
          <aside className={`${styles.panelDrawer} ${panelVisible ? styles.panelDrawerVisible : ''}`} onClick={event => event.stopPropagation()}>
            <div className={styles.panelHeader}>
              <div>
                <div className={styles.panelEyebrow}>{panelIsCreate ? 'Create profile' : 'Edit profile'}</div>
                <h3>{panelIsCreate ? 'New shared profile' : panelDraft?.name || 'Profile'}</h3>
              </div>
              <div className={styles.panelActions}>
                {panelIsEdit && panelDraft && (
                  <button className='app-button app-button--secondary' onClick={() => void deleteProfile()} disabled={panelSaving || panelLoading}>
                    <FaTrash />
                    Delete
                  </button>
                )}
                <button className='app-button app-button--secondary' onClick={() => closePanel()} disabled={panelSaving}>Cancel</button>
                <button className='app-button app-button--primary' onClick={() => void savePanel()} disabled={panelSaving || panelLoading || !panelDraft?.name.trim()}>
                  <FaFloppyDisk />
                  {panelSaving ? (panelIsCreate ? 'Creating...' : 'Saving...') : panelIsCreate ? 'Create' : 'Save'}
                </button>
              </div>
            </div>

            <div className={styles.panelBody}>
              {panelErr && <div className='auth-error'>{panelErr}</div>}

              {panelLoading ? (
                <div className='app-empty-state'>Loading profile...</div>
              ) : !panelDraft ? (
                <div className='app-empty-state'>Profile data is unavailable.</div>
              ) : (
                <>
                  <section className={styles.panelSection}>
                    <div className={styles.sectionHeading}>General</div>
                    <div className={styles.formGrid}>
                      <label className={styles.fieldGroup}>
                        <span className={styles.fieldLabel}>Name</span>
                        <input className={styles.fieldInput} value={panelDraft.name} onChange={e => setPanelDraft(curr => curr ? { ...curr, name: e.target.value } : curr)} placeholder='fleet-defaults' />
                      </label>
                      <label className={styles.fieldGroup}>
                        <span className={styles.fieldLabel}>Format</span>
                        <select className={styles.fieldInput} value={panelDraft.format} onChange={e => setDraftFormat(e.target.value as 'yaml' | 'json')}>
                          <option value='yaml'>YAML</option>
                          <option value='json'>JSON</option>
                        </select>
                      </label>
                    </div>
                    <label className={styles.fieldGroup}>
                      <span className={styles.fieldLabel}>Description</span>
                      <input className={styles.fieldInput} value={panelDraft.description} onChange={e => setPanelDraft(curr => curr ? { ...curr, description: e.target.value } : curr)} placeholder='Shared env vars for the default validation lane' />
                    </label>
                  </section>

                  <section className={styles.panelSection}>
                    <div className={styles.panelSectionTop}>
                      <div>
                        <div className={styles.sectionHeading}>Content</div>
                        <div className={styles.sectionCopy}>Edit the raw {panelDraft.format.toUpperCase()} document stored for this shared profile.</div>
                      </div>
                      <button className='app-button app-button--secondary' onClick={() => setPanelDraft(curr => curr ? { ...curr, content: curr.format === 'json' ? jsonTemplate : yamlTemplate } : curr)} disabled={panelSaving}>
                        Reset template
                      </button>
                    </div>
                    <div className={styles.editorWrap}>
                      <Editor
                        height='56vh'
                        language={panelDraft.format}
                        theme='vs'
                        value={panelDraft.content}
                        onChange={value => setPanelDraft(curr => curr ? { ...curr, content: value ?? '' } : curr)}
                        options={{
                          automaticLayout: true,
                          fontSize: 13,
                          minimap: { enabled: false },
                          scrollBeyondLastLine: false,
                          wordWrap: 'on',
                          tabSize: 2,
                        }}
                      />
                    </div>
                  </section>

                  {panelIsEdit && (
                    <section className={styles.panelMeta}>
                      <span>Created by {panelDraft.created_by_name || 'unknown'} {panelDraft.created_at ? `on ${new Date(panelDraft.created_at).toLocaleString()}` : ''}</span>
                      <span>Last updated by {panelDraft.updated_by_name || 'unknown'} {panelDraft.updated_at ? `${timeAgo(panelDraft.updated_at)} (${new Date(panelDraft.updated_at).toLocaleString()})` : ''}</span>
                    </section>
                  )}
                </>
              )}
            </div>
          </aside>
        </div>
      )}
    </Layout>
  )
}
