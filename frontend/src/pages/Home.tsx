import { startTransition, useEffect, useMemo, useState } from 'react'
import { createPortal } from 'react-dom'
import {
  FaChevronDown,
  FaCircleCheck,
  FaCircleXmark,
  FaCopy,
  FaCubes,
  FaDiagramProject,
  FaDocker,
  FaDownload,
  FaFile,
  FaFolderOpen,
  FaPlay,
  FaShieldHalved,
  FaSpinner,
  FaTerminal,
  FaXmark,
} from 'react-icons/fa6'
import { useNavigate } from 'react-router-dom'
import AppShell from '../components/AppShell'
import '../components/CatalogInspectModal.css'
import { logoGradient } from '../components/logoGradient'
import { useClipboardFeedback } from '../hooks/useClipboardFeedback'
import { useEscapeKey } from '../hooks/useEscapeKey'
import {
  createExecution,
  getExecutionOverview,
  getSession,
  getSuite,
  listExecutionLaunchSuites,
  type ExecutionLaunchSuite,
  type ExecutionOverview,
  type ExecutionOverviewItem,
  type SuiteDefinition,
  type SuiteSourceFile,
} from '../lib/api'
import { renderSourceLine, renderStarlarkLine } from '../lib/codeHighlight'
import './Home.css'

export default function Home() {
  const navigate = useNavigate()
  const session = getSession()
  const [overview, setOverview] = useState<ExecutionOverview | null>(null)
  const [launchSuites, setLaunchSuites] = useState<ExecutionLaunchSuite[]>([])
  const [showModal, setShowModal] = useState(false)
  const [suiteModal, setSuiteModal] = useState<string | null>(null)
  const [suiteId, setSuiteId] = useState('')
  const [profile, setProfile] = useState('')
  const [backend, setBackend] = useState('auto')
  const [error, setError] = useState('')
  const [creating, setCreating] = useState(false)

  const selectedSuite = useMemo(
    () => launchSuites.find((item) => item.id === suiteId) ?? launchSuites[0] ?? null,
    [launchSuites, suiteId],
  )

  useEffect(() => {
    let active = true

    const load = async () => {
      try {
        const [engineOverview, suites] = await Promise.all([
          getExecutionOverview(),
          listExecutionLaunchSuites(),
        ])

        if (!active) {
          return
        }

        setOverview(engineOverview)
        setLaunchSuites(suites)

        const firstSuite = suites[0]
        if (!firstSuite) {
          return
        }

        const defaultSuiteId = suites.some((item) => item.id === suiteId) ? suiteId : firstSuite.id
        const defaultSuite = suites.find((item) => item.id === defaultSuiteId) ?? firstSuite
        const defaultProfile = defaultSuite.profiles.find((item) => item.default)?.fileName ?? defaultSuite.profiles[0]?.fileName ?? ''

        setSuiteId(defaultSuiteId)
        setProfile((current) => (
          defaultSuite.profiles.some((item) => item.fileName === current)
            ? current
            : defaultProfile
        ))
        setBackend((current) => (
          current === 'auto' || defaultSuite.backends.some((item) => item.id === current)
            ? current
            : 'auto'
        ))
      } catch (loadError) {
        if (!active) {
          return
        }
        setError(loadError instanceof Error ? loadError.message : 'Could not load executions.')
      }
    }

    void load()

    return () => {
      active = false
    }
  }, [])

  useEffect(() => {
    const timer = window.setInterval(() => {
      void getExecutionOverview()
        .then((nextOverview) => setOverview(nextOverview))
        .catch(() => {
          // Keep the last known UI state if the polling request fails.
        })
    }, 2000)

    return () => window.clearInterval(timer)
  }, [])

  useEffect(() => {
    if (!selectedSuite) {
      return
    }

    if (selectedSuite.profiles.some((item) => item.fileName === profile)) {
      return
    }

    setProfile(selectedSuite.profiles.find((item) => item.default)?.fileName ?? selectedSuite.profiles[0]?.fileName ?? '')
  }, [profile, selectedSuite])

  if (!session) {
    return null
  }

  const runs = overview?.executions ?? []
  const triggerRun = async () => {
    if (!selectedSuite || !profile) {
      return
    }

    setCreating(true)
    setError('')

    try {
      const execution = await createExecution({
        suiteId: selectedSuite.id,
        profile,
        backend,
      })

      startTransition(() => {
        setOverview((current) => {
          if (!current) {
            return current
          }

          const nextExecution: ExecutionOverviewItem = {
            id: execution.id,
            suiteId: execution.suiteId,
            suiteTitle: execution.suiteTitle,
            profile: execution.profile,
            backendId: execution.backendId,
            backend: execution.backend,
            trigger: execution.trigger,
            status: execution.status,
            duration: execution.duration,
            startedAt: execution.startedAt,
            updatedAt: execution.startedAt,
            totalSteps: 0,
            runningSteps: 0,
            healthySteps: 0,
            failedSteps: 0,
            skippedSteps: 0,
            pendingSteps: 0,
            progressRatio: 0,
            steps: [],
          }

          const executions = [nextExecution, ...current.executions.filter((item) => item.id !== execution.id)]
          return {
            ...current,
            executions,
            summary: {
              ...current.summary,
              totalExecutions: executions.length,
              bootingExecutions: current.summary.bootingExecutions + 1,
            },
          }
        })
        setShowModal(false)
      })

      navigate(`/executions/${execution.id}`)
    } catch (createError) {
      setError(createError instanceof Error ? createError.message : 'Could not create execution.')
    } finally {
      setCreating(false)
    }
  }

  return (
    <AppShell
      section='Home'
      title='Executions & Runs'
      description='Launch suites quickly, inspect recent runs, and move from failures to logs without thinking about the underlying OCI layout.'
      actions={(
        <>
          <button type='button' className='runs-button runs-button--secondary' onClick={() => navigate('/environments')}>
            <FaDocker />
            <span>Environments</span>
          </button>
          <button type='button' className='runs-button runs-button--primary' onClick={() => setShowModal(true)} disabled={launchSuites.length === 0}>
            <FaPlay />
            <span>New Execution</span>
          </button>
        </>
      )}
    >
      <div className='runs-page'>
        <section className='runs-workbench'>
          <div className='runs-panel'>
            <div className='runs-panel__header'>
              <div>
                <p className='runs-panel__eyebrow'>Activity Feed</p>
                <h2>Recent executions</h2>
              </div>
            </div>

            <div className='runs-table'>
              <div className='runs-table__head'>
                <span>Status</span>
                <span>Suite</span>
                <span>Profile</span>
                <span>Trigger</span>
                <span>Duration</span>
                <span>Actions</span>
              </div>

              {runs.map((run) => (
                <div key={run.id} className='runs-table__row'>
                  <div className='runs-table__status'>
                    <RunStatus status={run.status} />
                    <small>{formatRelativeTime(run.startedAt)}</small>
                  </div>
                  <strong>{run.suiteTitle}</strong>
                  <span>{run.profile}</span>
                  <span>{run.trigger}</span>
                  <span>{run.duration} <small>{formatProgress(run)}</small></span>
                  <div className='runs-table__actions'>
                    <button
                      type='button'
                      className='runs-icon-button'
                      title='Open live execution'
                      onClick={() => navigate(`/executions/${run.id}`)}
                    >
                      <FaTerminal />
                    </button>
                    <button
                      type='button'
                      className='runs-icon-button'
                      title='Inspect suite'
                      onClick={() => setSuiteModal(run.suiteId)}
                    >
                      <FaDiagramProject />
                    </button>
                  </div>
                </div>
              ))}

              {runs.length === 0 && (
                <div className='runs-notice'>
                  No executions yet. Start a new execution to populate the live activity feed.
                </div>
              )}
            </div>
          </div>
        </section>
      </div>

      {suiteModal && (
        <SuiteInspectModal
          suiteId={suiteModal}
          onClose={() => setSuiteModal(null)}
        />
      )}

      {showModal && selectedSuite && (
        <ExecutionModal
          launchSuites={launchSuites}
          selectedSuite={selectedSuite}
          profile={profile}
          error={error}
          creating={creating}
          onChangeSuite={(id) => {
            const next = launchSuites.find((item) => item.id === id)
            setSuiteId(id)
            setProfile(next?.profiles.find((item) => item.default)?.fileName ?? next?.profiles[0]?.fileName ?? '')
            setBackend('auto')
          }}
          onChangeProfile={setProfile}
          backend={backend}
          onChangeBackend={setBackend}
          onClose={() => setShowModal(false)}
          onExecute={triggerRun}
        />
      )}
    </AppShell>
  )
}

interface ExecutionModalProps {
  launchSuites: ExecutionLaunchSuite[]
  selectedSuite: ExecutionLaunchSuite
  profile: string
  backend: string
  error: string
  creating: boolean
  onChangeSuite: (id: string) => void
  onChangeProfile: (fileName: string) => void
  onChangeBackend: (backendId: string) => void
  onClose: () => void
  onExecute: () => void
}

function ExecutionModal({
  launchSuites,
  selectedSuite,
  profile,
  backend,
  error,
  creating,
  onChangeSuite,
  onChangeProfile,
  onChangeBackend,
  onClose,
  onExecute,
}: ExecutionModalProps) {
  useEscapeKey(onClose)

  return createPortal(
    <div className='ex-modal' role='presentation'>
      <div className='ex-modal__backdrop' onClick={onClose} />
      <div className='ex-modal__dialog' role='dialog' aria-modal='true' aria-label='New execution'>
        <header className='ex-modal__header'>
          <div>
            <p className='ex-modal__eyebrow'>New Execution</p>
            <h2 className='ex-modal__title'>Boot an environment</h2>
          </div>
          <button type='button' className='ex-modal__close' onClick={onClose} aria-label='Close'>
            <FaCircleXmark />
          </button>
        </header>

        <div className='ex-modal__body'>
          <div className='ex-section'>
            <p className='ex-label'>Suite</p>
            {launchSuites.length === 1 ? (
              <div className='ex-suite-card'>
                <div className='ex-suite-card__info'>
                  <strong>{selectedSuite.title}</strong>
                  <span>{selectedSuite.repository}</span>
                </div>
              </div>
            ) : (
              <label className='ex-select-wrap'>
                <select
                  value={selectedSuite.id}
                  onChange={(e) => onChangeSuite(e.target.value)}
                  className='ex-select'
                >
                  {launchSuites.map((item) => (
                    <option key={item.id} value={item.id}>{item.title}</option>
                  ))}
                </select>
                <FaChevronDown className='ex-select-wrap__arrow' />
              </label>
            )}
            {selectedSuite.description && (
              <p className='ex-suite-desc'>{selectedSuite.description}</p>
            )}
          </div>

          <div className='ex-section'>
            <p className='ex-label'>Profile</p>
            <div className='ex-profiles'>
              {selectedSuite.profiles.map((item) => (
                <button
                  key={item.fileName}
                  type='button'
                  className={`ex-profile-card${item.fileName === profile ? ' ex-profile-card--active' : ''}`}
                  onClick={() => onChangeProfile(item.fileName)}
                >
                  <strong>{item.label || item.fileName}</strong>
                  {item.description && <span>{item.description}</span>}
                  <code>{item.fileName}</code>
                </button>
              ))}
            </div>
          </div>

          <div className='ex-section'>
            <p className='ex-label'>Backend</p>
            <div className='ex-profiles'>
              <button
                type='button'
                className={`ex-profile-card${backend === 'auto' ? ' ex-profile-card--active' : ''}`}
                onClick={() => onChangeBackend('auto')}
              >
                <strong>Auto</strong>
                <span>Pick the default available backend when the run starts.</span>
                <code>auto</code>
              </button>
              {selectedSuite.backends.map((item) => (
                <button
                  key={item.id}
                  type='button'
                  className={`ex-profile-card${item.id === backend ? ' ex-profile-card--active' : ''}`}
                  onClick={() => onChangeBackend(item.id)}
                  disabled={!item.available}
                >
                  <strong>{item.label}</strong>
                  {(item.description || item.kind) && <span>{item.description || item.kind}</span>}
                  <code>{item.available ? item.kind : 'unavailable'}</code>
                </button>
              ))}
            </div>
          </div>

          {error && <div className='ex-error'>{error}</div>}
        </div>

        <footer className='ex-modal__footer'>
          <button type='button' className='ex-btn ex-btn--ghost' onClick={onClose}>
            Cancel
          </button>
          <button type='button' className='ex-btn ex-btn--primary' onClick={onExecute} disabled={creating || !profile}>
            <FaPlay />
            <span>{creating ? 'Executing...' : 'Execute'}</span>
          </button>
        </footer>
      </div>
    </div>,
    document.body,
  )
}

function RunStatus({ status }: { status: ExecutionOverviewItem['status'] }) {
  if (status === 'Healthy') {
    return (
      <span className='runs-status runs-status--healthy'>
        <FaCircleCheck />
        <span>Healthy</span>
      </span>
    )
  }

  if (status === 'Failed') {
    return (
      <span className='runs-status runs-status--failed'>
        <FaCircleXmark />
        <span>Failed</span>
      </span>
    )
  }

  return (
    <span className='runs-status runs-status--booting'>
      <FaSpinner />
      <span>Booting</span>
    </span>
  )
}

function formatRelativeTime(value: string) {
  const startedAt = new Date(value)
  const seconds = Math.max(0, Math.round((Date.now() - startedAt.getTime()) / 1000))

  if (seconds < 10) {
    return 'Just now'
  }
  if (seconds < 60) {
    return `${seconds}s ago`
  }

  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) {
    return `${minutes}m ago`
  }

  const hours = Math.floor(minutes / 60)
  if (hours < 24) {
    return `${hours}h ago`
  }

  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

function formatProgress(run: ExecutionOverviewItem) {
  if (run.totalSteps === 0) {
    return 'planning'
  }
  return `${run.healthySteps + run.runningSteps + run.failedSteps + (run.skippedSteps ?? 0)}/${run.totalSteps} steps`
}

/* ── Suite inspect modal ──────────────────────────────── */

function SuiteInspectModal({ suiteId, onClose }: { suiteId: string; onClose: () => void }) {
  const [suite, setSuite] = useState<SuiteDefinition | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [selected, setSelected] = useState('suite.star')
  const { copiedId, copyToClipboard } = useClipboardFeedback(1600)

  useEscapeKey(onClose)

  useEffect(() => {
    let active = true
    setLoading(true)
    setError('')
    void getSuite(suiteId)
      .then((s) => { if (active) { setSuite(s); setLoading(false); setSelected('suite.star') } })
      .catch((e) => { if (active) { setError(e instanceof Error ? e.message : 'Could not load suite.'); setLoading(false) } })
    return () => { active = false }
  }, [suiteId])

  const suiteSourceFiles: SuiteSourceFile[] = suite?.sourceFiles ?? []
  const suiteFolders = suite?.folders ?? []
  const rootSourceFiles = useMemo(
    () => suiteSourceFiles.filter((file) => !file.path.includes('/')),
    [suiteSourceFiles],
  )
  const suiteProfiles = suite?.profiles ?? []
  const sourceFileByPath = useMemo(
    () => new Map(suiteSourceFiles.map((f) => [f.path, f])),
    [suiteSourceFiles],
  )

  const activeSourceFile = sourceFileByPath.get(selected)
  const showRichSourceUnavailable = !loading && Boolean(suite) && selected !== 'suite.star' && !activeSourceFile

  return createPortal(
    <div className='ci-backdrop' onClick={onClose}>
      <div className='ci-modal' onClick={(e) => e.stopPropagation()}>

        <div className='ci-header'>
          <div className='ci-header__logo' style={{ background: loading ? '#1f3a52' : logoGradient(suiteId) }}>
            {loading ? '…' : (suite?.title ?? suiteId).slice(0, 2).toUpperCase()}
          </div>
          <div className='ci-header__meta'>
            <div className='ci-header__title-row'>
              <h2 className='ci-header__title'>{loading ? 'Loading…' : (suite?.title ?? suiteId)}</h2>
              {suite?.status === 'Official' && (
                <span className='ci-badge ci-badge--official'><FaShieldHalved /> Official</span>
              )}
              {suite?.status === 'Verified' && (
                <span className='ci-badge ci-badge--verified'><FaCircleCheck /> Verified</span>
              )}
            </div>
            {suite && (
              <p className='ci-header__origin'>
                {suite.owner} · {suite.repository} · <strong>{suite.version}</strong>
              </p>
            )}
          </div>
          <button type='button' className='ci-close' onClick={onClose} aria-label='Close'><FaXmark /></button>
        </div>

        <div className='ci-body'>
          <aside className='ci-tree'>
            <p className='ci-tree__label'>Package Files</p>
            {loading
              ? [1, 2, 3, 4, 5].map((n) => <div key={n} className='ci-tree__skeleton' />)
              : (
                  <>
                    {suite && (
                      <button
                        type='button'
                        className={`ci-tree__item${selected === 'suite.star' ? ' ci-tree__item--active' : ''}`}
                        onClick={() => setSelected('suite.star')}
                      >
                        <span className='ci-tree__item-icon'><FaFile /></span>
                        <span>suite.star</span>
                      </button>
                    )}
                    {rootSourceFiles.map((file) => (
                      <button
                        key={file.path}
                        type='button'
                        className={`ci-tree__item${selected === file.path ? ' ci-tree__item--active' : ''}`}
                        onClick={() => setSelected(file.path)}
                      >
                        <span className='ci-tree__item-icon'><FaFile /></span>
                        <span>{file.path}</span>
                      </button>
                    ))}
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

          <div className='ci-content'>
            {loading && (
              <div className='ci-content__loading'>
                <div className='ci-spinner' />
                <p>Loading suite…</p>
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

            {!loading && activeSourceFile && selected !== 'suite.star' && (
              <>
                <div className='ci-content__toolbar'>
                  <span className='ci-content__filename'><FaFile /> {activeSourceFile.path}</span>
                  <span className='ci-folder-role'>{activeSourceFile.language}</span>
                  <button
                    type='button'
                    className={`ci-copy-btn${copiedId === activeSourceFile.path ? ' ci-copy-btn--ok' : ''}`}
                    onClick={() => void copyToClipboard(activeSourceFile.path, activeSourceFile.content)}
                  >
                    <FaCopy />
                    <span>{copiedId === activeSourceFile.path ? 'Copied!' : 'Copy'}</span>
                  </button>
                </div>
                <div className='ci-code'>
                  {activeSourceFile.content.split('\n').map((line, i) => (
                    <div key={`${activeSourceFile.path}-${i + 1}`} className='ci-code__line'>
                      <span className='ci-code__num'>{String(i + 1).padStart(3, ' ')}</span>
                      <code className='ci-code__text'>{renderSourceLine(line, activeSourceFile.language)}</code>
                    </div>
                  ))}
                </div>
              </>
            )}

            {showRichSourceUnavailable && (
              <div className='ci-content__empty'>
                Source content for this file is not available yet.
              </div>
            )}

            {error && <div className='ci-content__error'>{error}</div>}

            {!loading && suite && (
              <div className='ci-footer'>
                <button
                  type='button'
                  className={`ci-cmd-btn${copiedId === 'pull' ? ' ci-cmd-btn--ok' : ''}`}
                  onClick={() => void copyToClipboard('pull', suite.pullCommand)}
                >
                  <FaCopy />
                  <span>{copiedId === 'pull' ? 'Copied!' : 'Copy pull command'}</span>
                </button>
                <button
                  type='button'
                  className={`ci-cmd-btn ci-cmd-btn--ghost${copiedId === 'fork' ? ' ci-cmd-btn--ok' : ''}`}
                  onClick={() => void copyToClipboard('fork', suite.forkCommand)}
                >
                  <FaDownload />
                  <span>{copiedId === 'fork' ? 'Copied!' : 'Fork'}</span>
                </button>
                {suite.modules.length > 0 && (
                  <div className='ci-modules'>
                    {suite.modules.map((m) => (
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
