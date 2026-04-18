import type { Dispatch, SetStateAction } from 'react'
import {
  FaCodeBranch,
  FaFloppyDisk,
  FaLayerGroup,
  FaShieldHalved,
  FaTrash,
  FaWandMagicSparkles,
} from 'react-icons/fa6'
import { CollapsibleSection } from '../CollapsibleSection'
import SlidingPanel from '../SlidingPanel'
import type { ProfileRecord } from '../../lib/api'
import type { MergeRow } from '../../lib/profiles'
import { objectToYaml, yamlToJson } from '../../lib/simpleYaml'
import './ProfileCard.css'
import './ProfileEditorPanel.css'

export interface ProfileEditorSections {
  yaml: boolean
  secrets: boolean
  merge: boolean
}

type EditorMode = 'yaml' | 'json'

export function ProfileEditorPanel({
  isOpen,
  draft,
  profiles,
  selectedProfile,
  isCreating,
  editing,
  saving,
  message,
  error,
  openSections,
  editorMode,
  jsonText,
  mergeRows,
  onClose,
  onDiscard,
  onMarkDefault,
  onDelete,
  onSave,
  setDraft,
  setEditing,
  setOpenSections,
  setEditorMode,
  setJsonText,
}: {
  isOpen: boolean
  draft: ProfileRecord | null
  profiles: ProfileRecord[]
  selectedProfile: ProfileRecord | null
  isCreating: boolean
  editing: boolean
  saving: boolean
  message: string
  error: string
  openSections: ProfileEditorSections
  editorMode: EditorMode
  jsonText: string
  mergeRows: MergeRow[]
  onClose: () => void
  onDiscard: () => void
  onMarkDefault: () => void
  onDelete: () => void
  onSave: () => void
  setDraft: Dispatch<SetStateAction<ProfileRecord | null>>
  setEditing: Dispatch<SetStateAction<boolean>>
  setOpenSections: Dispatch<SetStateAction<ProfileEditorSections>>
  setEditorMode: Dispatch<SetStateAction<EditorMode>>
  setJsonText: Dispatch<SetStateAction<string>>
}) {
  const updateDraft = (patch: Partial<ProfileRecord>) => {
    setEditing(true)
    setDraft((cur) => cur ? { ...cur, ...patch } : cur)
  }

  const updateSecret = (index: number, field: 'key' | 'provider' | 'ref', value: string) => {
    setEditing(true)
    setDraft((cur) => {
      if (!cur) return cur
      const next = structuredClone(cur)
      next.secretRefs[index][field] = value
      return next
    })
  }

  const header = (
    <ProfileEditorHeader
      draft={draft}
      selectedProfile={selectedProfile}
      isCreating={isCreating}
      editing={editing}
      saving={saving}
      onDiscard={onDiscard}
      onMarkDefault={onMarkDefault}
      onDelete={onDelete}
      onSave={onSave}
    />
  )

  return (
    <SlidingPanel isOpen={isOpen} onClose={onClose} width='700px' header={header}>
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
              <ProfileYamlEditor
                draft={draft}
                profiles={profiles}
                editorMode={editorMode}
                jsonText={jsonText}
                setDraft={setDraft}
                setEditing={setEditing}
                setEditorMode={setEditorMode}
                setJsonText={setJsonText}
                updateDraft={updateDraft}
              />
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
                      <input value={secret.key} onChange={(e) => updateSecret(index, 'key', e.target.value)} />
                    </label>
                    <label className='profiles-field'>
                      <span>Provider</span>
                      <input value={secret.provider} onChange={(e) => updateSecret(index, 'provider', e.target.value)} />
                    </label>
                    <label className='profiles-field profiles-field--full'>
                      <span>Reference</span>
                      <input value={secret.ref} onChange={(e) => updateSecret(index, 'ref', e.target.value)} />
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
              <ProfileMergePreview draft={draft} profiles={profiles} rows={mergeRows} />
            </CollapsibleSection>
          </>
        )}
      </div>
    </SlidingPanel>
  )
}

function ProfileEditorHeader({
  draft,
  selectedProfile,
  isCreating,
  editing,
  saving,
  onDiscard,
  onMarkDefault,
  onDelete,
  onSave,
}: {
  draft: ProfileRecord | null
  selectedProfile: ProfileRecord | null
  isCreating: boolean
  editing: boolean
  saving: boolean
  onDiscard: () => void
  onMarkDefault: () => void
  onDelete: () => void
  onSave: () => void
}) {
  return (
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
          <button type='button' className='profiles-panel-btn profiles-panel-btn--ghost' onClick={onDiscard}>
            Discard
          </button>
        )}
        <button
          type='button'
          className='profiles-panel-btn profiles-panel-btn--ghost'
          onClick={onMarkDefault}
          disabled={!draft?.launchable || isCreating}
        >
          <FaLayerGroup />
          <span>Default</span>
        </button>
        <button
          type='button'
          className='profiles-panel-btn profiles-panel-btn--danger'
          onClick={onDelete}
          disabled={!draft || (!draft.launchable && !isCreating)}
        >
          <FaTrash />
        </button>
        <button
          type='button'
          className='profiles-panel-btn profiles-panel-btn--primary'
          onClick={onSave}
          disabled={!draft || saving || (!editing && !isCreating) || !selectedProfile && !isCreating}
        >
          <FaFloppyDisk />
          <span>{saving ? 'Saving...' : 'Save'}</span>
        </button>
      </div>
    </div>
  )
}

function ProfileYamlEditor({
  draft,
  profiles,
  editorMode,
  jsonText,
  setDraft,
  setEditing,
  setEditorMode,
  setJsonText,
  updateDraft,
}: {
  draft: ProfileRecord
  profiles: ProfileRecord[]
  editorMode: EditorMode
  jsonText: string
  setDraft: Dispatch<SetStateAction<ProfileRecord | null>>
  setEditing: Dispatch<SetStateAction<boolean>>
  setEditorMode: Dispatch<SetStateAction<EditorMode>>
  setJsonText: Dispatch<SetStateAction<string>>
  updateDraft: (patch: Partial<ProfileRecord>) => void
}) {
  return (
    <div className='profiles-form-grid'>
      <label className='profiles-field'>
        <span>Display Name</span>
        <input value={draft.name} onChange={(e) => updateDraft({ name: e.target.value })} />
      </label>
      <label className='profiles-field'>
        <span>File Name</span>
        <input value={draft.fileName} onChange={(e) => updateDraft({ fileName: e.target.value })} />
      </label>
      <label className='profiles-field'>
        <span>Scope</span>
        <input value={draft.scope} onChange={(e) => updateDraft({ scope: e.target.value })} />
      </label>
      <label className='profiles-field'>
        <span>Base Profile</span>
        <select value={draft.extendsId ?? ''} onChange={(e) => updateDraft({ extendsId: e.target.value })}>
          <option value=''>No base</option>
          {profiles.filter((profile) => profile.id !== draft.id).map((profile) => (
            <option key={profile.id} value={profile.id}>{profile.fileName}</option>
          ))}
        </select>
      </label>
      <label className='profiles-field profiles-field--full'>
        <span>Description</span>
        <input value={draft.description} onChange={(e) => updateDraft({ description: e.target.value })} />
      </label>
      <label className='profiles-field profiles-field--full'>
        <span>Content</span>
        <div className='profiles-editor-wrap'>
          <div className='profiles-format-toggle'>
            <button
              type='button'
              className={`profiles-format-toggle__btn${editorMode === 'yaml' ? ' profiles-format-toggle__btn--active' : ''}`}
              onClick={() => {
                if (editorMode === 'json') {
                  try {
                    const yaml = objectToYaml(JSON.parse(jsonText) as Record<string, unknown>)
                    setDraft((cur) => cur ? { ...cur, yaml } : cur)
                  } catch {
                    /* keep current YAML when JSON is invalid */
                  }
                }
                setEditorMode('yaml')
              }}
            >
              YAML
            </button>
            <button
              type='button'
              className={`profiles-format-toggle__btn${editorMode === 'json' ? ' profiles-format-toggle__btn--active' : ''}`}
              onClick={() => {
                setJsonText(yamlToJson(draft.yaml))
                setEditorMode('json')
              }}
            >
              JSON
            </button>
          </div>
          {editorMode === 'yaml' ? (
            <textarea value={draft.yaml} onChange={(e) => updateDraft({ yaml: e.target.value })} />
          ) : (
            <textarea
              value={jsonText}
              onChange={(e) => {
                setJsonText(e.target.value)
                try {
                  const yaml = objectToYaml(JSON.parse(e.target.value) as Record<string, unknown>)
                  setEditing(true)
                  setDraft((cur) => cur ? { ...cur, yaml } : cur)
                } catch {
                  /* invalid JSON does not replace YAML */
                }
              }}
            />
          )}
        </div>
      </label>
    </div>
  )
}

function ProfileMergePreview({
  draft,
  profiles,
  rows,
}: {
  draft: ProfileRecord
  profiles: ProfileRecord[]
  rows: MergeRow[]
}) {
  return (
    <div className='profiles-merge'>
      <div className='profiles-merge__header'>
        <div className='profiles-merge__header-pill profiles-merge__header-pill--base'>
          <FaCodeBranch />
          <span>{profiles.find((profile) => profile.id === draft.extendsId)?.fileName ?? 'No base'}</span>
        </div>
        <div className='profiles-merge__header-pill profiles-merge__header-pill--override'>
          <FaWandMagicSparkles />
          <span>{draft.fileName}</span>
        </div>
      </div>
      {!draft.extendsId ? (
        <div className='profiles-merge__empty'>
          No base profile selected. Choose a base profile above to see the deep merge preview.
        </div>
      ) : rows.length === 0 ? (
        <div className='profiles-merge__empty'>
          No keys found. Add content to the YAML above to see the merge result.
        </div>
      ) : (
        <div className='profiles-merge__table'>
          <div className='profiles-merge__head'>
            <span>Key</span>
            <span>Base</span>
            <span>Override</span>
          </div>
          {rows.map((row) => (
            <div key={row.key} className={`profiles-merge__row${row.conflicted ? ' profiles-merge__row--conflicted' : ''}`}>
              <code className='profiles-merge__key'>{row.key}</code>
              <span className='profiles-merge__cell'>{row.baseValue || <em>-</em>}</span>
              <span className='profiles-merge__cell profiles-merge__cell--override'>{row.overrideValue || <em>-</em>}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
