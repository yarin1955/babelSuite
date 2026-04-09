import type { ReactNode } from 'react'
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
import type { CatalogPackage, SuiteDefinition } from '../lib/api'
import { logoGradient } from './logoGradient'

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
  const [copyId, setCopyId] = useState('')
  const fallbackSourceFile = useMemo(() => buildRegistryPreviewFile(item, error), [item, error])
  const suiteSourceFiles = suite?.sourceFiles ?? []
  const suiteFolders = suite?.folders ?? []
  const suiteProfiles = suite?.profiles ?? []
  const sourceFileByPath = useMemo(
    () => new Map(suiteSourceFiles.map((file) => [file.path, file])),
    [suiteSourceFiles],
  )

  const copy = async (id: string, value: string) => {
    await navigator.clipboard.writeText(value)
    setCopyId(id)
    window.setTimeout(() => setCopyId(''), 1600)
  }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

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
                <p className='ci-tree__label' style={{ marginTop: 16 }}>Profiles</p>
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
                    className={`ci-copy-btn${copyId === 'star' ? ' ci-copy-btn--ok' : ''}`}
                    onClick={() => void copy('star', suite.suiteStar)}
                  >
                    <FaCopy />
                    <span>{copyId === 'star' ? 'Copied!' : 'Copy'}</span>
                  </button>
                </div>
                <div className='ci-code'>
                  {suite.suiteStar.split('\n').map((line, i) => (
                    <div key={i} className='ci-code__line'>
                      <span className='ci-code__num'>{String(i + 1).padStart(3, ' ')}</span>
                      <code className='ci-code__text'>{renderStarLine(line)}</code>
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
                    className={`ci-copy-btn${copyId === visibleSourceFile.path ? ' ci-copy-btn--ok' : ''}`}
                    onClick={() => void copy(visibleSourceFile.path, visibleSourceFile.content)}
                  >
                    <FaCopy />
                    <span>{copyId === visibleSourceFile.path ? 'Copied!' : 'Copy'}</span>
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
                  className={`ci-cmd-btn${copyId === 'pull' ? ' ci-cmd-btn--ok' : ''}`}
                  onClick={() => void copy('pull', pullCommand)}
                >
                  <FaCopy />
                  <span>{copyId === 'pull' ? 'Copied!' : 'Copy pull command'}</span>
                </button>
                <button
                  type='button'
                  className={`ci-cmd-btn ci-cmd-btn--ghost${copyId === 'fork' ? ' ci-cmd-btn--ok' : ''}`}
                  onClick={() => void copy('fork', forkCommand)}
                >
                  <FaDownload />
                  <span>{copyId === 'fork' ? 'Copied!' : 'Fork'}</span>
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

function renderStarLine(line: string): ReactNode[] {
  const ci = line.indexOf('#')
  const code = ci >= 0 ? line.slice(0, ci) : line
  const comment = ci >= 0 ? line.slice(ci) : ''
  const out: ReactNode[] = []
  const pat = /"[^"]*"|\b(load|service|mock|task|test|traffic|suite)\b|@[a-zA-Z0-9/_-]+/g
  let cur = 0
  for (const m of code.matchAll(pat)) {
    const v = m[0]
    const s = m.index ?? 0
    if (s > cur) out.push(code.slice(cur, s))
    const cls = v.startsWith('"') ? 'ci-tok ci-tok--str' : v.startsWith('@') ? 'ci-tok ci-tok--mod' : 'ci-tok ci-tok--kw'
    out.push(<span key={`${s}-${v}`} className={cls}>{v}</span>)
    cur = s + v.length
  }
  if (cur < code.length) out.push(code.slice(cur))
  if (comment) out.push(<span key='cmt' className='ci-tok ci-tok--cmt'>{comment}</span>)
  return out
}

function renderSourceLine(line: string, language: string): ReactNode[] {
  const trimmedLanguage = language.trim().toLowerCase()
  if (trimmedLanguage === 'yaml' || trimmedLanguage === 'python' || trimmedLanguage === 'bash' || trimmedLanguage === 'rego') {
    const commentIndex = line.indexOf('#')
    const code = commentIndex >= 0 ? line.slice(0, commentIndex) : line
    const comment = commentIndex >= 0 ? line.slice(commentIndex) : ''
    const fragments = highlightCodeTokens(code)
    if (comment) {
      fragments.push(<span key={`comment-${line}`} className='ci-tok ci-tok--cmt'>{comment}</span>)
    }
    return fragments
  }
  return highlightCodeTokens(line)
}

function highlightCodeTokens(line: string): ReactNode[] {
  const fragments: ReactNode[] = []
  const pattern = /"[^"]*"|'[^']*'|\b(message|service|rpc|package|import|const|let|type|interface|export|default|allow|if|true|false|null)\b|@[a-zA-Z0-9/_-]+/g
  let cursor = 0

  for (const match of line.matchAll(pattern)) {
    const value = match[0]
    const start = match.index ?? 0
    if (start > cursor) {
      fragments.push(line.slice(cursor, start))
    }
    const className = value.startsWith('"') || value.startsWith("'")
      ? 'ci-tok ci-tok--str'
      : value.startsWith('@')
        ? 'ci-tok ci-tok--mod'
        : 'ci-tok ci-tok--kw'
    fragments.push(<span key={`${start}-${value}`} className={className}>{value}</span>)
    cursor = start + value.length
  }

  if (cursor < line.length) {
    fragments.push(line.slice(cursor))
  }
  return fragments
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
