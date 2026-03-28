import type { ReactNode } from 'react'
import { startTransition, useEffect, useMemo, useState } from 'react'
import {
  FaChevronDown,
  FaCodeBranch,
  FaFloppyDisk,
  FaLayerGroup,
  FaPlus,
  FaShieldHalved,
  FaTrash,
  FaWandMagicSparkles,
} from 'react-icons/fa6'
import AppShell from '../components/AppShell'
import SlidingPanel from '../components/SlidingPanel'
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
import './Profiles.css'

const NEW_PROFILE_ID = '__new__'

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
        const nextSuiteId = suites.some((suite) => suite.id === selectedSuiteId)
          ? selectedSuiteId
          : suites[0]?.id ?? ''
        setSelectedSuiteId(nextSuiteId)
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
    })
  }, [selectedProfile, isCreating])

  const filteredProfiles = profiles.filter((profile) => {
    const haystack = [profile.name, profile.fileName, profile.description, profile.scope].join(' ').toLowerCase()
    return haystack.includes(search.trim().toLowerCase())
  })

  const mergeRows = useMemo(() => {
    if (!draft) return [] as MergeRow[]
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
      const fallback = profiles.find((p) => p.id === suiteData?.defaultProfileId) ?? profiles[0] ?? null
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
      const suites = await listProfileSuites()
      setSuiteSummaries(suites)
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
    const nextIndex = profiles.filter((profile) => profile.launchable).length + 1
    const profile: ProfileRecord = {
      id: NEW_PROFILE_ID,
      name: `New Profile ${nextIndex}`,
      fileName: `profile-${nextIndex}.yaml`,
      description: 'New suite-scoped execution context for an environment override.',
      scope: 'Local',
      default: false,
      extendsId: profiles.find((item) => !item.launchable)?.id ?? profiles[0]?.id ?? '',
      yaml: 'env:\n  LOG_LEVEL: info\n',
      secretRefs: [],
      launchable: true,
      updatedAt: new Date().toISOString(),
    }

    startTransition(() => {
      setSelectedProfileId(NEW_PROFILE_ID)
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
      const suites = await listProfileSuites()
      setSuiteSummaries(suites)
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
      const suites = await listProfileSuites()
      setSuiteSummaries(suites)
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

  const panelHeader = (
    <div className='profiles-panel-header'>
      <div className='profiles-panel-header__info'>
        <div className='profiles-panel-header__name'>
          <strong>{draft?.name ?? 'Profile'}</strong>
          {draft?.default && <span className='profile-badge profile-badge--default'>Default</span>}
          {draft && !draft.launchable && <span className='profile-badge profile-badge--base'>Base</span>}
          {isCreating && <span className='profile-badge profile-badge--new'>New</span>}
        </div>
        <p className='profiles-panel-header__file'>{draft?.fileName ?? ''}</p>
      </div>
      <div className='profiles-panel-header__actions'>
        {(editing || isCreating) && (
          <button
            type='button'
            className='profiles-panel-btn profiles-panel-btn--ghost'
            onClick={() => {
              if (isCreating) {
                closePanel()
              } else if (selectedProfile) {
                setDraft(structuredClone(selectedProfile))
                setEditing(false)
                setMessage('')
                setError('')
              }
            }}
          >
            Discard
          </button>
        )}
        <button
          type='button'
          className='profiles-panel-btn profiles-panel-btn--ghost'
          onClick={() => void markDefault()}
          disabled={!draft?.launchable || isCreating}
        >
          <FaLayerGroup />
          <span>Default</span>
        </button>
        <button
          type='button'
          className='profiles-panel-btn profiles-panel-btn--danger'
          onClick={() => void deleteProfile()}
          disabled={!draft || (!draft.launchable && !isCreating)}
        >
          <FaTrash />
        </button>
        <button
          type='button'
          className='profiles-panel-btn profiles-panel-btn--primary'
          onClick={() => void saveProfile()}
          disabled={!draft || saving || (!editing && !isCreating)}
        >
          <FaFloppyDisk />
          <span>{saving ? 'Saving…' : 'Save'}</span>
        </button>
      </div>
    </div>
  )

  return (
    <AppShell
      section='Profiles'
      title='Profiles & Configuration'
      description='Maintain suite-scoped YAML execution contexts, visualize deep merges on top of the base runtime, and keep secret references out of raw plaintext.'
    >
      <div className='profiles-page'>
        {/* ── Toolbar ── */}
        <div className='profiles-toolbar'>
          <div className='profiles-toolbar__filters'>
            <select
              className='profiles-toolbar__select'
              value={selectedSuiteId}
              onChange={(e) => {
                setSelectedSuiteId(e.target.value)
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
              className='profiles-toolbar__search'
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder='Search profiles…'
            />
          </div>
          <button
            type='button'
            className='profiles-new-btn'
            onClick={createProfile}
            disabled={!suiteData || loading}
          >
            <FaPlus />
            <span>New Profile</span>
          </button>
        </div>

        {/* ── Suite meta bar ── */}
        {selectedSuite && (
          <div className='profiles-suite-bar'>
            <span className='profiles-suite-bar__repo'>{selectedSuite.repository}</span>
            <span className='profiles-suite-bar__counts'>
              {selectedSuite.launchableCount} launchable · {selectedSuite.profileCount} total
            </span>
          </div>
        )}

        {/* ── Alerts ── */}
        {message && <div className='profiles-alert profiles-alert--info'>{message}</div>}
        {error && <div className='profiles-alert profiles-alert--error'>{error}</div>}

        {/* ── Card grid ── */}
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
              <button
                key={profile.id}
                type='button'
                className={[
                  'profile-card',
                  profile.default ? 'profile-card--default' : '',
                  !profile.launchable ? 'profile-card--base' : '',
                ].filter(Boolean).join(' ')}
                onClick={() => openProfile(profile)}
              >
                <div className='profile-card__body'>
                  <div className='profile-card__top'>
                    <span className='profile-card__name'>{profile.name}</span>
                    <div className='profile-card__badges'>
                      {profile.default && <span className='profile-badge profile-badge--default'>Default</span>}
                      {!profile.launchable && <span className='profile-badge profile-badge--base'>Base</span>}
                    </div>
                  </div>
                  <p className='profile-card__filename'>{profile.fileName}</p>
                  {profile.description && (
                    <p className='profile-card__desc'>{profile.description}</p>
                  )}
                </div>
                <div className='profile-card__footer'>
                  <span className='profile-card__scope'>{profile.scope}</span>
                  {profile.secretRefs.length > 0 && (
                    <span className='profile-card__secrets'>
                      <FaShieldHalved />
                      {profile.secretRefs.length}
                    </span>
                  )}
                  <span className='profile-card__date'>
                    {profile.updatedAt ? new Date(profile.updatedAt).toLocaleDateString() : ''}
                  </span>
                </div>
              </button>
            ))}
          </div>
        )}

        {/* ── Editor panel ── */}
        <SlidingPanel isOpen={panelOpen} onClose={closePanel} width='700px' header={panelHeader}>
          <div className='profiles-panel-body'>
          {message && <div className='profiles-alert profiles-alert--info'>{message}</div>}
          {error && <div className='profiles-alert profiles-alert--error'>{error}</div>}

          {draft && (
            <>
              <CollapsibleSection
                title='YAML Profile'
                open={openSections.yaml}
                onToggle={() => setOpenSections((cur) => ({ ...cur, yaml: !cur.yaml }))}
              >
                <div className='profiles-form-grid'>
                  <label className='profiles-field'>
                    <span>Display Name</span>
                    <input
                      value={draft.name}
                      onChange={(e) => { setEditing(true); setDraft((cur) => cur ? { ...cur, name: e.target.value } : cur) }}
                    />
                  </label>
                  <label className='profiles-field'>
                    <span>File Name</span>
                    <input
                      value={draft.fileName}
                      onChange={(e) => { setEditing(true); setDraft((cur) => cur ? { ...cur, fileName: e.target.value } : cur) }}
                    />
                  </label>
                  <label className='profiles-field'>
                    <span>Scope</span>
                    <input
                      value={draft.scope}
                      onChange={(e) => { setEditing(true); setDraft((cur) => cur ? { ...cur, scope: e.target.value } : cur) }}
                    />
                  </label>
                  <label className='profiles-field'>
                    <span>Base Profile</span>
                    <select
                      value={draft.extendsId ?? ''}
                      onChange={(e) => { setEditing(true); setDraft((cur) => cur ? { ...cur, extendsId: e.target.value } : cur) }}
                    >
                      <option value=''>No base</option>
                      {profiles.filter((p) => p.id !== draft.id).map((p) => (
                        <option key={p.id} value={p.id}>{p.fileName}</option>
                      ))}
                    </select>
                  </label>
                  <label className='profiles-field profiles-field--full'>
                    <span>Description</span>
                    <input
                      value={draft.description}
                      onChange={(e) => { setEditing(true); setDraft((cur) => cur ? { ...cur, description: e.target.value } : cur) }}
                    />
                  </label>
                  <label className='profiles-field profiles-field--full'>
                    <span>YAML</span>
                    <textarea
                      value={draft.yaml}
                      onChange={(e) => { setEditing(true); setDraft((cur) => cur ? { ...cur, yaml: e.target.value } : cur) }}
                    />
                  </label>
                </div>
              </CollapsibleSection>

              <CollapsibleSection
                title='Secret References'
                open={openSections.secrets}
                onToggle={() => setOpenSections((cur) => ({ ...cur, secrets: !cur.secrets }))}
              >
                <div className='profiles-secrets'>
                  {draft.secretRefs.map((secret, index) => (
                    <div key={`${secret.key}-${index}`} className='profiles-secret-row'>
                      <label className='profiles-field'>
                        <span>Key</span>
                        <input
                          value={secret.key}
                          onChange={(e) => {
                            setEditing(true)
                            setDraft((cur) => {
                              if (!cur) return cur
                              const next = structuredClone(cur)
                              next.secretRefs[index].key = e.target.value
                              return next
                            })
                          }}
                        />
                      </label>
                      <label className='profiles-field'>
                        <span>Provider</span>
                        <input
                          value={secret.provider}
                          onChange={(e) => {
                            setEditing(true)
                            setDraft((cur) => {
                              if (!cur) return cur
                              const next = structuredClone(cur)
                              next.secretRefs[index].provider = e.target.value
                              return next
                            })
                          }}
                        />
                      </label>
                      <label className='profiles-field profiles-field--full'>
                        <span>Reference</span>
                        <input
                          value={secret.ref}
                          onChange={(e) => {
                            setEditing(true)
                            setDraft((cur) => {
                              if (!cur) return cur
                              const next = structuredClone(cur)
                              next.secretRefs[index].ref = e.target.value
                              return next
                            })
                          }}
                        />
                      </label>
                    </div>
                  ))}
                  <button
                    type='button'
                    className='profiles-inline-button'
                    onClick={() => {
                      setEditing(true)
                      setDraft((cur) => cur ? {
                        ...cur,
                        secretRefs: [...cur.secretRefs, { key: '', provider: 'Vault', ref: '' }],
                      } : cur)
                    }}
                  >
                    <FaShieldHalved />
                    <span>Add Secret Reference</span>
                  </button>
                </div>
              </CollapsibleSection>

              <CollapsibleSection
                title='Deep Merge Visualizer'
                open={openSections.merge}
                onToggle={() => setOpenSections((cur) => ({ ...cur, merge: !cur.merge }))}
              >
                <div className='profiles-merge'>
                  <div className='profiles-merge__header'>
                    <div>
                      <FaCodeBranch />
                      <span>{profiles.find((p) => p.id === draft.extendsId)?.fileName ?? 'No base'}</span>
                    </div>
                    <div>
                      <FaWandMagicSparkles />
                      <span>{draft.fileName}</span>
                    </div>
                  </div>
                  <div className='profiles-merge__table'>
                    <div className='profiles-merge__head'>
                      <span>Key</span>
                      <span>Base</span>
                      <span>Override</span>
                      <span>Result</span>
                    </div>
                    {mergeRows.map((row) => (
                      <div key={row.key} className={`profiles-merge__row${row.conflicted ? ' profiles-merge__row--conflicted' : ''}`}>
                        <strong>{row.key}</strong>
                        <span>{row.baseValue || '(none)'}</span>
                        <span>{row.overrideValue || '(none)'}</span>
                        <span className='profiles-merge__result'>
                          {row.resultValue || '(none)'}
                          <small>{row.source}</small>
                        </span>
                      </div>
                    ))}
                  </div>
                </div>
              </CollapsibleSection>
            </>
          )}
          </div>
        </SlidingPanel>
      </div>
    </AppShell>
  )
}

function toUpsertPayload(profile: ProfileRecord) {
  return {
    name: profile.name,
    fileName: profile.fileName,
    description: profile.description,
    scope: profile.scope,
    yaml: profile.yaml,
    secretRefs: profile.secretRefs,
    default: profile.default,
    extendsId: profile.extendsId ?? '',
  }
}

function CollapsibleSection(props: {
  title: string
  open: boolean
  onToggle: () => void
  children: ReactNode
}) {
  return (
    <section className={`profiles-section${props.open ? ' profiles-section--open' : ''}`}>
      <button type='button' className='profiles-section__header' onClick={props.onToggle}>
        <span>{props.title}</span>
        <FaChevronDown />
      </button>
      <div className='profiles-section__body'>{props.children}</div>
    </section>
  )
}

interface MergeRow {
  key: string
  baseValue: string
  overrideValue: string
  resultValue: string
  source: string
  conflicted: boolean
}

function buildMergeRows(baseYaml: string, overrideYaml: string) {
  const baseParsed = parseSimpleYaml(baseYaml).values
  const overrideParsed = parseSimpleYaml(overrideYaml).values
  const keys = Array.from(new Set([...Object.keys(baseParsed), ...Object.keys(overrideParsed)])).sort()

  return keys.map((key) => {
    const baseValue = baseParsed[key] ?? ''
    const overrideValue = overrideParsed[key] ?? ''
    const resultValue = overrideValue || baseValue || ''
    const conflicted = Boolean(baseValue && overrideValue && baseValue !== overrideValue)

    return {
      key,
      baseValue,
      overrideValue,
      resultValue,
      source: overrideValue ? 'Override' : 'Base',
      conflicted,
    }
  })
}

function parseSimpleYaml(yaml: string) {
  const values: Record<string, string> = {}
  const errors: string[] = []
  const stack: string[] = []
  const indents: number[] = []

  for (const rawLine of yaml.split('\n')) {
    if (rawLine.trim() === '' || rawLine.trim().startsWith('#')) continue

    const indent = rawLine.length - rawLine.trimStart().length
    const trimmed = rawLine.trim()
    const separatorIndex = trimmed.indexOf(':')

    if (separatorIndex === -1) {
      errors.push(`Invalid YAML line: "${trimmed}"`)
      continue
    }

    while (indents.length > 0 && indent <= indents[indents.length - 1]) {
      indents.pop()
      stack.pop()
    }

    const key = trimmed.slice(0, separatorIndex).trim()
    const value = trimmed.slice(separatorIndex + 1).trim()

    if (value === '') {
      stack.push(key)
      indents.push(indent)
      continue
    }

    values[[...stack, key].join('.')] = value
  }

  return { values, errors }
}
