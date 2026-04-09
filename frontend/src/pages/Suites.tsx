import type { ReactNode } from 'react'
import { useEffect, useMemo, useState } from 'react'
import {
  FaBookOpen,
  FaCircleNodes,
  FaCopy,
  FaDownload,
  FaFolderTree,
  FaPlay,
  FaRegStar,
  FaRoute,
  FaServer,
  FaShieldHalved,
  FaCircleCheck,
  FaTerminal,
} from 'react-icons/fa6'
import { useNavigate, useParams } from 'react-router-dom'
import AppShell from '../components/AppShell'
import { HeaderList } from '../components/HeaderList'
import { logoGradient } from '../components/logoGradient'
import { createExecution, getSuite, listExecutionLaunchSuites, type ExecutionLaunchSuite, type SuiteDefinition } from '../lib/api'
import { groupTopologyByLevel, parseSuiteTopology } from '../lib/suites'
import './Suites.css'

export default function Suites() {
  const navigate = useNavigate()
  const params = useParams()
  const suiteId = params.suiteId ?? ''
  const [suite, setSuite] = useState<SuiteDefinition | null>(null)
  const [selectedFolder, setSelectedFolder] = useState('')
  const [selectedSourcePath, setSelectedSourcePath] = useState('')
  const [selectedOperationId, setSelectedOperationId] = useState('')
  const [selectedExchangeName, setSelectedExchangeName] = useState('')
  const [selectedProfile, setSelectedProfile] = useState('')
  const [selectedBackend, setSelectedBackend] = useState('auto')
  const [launchSuite, setLaunchSuite] = useState<ExecutionLaunchSuite | null>(null)
  const [notice, setNotice] = useState('')
  const [loadError, setLoadError] = useState('')
  const [showRunModal, setShowRunModal] = useState(false)
  const [launching, setLaunching] = useState(false)
  const [copiedId, setCopiedId] = useState('')

  useEffect(() => {
    let active = true
    setSuite(null)
    setLoadError('')

    const load = async () => {
      try {
        const [nextSuite, launchSuites] = await Promise.all([
          getSuite(suiteId),
          listExecutionLaunchSuites(),
        ])
        if (!active) return
        setSuite(nextSuite)
        setLaunchSuite(launchSuites.find((item) => item.id === suiteId) ?? null)
      } catch (error) {
        if (!active) return
        setLoadError(error instanceof Error ? error.message : 'Could not load suite.')
      }
    }

    if (suiteId) {
      void load()
    } else {
      setLoadError('Suite not found.')
    }

    return () => { active = false }
  }, [suiteId])

  const topology = useMemo(
    () => (
      suite
        ? ((suite.topology?.length ? suite.topology : parseSuiteTopology(suite.suiteStar)) as ReturnType<typeof parseSuiteTopology>)
        : []
    ),
    [suite],
  )
  const topologyLevels = useMemo(() => groupTopologyByLevel(topology), [topology])
  const suiteSourceFiles = suite?.sourceFiles ?? []
  const rootSourceFiles = useMemo(
    () => suiteSourceFiles.filter((file) => !file.path.includes('/')),
    [suiteSourceFiles],
  )
  const sourceFileByPath = useMemo(
    () => new Map(suiteSourceFiles.map((file) => [file.path, file])),
    [suiteSourceFiles],
  )

  useEffect(() => {
    if (!suite) return
    const defaultOperation = suite.apiSurfaces[0]?.operations[0]
    const defaultSourcePath = preferredSourcePath(suite, defaultOperation)
    const defaultFolder = folderNameFromPath(defaultSourcePath) || suite.folders[0]?.name || ''
    setSelectedFolder(defaultFolder)
    setSelectedSourcePath(defaultSourcePath)
    setSelectedOperationId(defaultOperation?.id ?? '')
    setSelectedExchangeName(defaultOperation?.exchanges[0]?.name ?? '')
    setSelectedProfile(suite.profiles.find((p) => p.default)?.fileName ?? suite.profiles[0]?.fileName ?? '')
    setSelectedBackend('auto')
    setNotice('')
    setShowRunModal(false)
  }, [suite])

  const copyValue = async (id: string, value: string) => {
    await navigator.clipboard.writeText(value)
    const label = id === 'pull'
      ? 'Pull command'
      : id === 'fork'
        ? 'Fork command'
        : id === 'star'
          ? 'suite.star'
          : id === 'url'
            ? 'Public endpoint'
            : id === 'resolver'
              ? 'Resolver URL'
            : id === 'curl'
              ? 'cURL command'
              : id
    setCopiedId(id)
    setNotice(`${label} copied.`)
    window.setTimeout(() => { setCopiedId(''); setNotice('') }, 1800)
  }

  const executeSuite = async () => {
    if (!selectedProfile) return
    setLaunching(true)
    try {
      const execution = await createExecution({ suiteId: suite!.id, profile: selectedProfile, backend: selectedBackend })
      navigate(`/executions/${execution.id}`)
    } catch (error) {
      setNotice(error instanceof Error ? error.message : 'Could not create execution.')
      window.setTimeout(() => setNotice(''), 2200)
    } finally {
      setLaunching(false)
      setShowRunModal(false)
    }
  }

  if (!suite) {
    return (
      <AppShell
        section='Catalog'
        title={loadError ? 'Package unavailable' : 'Loading…'}
        description=''
        actions={(
          <button type='button' className='suite-btn suite-btn--secondary' onClick={() => navigate('/catalog')}>
            <FaBookOpen />
            <span>Back to Catalog</span>
          </button>
        )}
      >
        <div className='suite-page'>
          <div className='suite-loading-card'>
            {loadError
              ? <><strong>Package unavailable</strong><p>{loadError}</p></>
              : <><strong>Loading suite…</strong><p>Fetching package definition from the control plane.</p></>}
          </div>
        </div>
      </AppShell>
    )
  }

  const activeFolder = suite.folders.find((f) => f.name === selectedFolder) ?? suite.folders[0]
  const selectedSurface = suite.apiSurfaces.find((s) => s.operations.some((o) => o.id === selectedOperationId)) ?? suite.apiSurfaces[0]
  const selectedOperation = selectedSurface?.operations.find((o) => o.id === selectedOperationId) ?? selectedSurface?.operations[0]
  const selectedExchange = selectedOperation?.exchanges.find((e) => e.name === selectedExchangeName) ?? selectedOperation?.exchanges[0]
  const selectedPublicUrl = selectedOperation?.mockUrl || ''
  const selectedResolverUrl = selectedOperation?.mockMetadata?.resolverUrl || ''
  const selectedRuntimeUrl = selectedOperation?.mockMetadata?.runtimeUrl || ''
  const activeSourceFile = selectedSourcePath ? sourceFileByPath.get(selectedSourcePath) : suiteSourceFiles[0]
  const selectSourceFile = (path: string) => {
    setSelectedSourcePath(path)
    const folderName = folderNameFromPath(path)
    if (folderName) setSelectedFolder(folderName)
  }

  return (
    <AppShell
      section='Catalog'
      title={suite.title}
      description=''
      actions={(
        <>
          <button type='button' className='suite-btn suite-btn--secondary' onClick={() => navigate('/catalog')}>
            <FaBookOpen />
            <span>Back to Catalog</span>
          </button>
          <button type='button' className='suite-btn' onClick={() => setShowRunModal(true)}>
            <FaPlay />
            <span>Run Suite</span>
          </button>
        </>
      )}
    >
      {/* ── Jumbotron ── */}
      <div className='suite-jumbotron'>
        <div className='suite-jumbotron__inner'>
          <div className='suite-jumbotron__logo' style={{ background: logoGradient(suite.id) }}>
            {suite.title.slice(0, 2).toUpperCase()}
          </div>

          <div className='suite-jumbotron__body'>
            <div className='suite-jumbotron__title-row'>
              <h2 className='suite-jumbotron__title'>{suite.title}</h2>
              {suite.status === 'Official' && (
                <span className='suite-badge suite-badge--official'><FaShieldHalved /> Official</span>
              )}
              {suite.status === 'Verified' && (
                <span className='suite-badge suite-badge--verified'><FaCircleCheck /> Verified</span>
              )}
            </div>
            <p className='suite-jumbotron__origin'>
              <span>{suite.owner}</span>
              <span className='suite-jumbotron__dot'>·</span>
              <span>{suite.repository}</span>
              <span className='suite-jumbotron__dot'>·</span>
              <strong>{suite.version}</strong>
            </p>
            <p className='suite-jumbotron__desc'>{suite.description ?? ''}</p>
          </div>

          <div className='suite-jumbotron__stats'>
            <div className='suite-jumbotron__stat'>
              <FaRegStar />
              <span>{suite.score.toFixed(1)}</span>
              <small>Score</small>
            </div>
            <div className='suite-jumbotron__stat'>
              <FaFolderTree />
              <span>{topology.length}</span>
              <small>Nodes</small>
            </div>
            <div className='suite-jumbotron__stat'>
              <FaRoute />
              <span>{suite.apiSurfaces.reduce((n, s) => n + s.operations.reduce((m, o) => m + o.exchanges.length, 0), 0)}</span>
              <small>Exchanges</small>
            </div>
            <div className='suite-jumbotron__stat'>
              <FaServer />
              <span>{suite.profiles.length}</span>
              <small>Profiles</small>
            </div>
          </div>
        </div>
      </div>

      {/* ── Body: main + sidebar ── */}
      <div className='suite-body'>
        {/* ── Main content ── */}
        <div className='suite-main'>
          {notice && <div className='suite-notice'>{notice}</div>}

          {/* Starlark viewer */}
          <section className='suite-section'>
            <div className='suite-section__header'>
              <div>
                <p className='suite-eyebrow'>Starlark</p>
                <h3>suite.star</h3>
              </div>
              <button type='button' className='suite-inline-btn' onClick={() => void copyValue('star', suite.suiteStar)}>
                <FaCopy />
                <span>{copiedId === 'star' ? 'Copied!' : 'Copy'}</span>
              </button>
            </div>
            <div className='suite-code-viewer'>
              {suite.suiteStar.split('\n').map((line, i) => (
                <div key={`${i + 1}-${line}`} className='suite-code-line'>
                  <span className='suite-code-line__number'>{String(i + 1).padStart(3, ' ')}</span>
                  <code className='suite-code-line__content'>{renderHighlightedLine(line)}</code>
                </div>
              ))}
            </div>
          </section>

          <section className='suite-section'>
            <div className='suite-section__header'>
              <div>
                <p className='suite-eyebrow'>Source Preview</p>
                <h3>{activeSourceFile?.path ?? 'Select a file from Package Explorer'}</h3>
              </div>
              {activeSourceFile && (
                <div className='suite-source-preview__actions'>
                  <span className='suite-source-preview__language'>{activeSourceFile.language}</span>
                  <button
                    type='button'
                    className='suite-inline-btn'
                    onClick={() => void copyValue(activeSourceFile.path, activeSourceFile.content)}
                  >
                    <FaCopy />
                    <span>{copiedId === activeSourceFile.path ? 'Copied!' : 'Copy'}</span>
                  </button>
                </div>
              )}
            </div>
            {activeSourceFile
              ? (
                  <div className='suite-code-viewer'>
                    {activeSourceFile.content.split('\n').map((line, i) => (
                      <div key={`${activeSourceFile.path}-${i + 1}`} className='suite-code-line'>
                        <span className='suite-code-line__number'>{String(i + 1).padStart(3, ' ')}</span>
                        <code className='suite-code-line__content'>{line || ' '}</code>
                      </div>
                    ))}
                  </div>
                )
              : (
                  <div className='suite-source-preview__empty'>
                    Select a file from Package Explorer to inspect the generated contract or mock data.
                  </div>
                )}
          </section>

          {/* Topology */}
          {topologyLevels.length > 0 && (
            <section className='suite-section'>
              <div className='suite-section__header'>
                <div>
                  <p className='suite-eyebrow'>Dependency Graph</p>
                  <h3>Topology Preview</h3>
                </div>
                <FaCircleNodes className='suite-section__icon' />
              </div>
              <div className='suite-topology'>
                {topologyLevels.map((level, i) => (
                  <div key={`level-${i + 1}`} className='suite-topology__column'>
                    <span className='suite-topology__label'>Wave {i + 1}</span>
                    {level.map((node) => (
                      <div key={node.id} className={`suite-node suite-node--${node.kind}`}>
                        <div className='suite-node__header'>
                          <strong>{node.name}</strong>
                          <small>{node.kind}</small>
                        </div>
                        <div className='suite-node__deps'>
                          {node.dependsOn.length > 0
                            ? node.dependsOn.map((d) => <span key={`${node.id}-${d}`}>{d}</span>)
                            : <span>root</span>}
                        </div>
                      </div>
                    ))}
                  </div>
                ))}
              </div>
            </section>
          )}

          {/* API Contracts */}
          {selectedSurface && selectedOperation && selectedExchange && (
            <section className='suite-section'>
              <div className='suite-section__header'>
                <div>
                  <p className='suite-eyebrow'>Contracts & Mocks</p>
                  <h3>{selectedSurface.title}</h3>
                </div>
                <FaRoute className='suite-section__icon' />
              </div>

              <div className='suite-surface'>
                <aside className='suite-surface__ops'>
                  <div className='suite-surface__intro'>
                    <strong>{selectedSurface.protocol}</strong>
                    <p>{selectedSurface.description}</p>
                  </div>
                  {selectedSurface.operations.map((op) => (
                    <button
                      key={op.id}
                      type='button'
                      className={`suite-op-btn${op.id === selectedOperation.id ? ' suite-op-btn--active' : ''}`}
                      onClick={() => {
                        setSelectedOperationId(op.id)
                        setSelectedExchangeName(op.exchanges[0]?.name ?? '')
                        const previewPath = preferredSourcePath(suite, op)
                        if (previewPath) selectSourceFile(previewPath)
                      }}
                    >
                      <div className='suite-op-btn__top'>
                        <span className='suite-op-btn__method'>{op.method}</span>
                        <strong>{op.name}</strong>
                      </div>
                      <p>{op.summary}</p>
                      <small>{op.mockMetadata?.adapter?.toUpperCase?.() ?? selectedSurface.protocol} · {op.dispatcher}</small>
                    </button>
                  ))}
                </aside>

                <div className='suite-surface__detail'>
                  <div className='suite-op-detail__header'>
                    <div>
                      <p className='suite-eyebrow'>Selected Operation</p>
                      <h4>{selectedOperation.name}</h4>
                      <p>{selectedOperation.summary}</p>
                    </div>
                    <div className='suite-op-detail__badges'>
                      <span>{selectedOperation.method}</span>
                      <span>{selectedOperation.mockMetadata.adapter}</span>
                      <span>{selectedOperation.dispatcher}</span>
                      <span>{selectedSurface.mockHost}</span>
                    </div>
                  </div>

                  <div className='suite-op-meta'>
                    <div><span>Contract</span><strong>{selectedOperation.contractPath}</strong></div>
                    <div><span>Mock File</span><strong>{selectedOperation.mockPath}</strong></div>
                    {selectedOperation.mockMetadata.metadataPath && (
                      <div><span>Metadata</span><strong>{selectedOperation.mockMetadata.metadataPath}</strong></div>
                    )}
                  </div>

                  <div className='suite-exchange-tabs'>
                    {selectedOperation.exchanges.map((ex) => (
                      <button
                        key={ex.name}
                        type='button'
                        className={`suite-exchange-tab${ex.name === selectedExchange.name ? ' suite-exchange-tab--active' : ''}`}
                        onClick={() => setSelectedExchangeName(ex.name)}
                      >
                        <strong>{ex.name}</strong>
                        <small>{ex.sourceArtifact}</small>
                      </button>
                    ))}
                  </div>

                  <div className='suite-mock-url'>
                    <div>
                      <p className='suite-eyebrow'>Public Endpoint</p>
                      <strong>{selectedPublicUrl || selectedRuntimeUrl}</strong>
                      <small>Dispatch: {selectedExchange.dispatchCriteria}</small>
                      {selectedResolverUrl && (
                        <small className='suite-mock-url__declared'>Resolver: {selectedResolverUrl}</small>
                      )}
                      {selectedRuntimeUrl && selectedRuntimeUrl !== selectedPublicUrl && (
                        <small className='suite-mock-url__declared'>Compatibility engine path: {selectedRuntimeUrl}</small>
                      )}
                    </div>
                    <div className='suite-mock-url__actions'>
                      <button type='button' className='suite-inline-btn' onClick={() => void copyValue('url', selectedPublicUrl || selectedRuntimeUrl)}>
                        <FaCopy /><span>Copy URL</span>
                      </button>
                      {selectedResolverUrl && (
                        <button type='button' className='suite-inline-btn' onClick={() => void copyValue('resolver', selectedResolverUrl)}>
                          <FaCopy /><span>Copy Resolver</span>
                        </button>
                      )}
                      <button type='button' className='suite-inline-btn' onClick={() => void copyValue('curl', selectedOperation.curlCommand)}>
                        <FaTerminal /><span>Copy cURL</span>
                      </button>
                    </div>
                  </div>

                  <div className='suite-runtime-grid'>
                    <div className='suite-runtime-card'>
                      <p className='suite-eyebrow'>Dispatch Metadata</p>
                      <div className='suite-runtime-card__rows'>
                        <div><span>Adapter</span><strong>{selectedOperation.mockMetadata.adapter}</strong></div>
                        <div><span>Dispatcher</span><strong>{selectedOperation.mockMetadata.dispatcher}</strong></div>
                        {selectedOperation.mockMetadata.resolverUrl && (
                          <div><span>Resolver</span><strong>{selectedOperation.mockMetadata.resolverUrl}</strong></div>
                        )}
                        <div><span>Rules</span><strong>{selectedOperation.mockMetadata.dispatcherRules || 'Default field mapping'}</strong></div>
                        <div><span>Delay</span><strong>{selectedOperation.mockMetadata.delayMillis ? `${selectedOperation.mockMetadata.delayMillis} ms` : 'None'}</strong></div>
                      </div>
                    </div>

                    <div className='suite-runtime-card'>
                      <p className='suite-eyebrow'>Constraints & Fallback</p>
                      {selectedOperation.mockMetadata.parameterConstraints?.length
                        ? (
                            <div className='suite-runtime-list'>
                              {selectedOperation.mockMetadata.parameterConstraints.map((constraint) => (
                                <div key={`${constraint.in}-${constraint.name}`} className='suite-runtime-list__item'>
                                  <strong>{constraint.name}</strong>
                                  <span>{constraint.in}{constraint.required ? ' · required' : ''}{constraint.recopy ? ' · recopy' : ''}</span>
                                  {constraint.mustMatchRegexp && <small>{constraint.mustMatchRegexp}</small>}
                                </div>
                              ))}
                            </div>
                          )
                        : <p className='suite-runtime-empty'>No parameter constraints.</p>}

                      {selectedOperation.mockMetadata.fallback && (
                        <div className='suite-runtime-fallback'>
                          <strong>Fallback: {selectedOperation.mockMetadata.fallback.mode}</strong>
                          <small>{fallbackSummary(selectedOperation)}</small>
                        </div>
                      )}
                    </div>

                    <div className='suite-runtime-card'>
                      <p className='suite-eyebrow'>State Model</p>
                      {selectedOperation.mockMetadata.state
                        ? (
                            <div className='suite-runtime-list'>
                              {selectedOperation.mockMetadata.state.lookupKeyTemplate && (
                                <div className='suite-runtime-list__item'>
                                  <strong>Lookup Key</strong>
                                  <small>{selectedOperation.mockMetadata.state.lookupKeyTemplate}</small>
                                </div>
                              )}
                              {selectedOperation.mockMetadata.state.mutationKeyTemplate && (
                                <div className='suite-runtime-list__item'>
                                  <strong>Mutation Key</strong>
                                  <small>{selectedOperation.mockMetadata.state.mutationKeyTemplate}</small>
                                </div>
                              )}
                              <div className='suite-runtime-list__item'>
                                <strong>Transitions</strong>
                                <span>{selectedOperation.mockMetadata.state.transitions?.length ?? 0} transition(s)</span>
                              </div>
                            </div>
                          )
                        : <p className='suite-runtime-empty'>No persisted state for this operation.</p>}
                    </div>
                  </div>

                  <div className='suite-exchange-grid'>
                    <div className='suite-exchange-card'>
                      <div className='suite-exchange-card__head'>
                        <div>
                          <p className='suite-eyebrow'>Request</p>
                          <h4>{selectedOperation.method} {selectedOperation.name}</h4>
                        </div>
                        <span>{selectedExchange.sourceArtifact}</span>
                      </div>
                      <HeaderList headers={selectedExchange.requestHeaders} />
                      <pre>{selectedExchange.requestBody || '(empty body)'}</pre>
                    </div>
                    <div className='suite-exchange-card'>
                      <div className='suite-exchange-card__head'>
                        <div>
                          <p className='suite-eyebrow'>Response</p>
                          <h4>{selectedExchange.responseStatus} — {selectedExchange.responseMediaType}</h4>
                        </div>
                        <span>Mocked</span>
                      </div>
                      <HeaderList headers={selectedExchange.responseHeaders} />
                      <pre>{selectedExchange.responseBody}</pre>
                    </div>
                  </div>
                </div>
              </div>
            </section>
          )}
        </div>

        {/* ── Right sidebar ── */}
        <aside className='suite-sidebar'>
          {/* Metadata */}
          <div className='suite-info-card'>
            <p className='suite-info-card__title'>Package Info</p>

            <div className='suite-info-row'><span>Owner</span><strong>{suite.owner}</strong></div>
            <div className='suite-info-row'><span>Provider</span><strong>{suite.provider}</strong></div>
            <div className='suite-info-row'><span>Version</span><strong>{suite.version}</strong></div>
            <div className='suite-info-row'><span>Score</span><strong>{suite.score.toFixed(1)}</strong></div>

            {suite.modules.length > 0 && (
              <div className='suite-info-section'>
                <p className='suite-info-section__label'>Modules</p>
                <div className='suite-pills'>
                  {suite.modules.map((m) => (
                    <span key={m} className='suite-pill suite-pill--module'>{m.replace('@babelsuite/', '')}</span>
                  ))}
                </div>
              </div>
            )}

            {suite.tags?.length > 0 && (
              <div className='suite-info-section'>
                <p className='suite-info-section__label'>Tags</p>
                <div className='suite-pills'>
                  {suite.tags.map((t) => (
                    <span key={t} className='suite-pill'>{t}</span>
                  ))}
                </div>
              </div>
            )}

            {suite.contracts.length > 0 && (
              <div className='suite-info-section'>
                <p className='suite-info-section__label'>Registered Modules</p>
                {suite.contracts.map((c) => (
                  <p key={c} className='suite-info-mono'>{c}</p>
                ))}
              </div>
            )}
          </div>

          {/* Commands */}
          <div className='suite-info-card'>
            <p className='suite-info-card__title'>Commands</p>
            <div className='suite-cmd-row'>
              <div className='suite-cmd-row__label'>Pull</div>
              <code className='suite-cmd-row__value'>{suite.pullCommand}</code>
              <button
                type='button'
                className={`suite-cmd-row__copy${copiedId === 'pull' ? ' suite-cmd-row__copy--ok' : ''}`}
                onClick={() => void copyValue('pull', suite.pullCommand)}
              >
                <FaCopy />
              </button>
            </div>
            <div className='suite-cmd-row'>
              <div className='suite-cmd-row__label'>Fork</div>
              <code className='suite-cmd-row__value'>{suite.forkCommand}</code>
              <button
                type='button'
                className={`suite-cmd-row__copy${copiedId === 'fork' ? ' suite-cmd-row__copy--ok' : ''}`}
                onClick={() => void copyValue('fork', suite.forkCommand)}
              >
                <FaDownload />
              </button>
            </div>
          </div>

          {/* Profiles */}
          {suite.profiles.length > 0 && (
            <div className='suite-info-card'>
              <p className='suite-info-card__title'>Profiles</p>
              {suite.profiles.map((p) => (
                <div key={p.fileName} className='suite-profile-row'>
                  <div>
                    <strong>{p.label}</strong>
                    <small>{p.fileName}</small>
                  </div>
                  {p.default && <span className='suite-default-badge'>default</span>}
                </div>
              ))}
            </div>
          )}

          {/* Package explorer */}
          {(suite.folders.length > 0 || rootSourceFiles.length > 0) && (
            <div className='suite-info-card'>
              <p className='suite-info-card__title'>Package Explorer</p>
              {rootSourceFiles.length > 0 && (
                <div className='suite-folder-detail'>
                  <p className='suite-eyebrow'>Root Files</p>
                  <div className='suite-folder-files'>
                    {rootSourceFiles.map((file) => (
                      <button
                        key={file.path}
                        type='button'
                        className={`suite-folder-file${selectedSourcePath === file.path ? ' suite-folder-file--active' : ''}`}
                        onClick={() => setSelectedSourcePath(file.path)}
                        title={file.path}
                      >
                        {file.path}
                      </button>
                    ))}
                  </div>
                </div>
              )}
              <div className='suite-folder-list'>
                {suite.folders.map((folder) => (
                  <button
                    key={folder.name}
                    type='button'
                    className={`suite-folder-btn${folder.name === activeFolder?.name ? ' suite-folder-btn--active' : ''}`}
                    onClick={() => {
                      setSelectedFolder(folder.name)
                      const previewPath = folder.files
                        .map((fileName) => `${folder.name}/${fileName}`)
                        .find((path) => sourceFileByPath.has(path))
                      if (previewPath) setSelectedSourcePath(previewPath)
                    }}
                  >
                    <span className='suite-folder-btn__name'>{folder.name}/</span>
                    <span className='suite-folder-btn__count'>{folder.files.length}</span>
                  </button>
                ))}
              </div>
              {activeFolder && (
                <div className='suite-folder-detail'>
                  <p className='suite-eyebrow'>{activeFolder.role}</p>
                  <p>{activeFolder.description}</p>
                  <div className='suite-folder-files'>
                    {activeFolder.files.map((fileName) => {
                      const sourcePath = `${activeFolder.name}/${fileName}`
                      const hasPreview = sourceFileByPath.has(sourcePath)
                      return (
                        <button
                          key={sourcePath}
                          type='button'
                          className={`suite-folder-file${selectedSourcePath === sourcePath ? ' suite-folder-file--active' : ''}`}
                          disabled={!hasPreview}
                          onClick={() => selectSourceFile(sourcePath)}
                          title={hasPreview ? sourcePath : `${sourcePath} preview is unavailable`}
                        >
                          {fileName}
                        </button>
                      )
                    })}
                  </div>
                </div>
              )}
            </div>
          )}
        </aside>
      </div>

      {/* ── Run modal ── */}
      {showRunModal && (
        <div className='suite-modal' role='presentation'>
          <div className='suite-modal__backdrop' onClick={() => setShowRunModal(false)} />
          <div className='suite-modal__dialog' role='dialog' aria-modal='true' aria-label='Run suite'>
            <div className='suite-modal__header'>
              <div>
                <p className='suite-eyebrow'>Launch Execution</p>
                <h3>Run {suite.title}</h3>
              </div>
              <button type='button' className='suite-inline-btn' onClick={() => setShowRunModal(false)}>Close</button>
            </div>

            <label className='suite-field'>
              <span>Profile</span>
              <select value={selectedProfile} onChange={(e) => setSelectedProfile(e.target.value)}>
                {suite.profiles.map((p) => (
                  <option key={p.fileName} value={p.fileName}>{p.fileName} — {p.label}</option>
                ))}
              </select>
            </label>

            <div className='suite-profile-picker'>
              {suite.profiles.map((p) => (
                <button
                  key={p.fileName}
                  type='button'
                  className={`suite-profile-card${p.fileName === selectedProfile ? ' suite-profile-card--active' : ''}`}
                  onClick={() => setSelectedProfile(p.fileName)}
                >
                  <strong>{p.label}</strong>
                  <span>{p.description}</span>
                </button>
              ))}
            </div>

            <label className='suite-field'>
              <span>Backend</span>
              <select value={selectedBackend} onChange={(e) => setSelectedBackend(e.target.value)}>
                <option value='auto'>auto — pick the default available backend</option>
                {(launchSuite?.backends ?? []).map((backend) => (
                  <option key={backend.id} value={backend.id} disabled={!backend.available}>
                    {backend.label} — {backend.available ? backend.kind : 'unavailable'}
                  </option>
                ))}
              </select>
            </label>

            {(launchSuite?.backends?.length ?? 0) > 0 && (
              <div className='suite-profile-picker'>
                <button
                  type='button'
                  className={`suite-profile-card${selectedBackend === 'auto' ? ' suite-profile-card--active' : ''}`}
                  onClick={() => setSelectedBackend('auto')}
                >
                  <strong>Auto</strong>
                  <span>Choose the default available backend when the run starts.</span>
                </button>
                {(launchSuite?.backends ?? []).map((backend) => (
                  <button
                    key={backend.id}
                    type='button'
                    className={`suite-profile-card${backend.id === selectedBackend ? ' suite-profile-card--active' : ''}`}
                    onClick={() => setSelectedBackend(backend.id)}
                    disabled={!backend.available}
                  >
                    <strong>{backend.label}</strong>
                    <span>{backend.description || backend.kind}</span>
                  </button>
                ))}
              </div>
            )}

            <div className='suite-modal__footer'>
              <button type='button' className='suite-btn suite-btn--secondary' onClick={() => setShowRunModal(false)}>Cancel</button>
              <button type='button' className='suite-btn' onClick={() => void executeSuite()} disabled={launching || !selectedProfile}>
                <FaPlay />
                <span>{launching ? 'Executing…' : 'Execute'}</span>
              </button>
            </div>
          </div>
        </div>
      )}
    </AppShell>
  )
}


function fallbackSummary(operation: SuiteDefinition['apiSurfaces'][number]['operations'][number]): string {
  const fallback = operation.mockMetadata.fallback
  if (!fallback) return ''
  if (fallback.mode === 'example') return `Uses example ${fallback.exampleName || '(unknown)'}.`
  if (fallback.mode === 'proxy') return `Proxies unmatched traffic to ${fallback.proxyUrl || '(missing url)'}.`
  return `${fallback.status || 'default'} ${fallback.mediaType || ''}`.trim() || 'Static fallback response.'
}

function renderHighlightedLine(line: string): ReactNode[] {
  const commentIndex = line.indexOf('#')
  const code = commentIndex >= 0 ? line.slice(0, commentIndex) : line
  const comment = commentIndex >= 0 ? line.slice(commentIndex) : ''
  const fragments: ReactNode[] = []
  const pattern = /"[^"]*"|\b(load|service|task|test|traffic|suite|container|mock|script|scenario)\b|@[a-zA-Z0-9/_-]+/g
  let cursor = 0

  for (const match of code.matchAll(pattern)) {
    const value = match[0]
    const start = match.index ?? 0
    if (start > cursor) fragments.push(code.slice(cursor, start))
    const cls = value.startsWith('"')
      ? 'suite-token suite-token--string'
      : value.startsWith('@')
        ? 'suite-token suite-token--module'
        : 'suite-token suite-token--keyword'
    fragments.push(<span key={`${start}-${value}`} className={cls}>{value}</span>)
    cursor = start + value.length
  }

  if (cursor < code.length) fragments.push(code.slice(cursor))
  if (comment) fragments.push(<span key={`comment-${comment}`} className='suite-token suite-token--comment'>{comment}</span>)
  return fragments
}

function preferredSourcePath(
  suite: SuiteDefinition,
  operation?: SuiteDefinition['apiSurfaces'][number]['operations'][number],
): string {
  const availablePaths = new Set(suite.sourceFiles.map((file) => file.path))
  const candidates = [
    operation?.mockPath,
    operation?.mockMetadata.metadataPath,
    operation?.contractPath,
    suite.sourceFiles[0]?.path,
  ]

  for (const path of candidates) {
    if (typeof path === 'string' && availablePaths.has(path)) {
      return path
    }
  }
  return ''
}

function folderNameFromPath(path: string): string {
  const slashIndex = path.indexOf('/')
  return slashIndex >= 0 ? path.slice(0, slashIndex) : ''
}
