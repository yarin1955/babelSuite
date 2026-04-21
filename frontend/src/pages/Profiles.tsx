import { startTransition, useEffect, useMemo, useRef, useState } from 'react'
import { FaPlus } from 'react-icons/fa6'
import AppShell from '../components/AppShell'
import { ProfileCard } from '../components/profiles/ProfileCard'
import { ProfileEditorPanel } from '../components/profiles/ProfileEditorPanel'
import {
  createSuiteProfile,
  deleteSuiteProfile,
  getSuiteProfiles,
  listProfileSuites,
  setDefaultSuiteProfile,
  updateSuiteProfile,
  type ProfileRecord,
  type ProfileSuiteSummary,
  type SuiteProfilesResponse,
} from '../lib/api'
import {
  buildMergeRows,
  createDraftProfile,
  toUpsertPayload,
} from '../lib/profiles'
import { parseSimpleYaml } from '../lib/simpleYaml'
import './Profiles.css'

export default function Profiles() {
  const [suiteSummaries, setSuiteSummaries] = useState<ProfileSuiteSummary[]>([])
  const [selectedSuiteId, setSelectedSuiteId] = useState('')
  const [suiteData, setSuiteData] = useState<SuiteProfilesResponse | null>(null)
  const [selectedProfileId, setSelectedProfileId] = useState('')
  const [search, setSearch] = useState('')
  const [editing, setEditing] = useState(false)
  const [isCreating, setIsCreating] = useState(false)
  const [draft, setDraft] = useState<ProfileRecord | null>(null)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [panelOpen, setPanelOpen] = useState(false)
  const [openSections, setOpenSections] = useState({
    yaml: true,
    secrets: true,
    merge: true,
  })
  const [editorMode, setEditorMode] = useState<'yaml' | 'json'>('yaml')
  const [jsonText, setJsonText] = useState('')
  const searchRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      const activeTag = document.activeElement?.tagName
      if (event.key === '/' && activeTag !== 'INPUT' && activeTag !== 'TEXTAREA') {
        event.preventDefault()
        searchRef.current?.focus()
      }
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [])

  const profiles = suiteData?.profiles ?? []
  const selectedSuite = suiteSummaries.find((suite) => suite.id === selectedSuiteId) ?? suiteSummaries[0] ?? null
  const selectedProfile = !isCreating
    ? profiles.find((profile) => profile.id === selectedProfileId) ?? profiles[0] ?? null
    : null

  useEffect(() => {
    let active = true

    const loadSuites = async () => {
      try {
        const suites = await listProfileSuites()
        if (!active) return
        setSuiteSummaries(suites)
        setSelectedSuiteId(suites.some((suite) => suite.id === selectedSuiteId) ? selectedSuiteId : suites[0]?.id ?? '')
      } catch (loadError) {
        if (!active) return
        setError(loadError instanceof Error ? loadError.message : 'Could not load suites.')
        setLoading(false)
      }
    }

    void loadSuites()
    return () => { active = false }
  }, [])

  useEffect(() => {
    if (!selectedSuiteId) {
      setLoading(false)
      return
    }

    let active = true
    setLoading(true)
    setError('')
    setMessage('')

    const loadProfiles = async () => {
      try {
        const payload = await getSuiteProfiles(selectedSuiteId)
        if (!active) return
        applySuitePayload(payload)
      } catch (loadError) {
        if (!active) return
        setError(loadError instanceof Error ? loadError.message : 'Could not load profiles.')
        setLoading(false)
      }
    }

    void loadProfiles()
    return () => { active = false }
  }, [selectedSuiteId])

  useEffect(() => {
    if (isCreating || !selectedProfile) return
    startTransition(() => {
      setDraft(structuredClone(selectedProfile))
      setEditing(false)
      setEditorMode('yaml')
    })
  }, [selectedProfile, isCreating])

  const filteredProfiles = profiles.filter((profile) => {
    const haystack = [profile.name, profile.fileName, profile.description, profile.scope].join(' ').toLowerCase()
    return haystack.includes(search.trim().toLowerCase())
  })

  const mergeRows = useMemo(() => {
    if (!draft) return []
    const baseProfile = profiles.find((profile) => profile.id === draft.extendsId)
    return buildMergeRows(baseProfile?.yaml ?? '', draft.yaml)
  }, [draft, profiles])

  const openProfile = (profile: ProfileRecord) => {
    startTransition(() => {
      setSelectedProfileId(profile.id)
      setIsCreating(false)
      setEditing(false)
      setMessage('')
      setError('')
      setPanelOpen(true)
    })
  }

  const closePanel = () => {
    if (isCreating) {
      const fallback = profiles.find((profile) => profile.id === suiteData?.defaultProfileId) ?? profiles[0] ?? null
      startTransition(() => {
        setIsCreating(false)
        setDraft(fallback ? structuredClone(fallback) : null)
        setSelectedProfileId(fallback?.id ?? '')
        setEditing(false)
        setMessage('')
        setError('')
      })
    } else if (editing && selectedProfile) {
      startTransition(() => {
        setDraft(structuredClone(selectedProfile))
        setEditing(false)
        setMessage('')
        setError('')
      })
    }
    setPanelOpen(false)
  }

  const discardDraft = () => {
    if (isCreating) {
      closePanel()
      return
    }

    if (!selectedProfile) return
    setDraft(structuredClone(selectedProfile))
    setEditing(false)
    setMessage('')
    setError('')
  }

  const saveProfile = async () => {
    if (!draft || !suiteData) return

    const parsed = parseSimpleYaml(draft.yaml)
    if (parsed.errors.length > 0) {
      setError(parsed.errors[0])
      return
    }

    setSaving(true)
    setError('')

    try {
      const payload = toUpsertPayload(draft)
      const nextSuite = isCreating
        ? await createSuiteProfile(suiteData.suiteId, payload)
        : await updateSuiteProfile(suiteData.suiteId, draft.id, payload)

      applySuitePayload(nextSuite, isCreating ? draft.fileName : '', isCreating ? '' : draft.id)
      setSuiteSummaries(await listProfileSuites())
      setMessage(isCreating
        ? 'Profile created. Launchers now see the new suite-scoped override.'
        : 'Profile saved. Merge preview and launch options were updated.')
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : 'Could not save profile.')
    } finally {
      setSaving(false)
    }
  }

  const createProfile = () => {
    const profile = createDraftProfile(profiles)

    startTransition(() => {
      setSelectedProfileId(profile.id)
      setIsCreating(true)
      setEditing(true)
      setDraft(profile)
      setMessage('New profile drafted locally. Save to make it available in the suite launcher.')
      setError('')
      setPanelOpen(true)
    })
  }

  const deleteProfile = async () => {
    if (!draft || !suiteData) return

    if (isCreating) {
      closePanel()
      return
    }

    if (!draft.launchable) return

    try {
      const nextSuite = await deleteSuiteProfile(suiteData.suiteId, draft.id)
      applySuitePayload(nextSuite)
      setSuiteSummaries(await listProfileSuites())
      setPanelOpen(false)
      setMessage('Profile removed from the suite-scoped profile set.')
      setError('')
    } catch (deleteError) {
      setError(deleteError instanceof Error ? deleteError.message : 'Could not delete profile.')
    }
  }

  const markDefault = async () => {
    if (!draft || !suiteData || !draft.launchable || isCreating) return

    try {
      const nextSuite = await setDefaultSuiteProfile(suiteData.suiteId, draft.id)
      applySuitePayload(nextSuite, '', draft.id)
      setSuiteSummaries(await listProfileSuites())
      setMessage(`${draft.fileName} is now the default launch profile for ${suiteData.suiteTitle}.`)
      setError('')
    } catch (markError) {
      setError(markError instanceof Error ? markError.message : 'Could not set the default profile.')
    }
  }

  const applySuitePayload = (payload: SuiteProfilesResponse, preferredFileName = '', preferredProfileId = '') => {
    const nextSelected = payload.profiles.find((profile) => profile.id === preferredProfileId)
      ?? payload.profiles.find((profile) => profile.fileName === preferredFileName)
      ?? payload.profiles.find((profile) => profile.id === payload.defaultProfileId)
      ?? payload.profiles[0]
      ?? null

    startTransition(() => {
      setSuiteData(payload)
      setSelectedProfileId(nextSelected?.id ?? '')
      setDraft(nextSelected ? structuredClone(nextSelected) : null)
      setIsCreating(false)
      setEditing(false)
      setLoading(false)
    })
  }

  return (
    <AppShell
      section='Profiles'
      title='Profiles & Configuration'
      description='Maintain suite-scoped YAML execution contexts, visualize deep merges on top of the base runtime, and keep secret references out of raw plaintext.'
    >
      <div className='profiles-page'>
        <div className='profiles-toolbar'>
          <div className='profiles-toolbar__filters'>
            <select
              className='profiles-toolbar__select'
              value={selectedSuiteId}
              onChange={(event) => {
                setSelectedSuiteId(event.target.value)
                setSearch('')
                setMessage('')
                setError('')
              }}
            >
              {suiteSummaries.map((suite) => (
                <option key={suite.id} value={suite.id}>{suite.title}</option>
              ))}
            </select>
            <input
              ref={searchRef}
              className='profiles-toolbar__search'
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              onKeyDown={(event) => event.key === 'Escape' && searchRef.current?.blur()}
              placeholder='Search profiles...'
            />
          </div>
          <button type='button' className='profiles-new-btn' onClick={createProfile} disabled={!suiteData || loading}>
            <FaPlus />
            <span>New Profile</span>
          </button>
        </div>

        {selectedSuite && (
          <div className='profiles-suite-bar'>
            <span className='profiles-suite-bar__repo'>{selectedSuite.repository}</span>
            <span className='profiles-suite-bar__counts'>
              {selectedSuite.launchableCount} launchable / {selectedSuite.profileCount} total
            </span>
          </div>
        )}

        {message && <div className='profiles-alert profiles-alert--info'>{message}</div>}
        {error && <div className='profiles-alert profiles-alert--error'>{error}</div>}

        {loading ? (
          <div className='profiles-loading'>
            <div className='profiles-loading__card' />
            <div className='profiles-loading__card' />
            <div className='profiles-loading__card' />
          </div>
        ) : filteredProfiles.length === 0 ? (
          <div className='profiles-empty'>
            <strong>No profiles found.</strong>
            <p>Try a different search term, select a different suite, or create a new profile.</p>
          </div>
        ) : (
          <div className='profiles-grid'>
            {filteredProfiles.map((profile) => (
              <ProfileCard
                key={profile.id}
                profile={profile}
                selected={profile.id === selectedProfileId && panelOpen}
                onOpen={openProfile}
              />
            ))}
          </div>
        )}

        <ProfileEditorPanel
          isOpen={panelOpen}
          draft={draft}
          profiles={profiles}
          selectedProfile={selectedProfile}
          isCreating={isCreating}
          editing={editing}
          saving={saving}
          message={message}
          error={error}
          openSections={openSections}
          editorMode={editorMode}
          jsonText={jsonText}
          mergeRows={mergeRows}
          onClose={closePanel}
          onDiscard={discardDraft}
          onMarkDefault={() => void markDefault()}
          onDelete={() => void deleteProfile()}
          onSave={() => void saveProfile()}
          setDraft={setDraft}
          setEditing={setEditing}
          setOpenSections={setOpenSections}
          setEditorMode={setEditorMode}
          setJsonText={setJsonText}
        />
      </div>
    </AppShell>
  )
}
