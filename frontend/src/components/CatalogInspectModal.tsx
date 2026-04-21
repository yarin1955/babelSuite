import { createPortal } from 'react-dom'
import { useEffect, useMemo, useState } from 'react'
import {
  FaCircleCheck,
  FaCopy,
  FaCubes,
  FaDownload,
  FaFile,
  FaFolderOpen,
  FaRegStar,
  FaShieldHalved,
  FaStar,
  FaXmark,
} from 'react-icons/fa6'
import { useClipboardFeedback } from '../hooks/useClipboardFeedback'
import { useEscapeKey } from '../hooks/useEscapeKey'
import type { CatalogPackage, SuiteDefinition } from '../lib/api'
import { renderSourceLine, renderStarlarkLine } from '../lib/codeHighlight'
import { logoGradient } from './logoGradient'
import './CatalogInspectModal.css'

interface InspectModalProps {
  item: CatalogPackage
  suite: SuiteDefinition | null
  starred: boolean
  loading: boolean
  error: string
  favoriteBusy: boolean
  onClose: () => void
  onToggleFavorite: () => void
}

export function CatalogInspectModal({ item, suite, starred, loading, error, favoriteBusy, onClose, onToggleFavorite }: InspectModalProps) {
  const [selected, setSelected] = useState<string>('suite.star')
  const { copiedId, copyToClipboard } = useClipboardFeedback(1600)
  const fallbackSourceFile = useMemo(() => buildRegistryPreviewFile(item, error), [item, error])
  const suiteSourceFiles = suite?.sourceFiles ?? []
  const suiteFolders = suite?.folders ?? []
  const suiteProfiles = suite?.profiles ?? []
  const sourceFileByPath = useMemo(
    () => new Map(suiteSourceFiles.map((file) => [file.path, file])),
    [suiteSourceFiles],
  )

  useEscapeKey(onClose)

  useEffect(() => {
    setSelected(suite ? 'suite.star' : fallbackSourceFile.path)
  }, [fallbackSourceFile.path, suite, item.id])

  const activeSourceFile = sourceFileByPath.get(selected)
  const visibleSourceFile = activeSourceFile ?? (!suite ? fallbackSourceFile : null)
  const visibleModules = suite?.modules ?? item.modules
  const pullCommand = suite?.pullCommand ?? item.pullCommand
  const forkCommand = suite?.forkCommand ?? item.forkCommand
  const showRichSourceUnavailable = !loading && Boolean(suite) && selected !== 'suite.star' && !activeSourceFile

  return createPortal(
    <div className='ci-backdrop' onClick={onClose}>
      <div className='ci-modal' onClick={(e) => e.stopPropagation()}>

        {/* Header */}
        <div className='ci-header'>
          <div className='ci-header__logo' style={{ background: logoGradient(item.id) }}>
            {item.title.slice(0, 2).toUpperCase()}
          </div>
          <div className='ci-header__meta'>
            <div className='ci-header__title-row'>
              <h2 className='ci-header__title'>{item.title}</h2>
              {item.status === 'Official' && (
                <span className='ci-badge ci-badge--official'><FaShieldHalved /> Official</span>
              )}
              {item.status === 'Verified' && (
                <span className='ci-badge ci-badge--verified'><FaCircleCheck /> Verified</span>
              )}
            </div>
            <p className='ci-header__origin'>
              {item.owner} · {item.repository} · <strong>{item.version}</strong>
            </p>
          </div>
          <button
            type='button'
            className={`ci-header__star${starred ? ' ci-header__star--active' : ''}`}
            aria-pressed={starred}
            aria-label={starred ? `Unstar ${item.title}` : `Star ${item.title}`}
            title={starred ? 'Unstar' : 'Star'}
            disabled={favoriteBusy}
            onClick={onToggleFavorite}
          >
            {starred ? <FaStar /> : <FaRegStar />}
          </button>
          <button type='button' className='ci-close' onClick={onClose} aria-label='Close'><FaXmark /></button>
        </div>

        {/* Body */}
        <div className='ci-body'>
          {/* Left tree */}
          <aside className='ci-tree'>
            <p className='ci-tree__label'>Package Files</p>
            {loading
              ? [1, 2, 3, 4, 5].map((n) => <div key={n} className='ci-tree__skeleton' />)
              : (
                  <>
                    {suite ? (
                      <button
                        type='button'
                        className={`ci-tree__item${selected === 'suite.star' ? ' ci-tree__item--active' : ''}`}
                        onClick={() => setSelected('suite.star')}
                      >
                        <span className='ci-tree__item-icon'><FaFile /></span>
                        <span>suite.star</span>
                      </button>
                    ) : (
                      <button
                        type='button'
                        className={`ci-tree__item${selected === fallbackSourceFile.path ? ' ci-tree__item--active' : ''}`}
                        onClick={() => setSelected(fallbackSourceFile.path)}
                      >
                        <span className='ci-tree__item-icon'><FaFile /></span>
                        <span>{fallbackSourceFile.path}</span>
                      </button>
                    )}
                    {suiteFolders.map((folder) => (
                      <div key={folder.name} className='ci-tree__group'>
                        <div className='ci-tree__folder'>
                          <span className='ci-tree__item-icon'><FaFolderOpen /></span>
                          <span>{folder.name}/</span>
                        </div>
                        {folder.files.map((filePath) => {
                          const sourcePath = `${folder.name}/${filePath}`
                          const file = sourceFileByPath.get(sourcePath)
                          return (
                            <button
                              key={sourcePath}
                              type='button'
                              className={`ci-tree__item ci-tree__item--child${selected === sourcePath ? ' ci-tree__item--active' : ''}`}
                              onClick={() => setSelected(sourcePath)}
                              disabled={!file}
                            >
                              <span className='ci-tree__item-icon'><FaFile /></span>
                              <span>{filePath}</span>
                            </button>
                          )
                        })}
                      </div>
                    ))}
                  </>
                )}
            {suiteProfiles.length > 0 && (
              <>
                <p className='ci-tree__label ci-tree__label--profiles'>Profiles</p>
                {suiteProfiles.map((p) => (
                  <div key={p.fileName} className='ci-tree__profile'>
                    <span>{p.label}</span>
                    {p.default && <span className='ci-tree__default'>default</span>}
                  </div>
                ))}
              </>
            )}
          </aside>

          {/* Right content */}
          <div className='ci-content'>
            {loading && (
              <div className='ci-content__loading'>
                <div className='ci-spinner' />
                <p>Loading package…</p>
              </div>
            )}

            {!loading && !error && suite && selected === 'suite.star' && (
              <>
                <div className='ci-content__toolbar'>
                  <span className='ci-content__filename'><FaFile /> suite.star</span>
                  <button
                    type='button'
                    className={`ci-copy-btn${copiedId === 'star' ? ' ci-copy-btn--ok' : ''}`}
                    onClick={() => void copyToClipboard('star', suite.suiteStar)}
                  >
                    <FaCopy />
                    <span>{copiedId === 'star' ? 'Copied!' : 'Copy'}</span>
                  </button>
                </div>
                <div className='ci-code'>
                  {suite.suiteStar.split('\n').map((line, i) => (
                    <div key={i} className='ci-code__line'>
                      <span className='ci-code__num'>{String(i + 1).padStart(3, ' ')}</span>
                      <code className='ci-code__text'>{renderStarlarkLine(line)}</code>
                    </div>
                  ))}
                </div>
              </>
            )}

            {!loading && visibleSourceFile && selected !== 'suite.star' && (
              <>
                <div className='ci-content__toolbar'>
                  <span className='ci-content__filename'><FaFile /> {visibleSourceFile.path}</span>
                  <span className='ci-folder-role'>{visibleSourceFile.language}</span>
                  <button
                    type='button'
                    className={`ci-copy-btn${copiedId === visibleSourceFile.path ? ' ci-copy-btn--ok' : ''}`}
                    onClick={() => void copyToClipboard(visibleSourceFile.path, visibleSourceFile.content)}
                  >
                    <FaCopy />
                    <span>{copiedId === visibleSourceFile.path ? 'Copied!' : 'Copy'}</span>
                  </button>
                </div>
                {error && (
                  <div className='ci-content__notice'>
                    {error}
                  </div>
                )}
                <div className='ci-code'>
                  {visibleSourceFile.content.split('\n').map((line, i) => (
                    <div key={`${visibleSourceFile.path}-${i + 1}`} className='ci-code__line'>
                      <span className='ci-code__num'>{String(i + 1).padStart(3, ' ')}</span>
                      <code className='ci-code__text'>{renderSourceLine(line, visibleSourceFile.language)}</code>
                    </div>
                  ))}
                </div>
              </>
            )}

            {showRichSourceUnavailable && (
              <div className='ci-content__empty'>
                Source content for this file is not available yet. The suite published the folder entry, but not the file preview.
              </div>
            )}

            {!loading && (
              <div className='ci-footer'>
                <button
                  type='button'
                  className={`ci-cmd-btn${copiedId === 'pull' ? ' ci-cmd-btn--ok' : ''}`}
                  onClick={() => void copyToClipboard('pull', pullCommand)}
                >
                  <FaCopy />
                  <span>{copiedId === 'pull' ? 'Copied!' : 'Copy pull command'}</span>
                </button>
                <button
                  type='button'
                  className={`ci-cmd-btn ci-cmd-btn--ghost${copiedId === 'fork' ? ' ci-cmd-btn--ok' : ''}`}
                  onClick={() => void copyToClipboard('fork', forkCommand)}
                >
                  <FaDownload />
                  <span>{copiedId === 'fork' ? 'Copied!' : 'Fork'}</span>
                </button>
                {visibleModules.length > 0 && (
                  <div className='ci-modules'>
                    {visibleModules.map((m) => (
                      <span key={m} className='ci-module-pill'>
                        <FaCubes />{m.replace('@babelsuite/', '')}
                      </span>
                    ))}
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>,
    document.body,
  )
}

function buildRegistryPreviewFile(item: CatalogPackage, error: string): { path: string; language: string; content: string } {
  const modules = item.modules.length > 0 ? item.modules.map((module) => `  - ${module}`) : ['  - none published']
  const tags = item.tags.length > 0 ? item.tags.map((tag) => `  - ${tag}`) : ['  - latest']
  const lines = [
    '# BabelSuite registry preview',
    `title: ${item.title}`,
    `repository: ${item.repository}`,
    `provider: ${item.provider}`,
    `version: ${item.version}`,
    `status: ${item.status}`,
    'modules:',
    ...modules,
    'publishedTags:',
    ...tags,
    'commands:',
    `  pull: ${item.pullCommand}`,
    `  fork: ${item.forkCommand}`,
    'description: |',
    ...item.description.split('\n').map((line) => `  ${line}`),
  ]

  if (error.trim()) {
    lines.push(
      'note: |',
      `  ${error}`,
      '  Rich suite files are not available from the backend for this registry package yet.',
    )
  }

  return {
    path: 'registry-preview.yaml',
    language: 'yaml',
    content: lines.join('\n') + '\n',
  }
}
