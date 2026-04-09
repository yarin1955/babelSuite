import { useEffect, useMemo, useState } from 'react'
import {
  FaArrowRotateRight,
  FaBoxOpen,
  FaCircleCheck,
  FaDocker,
  FaPlay,
  FaTerminal,
  FaTriangleExclamation,
} from 'react-icons/fa6'
import { useNavigate } from 'react-router-dom'
import AppShell from '../components/AppShell'
import {
  ApiError,
  createExecution,
  listCatalogPackages,
  listExecutionLaunchSuites,
  type CatalogPackage,
  type ExecutionLaunchSuite,
} from '../lib/api'
import {
  extractSuiteRef,
  resolveCatalogPackageRef,
  resolveLaunchSuiteRef,
  SANDBOX_SUITE_REF_MIME,
} from '../lib/sandboxRef'
import { useSandboxStream } from '../hooks/useSandboxStream'
import './Sandbox.css'

export default function Sandbox() {
  const navigate = useNavigate()
  const { snapshot, loading: inventoryLoading, refreshing, error: inventoryError, streamState, refresh } = useSandboxStream()
  const [launchSuites, setLaunchSuites] = useState<ExecutionLaunchSuite[]>([])
  const [catalogPackages, setCatalogPackages] = useState<CatalogPackage[]>([])
  const [loading, setLoading] = useState(true)
  const [loadingError, setLoadingError] = useState('')
  const [dragActive, setDragActive] = useState(false)
  const [rawRefInput, setRawRefInput] = useState('')
  const [droppedRef, setDroppedRef] = useState('')
  const [selectedSuiteId, setSelectedSuiteId] = useState('')
  const [selectedProfile, setSelectedProfile] = useState('')
  const [selectedBackend, setSelectedBackend] = useState('auto')
  const [resolveError, setResolveError] = useState('')
  const [launchError, setLaunchError] = useState('')
  const [launching, setLaunching] = useState(false)

  useEffect(() => {
    let active = true

    const load = async () => {
      try {
        const [nextLaunchSuites, nextCatalogPackages] = await Promise.all([
          listExecutionLaunchSuites(),
          listCatalogPackages(),
        ])
        if (!active) {
          return
        }
        setLaunchSuites(nextLaunchSuites)
        setCatalogPackages(nextCatalogPackages)
        setLoading(false)
      } catch (reason) {
        if (!active) {
          return
        }
        setLoadingError(reason instanceof Error ? reason.message : 'Could not load runnable suites.')
        setLoading(false)
      }
    }

    void load()
    return () => {
      active = false
    }
  }, [])

  const selectedSuite = useMemo(
    () => launchSuites.find((item) => item.id === selectedSuiteId) ?? null,
    [launchSuites, selectedSuiteId],
  )

  useEffect(() => {
    if (!selectedSuite) {
      setSelectedProfile('')
      setSelectedBackend('auto')
      return
    }

    setSelectedProfile((current) => (
      selectedSuite.profiles.some((item) => item.fileName === current)
        ? current
        : selectedSuite.profiles.find((item) => item.default)?.fileName ?? selectedSuite.profiles[0]?.fileName ?? ''
    ))
    setSelectedBackend((current) => (
      current === 'auto' || selectedSuite.backends.some((item) => item.id === current)
        ? current
        : 'auto'
    ))
  }, [selectedSuite])

  const recentEnvironments = useMemo(
    () => (snapshot?.sandboxes ?? []).slice(0, 4),
    [snapshot?.sandboxes],
  )

  const resolveDroppedSuite = (candidate: string) => {
    const ref = extractSuiteRef(candidate)
    setRawRefInput(ref)
    setDroppedRef(ref)
    setResolveError('')
    setLaunchError('')

    if (ref === '') {
      setSelectedSuiteId('')
      return
    }

    const launchMatch = resolveLaunchSuiteRef(launchSuites, ref)
    if (launchMatch) {
      setSelectedSuiteId(launchMatch.id)
      return
    }

    setSelectedSuiteId('')
    const catalogMatch = resolveCatalogPackageRef(catalogPackages, ref)
    if (catalogMatch) {
      setResolveError(`"${catalogMatch.title}" is visible in the catalog, but it is not available in the runnable suite set on this machine yet.`)
      return
    }

    setResolveError('That suite reference does not match any runnable suite right now.')
  }

  const handleDrop = (event: React.DragEvent<HTMLDivElement>) => {
    event.preventDefault()
    setDragActive(false)

    const ref = event.dataTransfer.getData(SANDBOX_SUITE_REF_MIME)
      || event.dataTransfer.getData('text/plain')
      || event.dataTransfer.getData('text/uri-list')

    resolveDroppedSuite(ref)
  }

  const launchSandbox = async () => {
    if (!selectedSuite || !selectedProfile) {
      return
    }

    setLaunching(true)
    setLaunchError('')
    try {
      const execution = await createExecution({
        suiteId: selectedSuite.id,
        profile: selectedProfile,
        backend: selectedBackend,
      })
      navigate(`/executions/${execution.id}`)
    } catch (reason) {
      setLaunchError(reason instanceof ApiError ? reason.message : 'Could not start the sandbox run.')
    } finally {
      setLaunching(false)
    }
  }

  return (
    <AppShell
      section='Sandbox'
      title='Drop & Run OCI Suite'
      description='Drop a suite reference, resolve it against the runnable suite set, and boot an isolated environment without leaving the app.'
      actions={(
        <>
          <button type='button' className='sbx-action' onClick={() => navigate('/environments')}>
            <FaDocker />
            <span>Open Environments</span>
          </button>
          <button type='button' className='sbx-action sbx-action--ghost' onClick={() => void refresh(true)} disabled={refreshing}>
            <FaArrowRotateRight />
            <span>{refreshing ? 'Refreshing…' : 'Refresh Inventory'}</span>
          </button>
        </>
      )}
    >
      <div className='sbx-page'>
        <section className='sbx-layout'>
          <div className='sbx-panel sbx-panel--launcher'>
            <div className='sbx-panel__header'>
              <div>
                <p className='sbx-panel__eyebrow'>Launch Pad</p>
                <h2>Resolve a suite reference</h2>
              </div>
              {selectedSuite && (
                <button type='button' className='sbx-inline-link' onClick={() => navigate(`/suites/${selectedSuite.id}`)}>
                  <FaTerminal />
                  <span>Inspect suite</span>
                </button>
              )}
            </div>

            <div
              className={`sbx-dropzone${dragActive ? ' sbx-dropzone--active' : ''}`}
              onDragEnter={(event) => { event.preventDefault(); setDragActive(true) }}
              onDragOver={(event) => { event.preventDefault(); setDragActive(true) }}
              onDragLeave={(event) => {
                event.preventDefault()
                if (event.currentTarget.contains(event.relatedTarget as Node | null)) {
                  return
                }
                setDragActive(false)
              }}
              onDrop={handleDrop}
            >
              <div className='sbx-dropzone__icon'><FaBoxOpen /></div>
              <strong>Drop a suite reference here</strong>
              <p>Drag a suite card from the catalog or drop plain text like:</p>
              <code>localhost:5000/qa/storefront-browser-lab:workspace</code>
            </div>

            <div className='sbx-input-row'>
              <input
                value={rawRefInput}
                onChange={(event) => setRawRefInput(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    event.preventDefault()
                    resolveDroppedSuite(rawRefInput)
                  }
                }}
                placeholder='Paste an OCI suite reference or a run command'
              />
              <button type='button' className='sbx-action' onClick={() => resolveDroppedSuite(rawRefInput)} disabled={loading}>
                Resolve
              </button>
            </div>

            {loading && (
              <div className='sbx-alert sbx-alert--info'>Loading runnable suites…</div>
            )}
            {loadingError && (
              <div className='sbx-alert sbx-alert--error'>{loadingError}</div>
            )}
            {resolveError && (
              <div className='sbx-alert sbx-alert--warning'>
                <FaTriangleExclamation />
                <span>{resolveError}</span>
              </div>
            )}
            {launchError && (
              <div className='sbx-alert sbx-alert--error'>{launchError}</div>
            )}

            {selectedSuite ? (
              <div className='sbx-suite-card'>
                <div className='sbx-suite-card__head'>
                  <div>
                    <strong>{selectedSuite.title}</strong>
                    <p>{selectedSuite.repository}</p>
                  </div>
                  <span className='sbx-badge'>{selectedSuite.provider}</span>
                </div>
                <p className='sbx-suite-card__body'>{selectedSuite.description || 'No description published for this suite.'}</p>

                <div className='sbx-choice-group'>
                  <p className='sbx-choice-group__label'>Profile</p>
                  <div className='sbx-choice-grid'>
                    {selectedSuite.profiles.map((item) => (
                      <button
                        key={item.fileName}
                        type='button'
                        className={`sbx-choice${item.fileName === selectedProfile ? ' sbx-choice--active' : ''}`}
                        onClick={() => setSelectedProfile(item.fileName)}
                      >
                        <strong>{item.label || item.fileName}</strong>
                        {item.description && <span>{item.description}</span>}
                        <code>{item.fileName}</code>
                      </button>
                    ))}
                  </div>
                </div>

                <div className='sbx-choice-group'>
                  <p className='sbx-choice-group__label'>Backend</p>
                  <div className='sbx-choice-grid'>
                    <button
                      type='button'
                      className={`sbx-choice${selectedBackend === 'auto' ? ' sbx-choice--active' : ''}`}
                      onClick={() => setSelectedBackend('auto')}
                    >
                      <strong>Auto</strong>
                      <span>Use the default available backend when the run starts.</span>
                      <code>auto</code>
                    </button>
                    {selectedSuite.backends.map((item) => (
                      <button
                        key={item.id}
                        type='button'
                        className={`sbx-choice${item.id === selectedBackend ? ' sbx-choice--active' : ''}`}
                        onClick={() => setSelectedBackend(item.id)}
                        disabled={!item.available}
                      >
                        <strong>{item.label}</strong>
                        {(item.description || item.kind) && <span>{item.description || item.kind}</span>}
                        <code>{item.available ? item.kind : 'unavailable'}</code>
                      </button>
                    ))}
                  </div>
                </div>

                <div className='sbx-suite-card__footer'>
                  <div>
                    <span className='sbx-muted'>Resolved ref</span>
                    <code>{droppedRef || selectedSuite.repository}</code>
                  </div>
                  <button type='button' className='sbx-action sbx-action--primary' onClick={() => void launchSandbox()} disabled={launching || !selectedProfile}>
                    <FaPlay />
                    <span>{launching ? 'Launching…' : 'Run in Sandbox'}</span>
                  </button>
                </div>
              </div>
            ) : (
              <div className='sbx-empty'>
                <p className='sbx-empty__eyebrow'>Ready</p>
                <h3>Drop a suite to begin</h3>
                <p>The launcher will match the dropped reference against the runnable suite set, then let you choose a profile and backend.</p>
              </div>
            )}
          </div>

          <div className='sbx-panel sbx-panel--inventory'>
            <div className='sbx-panel__header'>
              <div>
                <p className='sbx-panel__eyebrow'>Live Inventory</p>
                <h2>Current environments</h2>
              </div>
              <span className={`sbx-stream${streamState === 'live' ? ' sbx-stream--live' : streamState === 'reconnecting' ? ' sbx-stream--warn' : ''}`}>
                {streamState}
              </span>
            </div>

            {inventoryError && (
              <div className='sbx-alert sbx-alert--warning'>{inventoryError}</div>
            )}

            <div className='sbx-summary-grid'>
              <div className='sbx-summary-card'>
                <strong>{snapshot?.summary.activeSandboxes ?? 0}</strong>
                <span>Active</span>
              </div>
              <div className='sbx-summary-card'>
                <strong>{snapshot?.summary.zombieSandboxes ?? 0}</strong>
                <span>Zombie</span>
              </div>
              <div className='sbx-summary-card'>
                <strong>{snapshot?.summary.containers ?? 0}</strong>
                <span>Containers</span>
              </div>
              <div className='sbx-summary-card'>
                <strong>{formatBytes(snapshot?.summary.totalMemoryBytes ?? 0)}</strong>
                <span>Memory</span>
              </div>
            </div>

            {inventoryLoading && !snapshot ? (
              <div className='sbx-empty sbx-empty--compact'>
                <h3>Loading inventory…</h3>
              </div>
            ) : recentEnvironments.length > 0 ? (
              <div className='sbx-list'>
                {recentEnvironments.map((item) => (
                  <button key={item.sandboxId} type='button' className='sbx-environment' onClick={() => navigate('/environments')}>
                    <div className='sbx-environment__head'>
                      <strong>{item.suite}</strong>
                      <span className={`sbx-status sbx-status--${statusTone(item.status)}`}>{item.status}</span>
                    </div>
                    <p>{item.runId || item.sandboxId}</p>
                    <div className='sbx-environment__meta'>
                      <span>{item.profile}</span>
                      <span>{item.owner}</span>
                      <span>{item.containers.length} containers</span>
                    </div>
                  </button>
                ))}
              </div>
            ) : (
              <div className='sbx-empty sbx-empty--compact'>
                <p className='sbx-empty__eyebrow'>Quiet</p>
                <h3>No active environments</h3>
                <p>The next run you launch here will show up in the live environment inventory.</p>
              </div>
            )}
          </div>
        </section>
      </div>
    </AppShell>
  )
}

function statusTone(status: string) {
  const normalized = status.toLowerCase()
  if (normalized.includes('zombie') || normalized.includes('dead') || normalized.includes('error') || normalized.includes('exited')) {
    return 'danger'
  }
  if (normalized.includes('degraded') || normalized.includes('residual') || normalized.includes('stale')) {
    return 'warning'
  }
  if (normalized.includes('running') || normalized.includes('alive')) {
    return 'success'
  }
  return 'neutral'
}

function formatBytes(bytes: number) {
  if (bytes <= 0) {
    return '0 B'
  }

  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index += 1
  }
  return `${value.toFixed(value >= 10 || index === 0 ? 0 : 1)} ${units[index]}`
}
