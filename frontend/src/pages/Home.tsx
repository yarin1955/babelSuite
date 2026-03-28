import { startTransition, useEffect, useMemo, useState } from 'react'
import { createPortal } from 'react-dom'
import {
  FaBolt,
  FaChevronDown,
  FaCircleCheck,
  FaCircleXmark,
  FaDiagramProject,
  FaDocker,
  FaPlay,
  FaSpinner,
  FaTerminal,
} from 'react-icons/fa6'
import { useNavigate } from 'react-router-dom'
import AppShell from '../components/AppShell'
import {
  createExecution,
  getExecutionOverview,
  getSession,
  listExecutionLaunchSuites,
  type ExecutionLaunchSuite,
  type ExecutionOverview,
  type ExecutionOverviewItem,
} from '../lib/api'
import './Home.css'

export default function Home() {
  const navigate = useNavigate()
  const session = getSession()
  const [overview, setOverview] = useState<ExecutionOverview | null>(null)
  const [launchSuites, setLaunchSuites] = useState<ExecutionLaunchSuite[]>([])
  const [showModal, setShowModal] = useState(false)
  const [suiteId, setSuiteId] = useState('')
  const [profile, setProfile] = useState('')
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
            trigger: execution.trigger,
            status: execution.status,
            duration: execution.duration,
            startedAt: execution.startedAt,
            updatedAt: execution.startedAt,
            totalSteps: 0,
            runningSteps: 0,
            healthySteps: 0,
            failedSteps: 0,
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
          <div className='runs-toolbar__identity'>
            <strong>{session.workspace.name}</strong>
            <span>{session.user.email}</span>
          </div>
          <button type='button' className='runs-button runs-button--secondary' onClick={() => navigate('/sandbox')}>
            <FaDocker />
            <span>Sandbox</span>
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
          <div className='runs-panel runs-panel--launch'>
            <div className='runs-panel__header'>
              <div>
                <p className='runs-panel__eyebrow'>Execution Modal</p>
                <h2>Fast path to a booted environment</h2>
              </div>
              <button type='button' className='runs-button runs-button--secondary' onClick={() => setShowModal(true)} disabled={launchSuites.length === 0}>
                <FaBolt />
                <span>Open Launcher</span>
              </button>
            </div>

          </div>

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
                      onClick={() => navigate(`/suites/${run.suiteId}`)}
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
          }}
          onChangeProfile={setProfile}
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
  error: string
  creating: boolean
  onChangeSuite: (id: string) => void
  onChangeProfile: (fileName: string) => void
  onClose: () => void
  onExecute: () => void
}

function ExecutionModal({
  launchSuites,
  selectedSuite,
  profile,
  error,
  creating,
  onChangeSuite,
  onChangeProfile,
  onClose,
  onExecute,
}: ExecutionModalProps) {
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        onClose()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

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
          {/* Suite picker */}
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

          {/* Profile picker */}
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
  return `${run.healthySteps + run.runningSteps + run.failedSteps}/${run.totalSteps} steps`
}
