import { useEffect, useMemo, useState } from 'react'
import {
  FaBoxOpen,
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
  resolveExecutionRef,
  type CatalogPackage,
  type ExecutionLaunchSuite,
} from '../lib/api'
import {
  extractSuiteRef,
  extractSuiteRefs,
  resolveCatalogPackageRef,
  resolveLaunchSuiteRef,
  SANDBOX_SUITE_REF_MIME,
} from '../lib/sandboxRef'
import './Sandbox.css'

export default function Sandbox() {
  const navigate = useNavigate()
  const [launchSuites, setLaunchSuites] = useState<ExecutionLaunchSuite[]>([])
  const [catalogPackages, setCatalogPackages] = useState<CatalogPackage[]>([])
  const [loading, setLoading] = useState(true)
  const [loadingError, setLoadingError] = useState('')
  const [dragActive, setDragActive] = useState(false)
const [droppedRef, setDroppedRef] = useState('')
  const [selectedSuiteId, setSelectedSuiteId] = useState('')
  const [selectedProfile, setSelectedProfile] = useState('')
  const [selectedBackend, setSelectedBackend] = useState('auto')
  const [resolvedSuite, setResolvedSuite] = useState<ExecutionLaunchSuite | null>(null)
  const [rawRef, setRawRef] = useState('')
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

  const effectiveSuite = useMemo(
    () => launchSuites.find((item) => item.id === selectedSuiteId) ?? resolvedSuite,
    [launchSuites, selectedSuiteId, resolvedSuite],
  )

  useEffect(() => {
    if (!effectiveSuite) {
      setSelectedProfile('')
      setSelectedBackend('auto')
      return
    }

    setSelectedProfile((current) => (
      effectiveSuite.profiles.some((item) => item.fileName === current)
        ? current
        : effectiveSuite.profiles.find((item) => item.default)?.fileName ?? effectiveSuite.profiles[0]?.fileName ?? ''
    ))
    setSelectedBackend((current) => (
      current === 'auto' || effectiveSuite.backends.some((item) => item.id === current)
        ? current
        : 'auto'
    ))
  }, [effectiveSuite])

  const resolveDroppedSuite = async (candidate: string) => {
    const ref = extractSuiteRef(candidate)
    setDroppedRef(ref)
    setResolveError('')
    setLaunchError('')
    setResolvedSuite(null)
    setRawRef('')

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
    try {
      const backendSuite = await resolveExecutionRef(ref)
      setResolvedSuite(backendSuite)
    } catch {
      const catalogMatch = resolveCatalogPackageRef(catalogPackages, ref)
      if (catalogMatch) {
        setResolveError(`"${catalogMatch.title}" is visible in the catalog, but it is not available in the runnable suite set on this machine yet.`)
      } else {
        setRawRef(ref)
      }
    }
  }

  const resolveDroppedCandidates = (candidates: string[]) => {
    const refs = Array.from(new Set(candidates.flatMap((candidate) => extractSuiteRefs(candidate)).filter(Boolean)))
    if (refs.length === 0) {
      void resolveDroppedSuite('')
      return
    }

    const runnableRef = refs.find((ref) => resolveLaunchSuiteRef(launchSuites, ref))
    if (runnableRef) {
      void resolveDroppedSuite(runnableRef)
      return
    }

    const catalogRef = refs.find((ref) => resolveCatalogPackageRef(catalogPackages, ref))
    if (catalogRef) {
      void resolveDroppedSuite(catalogRef)
      return
    }

    void resolveDroppedSuite(refs[0])
  }

  const handleDrop = (event: React.DragEvent<HTMLDivElement>) => {
    event.preventDefault()
    setDragActive(false)
    const transfer = event.dataTransfer
    void droppedTransferCandidates(transfer).then((candidates) => {
      resolveDroppedCandidates(candidates)
    })
  }

  const launchSandbox = async (suiteIdOverride?: string) => {
    const suiteId = suiteIdOverride ?? effectiveSuite?.id ?? ''
    if (!suiteId) {
      return
    }

    setLaunching(true)
    setLaunchError('')
    try {
      const execution = await createExecution({
        suiteId,
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
              {effectiveSuite && selectedSuiteId && (
                <button type='button' className='sbx-inline-link' onClick={() => navigate(`/suites/${effectiveSuite.id}`)}>
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
              <p>Drag a suite card, drop an `examples/oci-suites/...` folder or file, or paste plain text like:</p>
              <code>localhost:5000/qa/storefront-browser-lab:workspace</code>
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

            {rawRef && !effectiveSuite && (
              <div className='sbx-suite-card'>
                <div className='sbx-alert sbx-alert--warning'>
                  <FaTriangleExclamation />
                  <span>Ref not found in the local suite registry — structure not validated. You can still attempt to launch it.</span>
                </div>
                <div className='sbx-suite-card__footer'>
                  <div>
                    <span className='sbx-muted'>Raw ref</span>
                    <code>{rawRef}</code>
                  </div>
                  <button type='button' className='sbx-action sbx-action--primary' onClick={() => void launchSandbox(rawRef)} disabled={launching}>
                    <FaPlay />
                    <span>{launching ? 'Launching…' : 'Try Launch'}</span>
                  </button>
                </div>
              </div>
            )}

            {effectiveSuite && (
              <div className='sbx-suite-card'>
                <div className='sbx-suite-card__head'>
                  <div>
                    <strong>{effectiveSuite.title}</strong>
                    <p>{effectiveSuite.repository}</p>
                  </div>
                  <span className='sbx-badge'>{effectiveSuite.provider}</span>
                </div>
                <p className='sbx-suite-card__body'>{effectiveSuite.description || 'No description published for this suite.'}</p>

                <div className='sbx-choice-group'>
                  <p className='sbx-choice-group__label'>Profile</p>
                  <div className='sbx-choice-grid'>
                    {effectiveSuite.profiles.map((item) => (
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
                    {effectiveSuite.backends.map((item) => (
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
                    <code>{droppedRef || effectiveSuite.repository}</code>
                  </div>
                  <button type='button' className='sbx-action sbx-action--primary' onClick={() => void launchSandbox()} disabled={launching || !selectedProfile}>
                    <FaPlay />
                    <span>{launching ? 'Launching…' : 'Run in Sandbox'}</span>
                  </button>
                </div>
              </div>
            )}
          </div>

        </section>
      </div>
    </AppShell>
  )
}

type DroppedFile = File & {
  path?: string
  webkitRelativePath?: string
}

type DragTransferItem = DataTransferItem & {
  getAsFileSystemHandle?: () => Promise<{ kind?: string; name?: string } | null>
  webkitGetAsEntry?: () => { fullPath?: string; name?: string } | null
}

async function droppedTransferCandidates(dataTransfer: DataTransfer) {
  const candidates: string[] = []
  const push = (value?: string | null) => {
    const trimmed = value?.trim()
    if (trimmed) {
      candidates.push(trimmed)
    }
  }

  for (const type of [SANDBOX_SUITE_REF_MIME, 'text/plain', 'text/uri-list', 'DownloadURL']) {
    push(dataTransfer.getData(type))
  }

  for (const item of Array.from(dataTransfer.items) as DragTransferItem[]) {
    if (item.kind !== 'file') {
      continue
    }

    const file = item.getAsFile() as DroppedFile | null
    push(file?.path)
    push(file?.webkitRelativePath)
    push(file?.name)

    const entry = item.webkitGetAsEntry?.()
    push(entry?.fullPath)
    push(entry?.name)

    if (typeof item.getAsFileSystemHandle === 'function') {
      try {
        const handle = await item.getAsFileSystemHandle()
        push(handle?.name)
      } catch {
        // Some browsers expose the API but still reject it for drops from outside the page.
      }
    }
  }

  for (const file of Array.from(dataTransfer.files) as DroppedFile[]) {
    push(file.path)
    push(file.webkitRelativePath)
    push(file.name)
  }

  return Array.from(new Set(candidates))
}
