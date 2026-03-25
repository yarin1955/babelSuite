import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import {
  FaArrowLeft,
  FaCircleNodes,
  FaClock,
  FaCube,
  FaServer,
  FaWaveSquare,
} from 'react-icons/fa6'
import Layout from '../components/Layout'
import Page from '../components/Page'
import styles from './RunDetail.module.css'

const API = 'http://localhost:8090'
const WAVE_THRESHOLD_MS = 2500

interface Run {
  run_id: string
  package_id: string
  image_ref: string
  profile: string
  status: string
  started_at: string | null
  finished_at: string | null
  created_at: string
}

interface Step {
  step_id: string
  run_id: string
  name: string
  position: number
  type: string
  status: string
  exit_code: number
  error: string
  started_at: string | null
  finished_at: string | null
}

interface LogEntry {
  log_id: string
  step_id: string
  line: number
  data: string
  time: number
  type: number
}

interface LogLine {
  step_id: string
  line: number
  text: string
  time: number
  stderr: boolean
}

interface StepWave {
  id: string
  label: string
  anchor: number | null
  steps: Step[]
}

const STATUS_LABEL: Record<string, string> = {
  pending: 'Pending',
  running: 'Running',
  success: 'Success',
  failure: 'Failure',
  canceled: 'Canceled',
  error: 'Error',
}

export default function RunDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const token = localStorage.getItem('token') || ''

  const [run, setRun] = useState<Run | null>(null)
  const [steps, setSteps] = useState<Step[]>([])
  const [activeTab, setActiveTab] = useState('all')
  const [logsByStep, setLogsByStep] = useState<Record<string, LogLine[]>>({})
  const [streamingSteps, setStreamingSteps] = useState<Record<string, boolean>>({})
  const [autoScroll, setAutoScroll] = useState(true)
  const [err, setErr] = useState('')

  const logBoxRef = useRef<HTMLDivElement>(null)
  const streamsRef = useRef<Record<string, EventSource>>({})
  const lastLineRef = useRef<Record<string, number>>({})
  const runStatusRef = useRef('')

  const orderedSteps = useMemo(
    () => [...steps].sort((left, right) => left.position - right.position || left.name.localeCompare(right.name)),
    [steps],
  )

  const stepMap = useMemo(
    () => Object.fromEntries(orderedSteps.map(step => [step.step_id, step])),
    [orderedSteps],
  )

  const stepPositionMap = useMemo(
    () => Object.fromEntries(orderedSteps.map(step => [step.step_id, step.position])),
    [orderedSteps],
  )

  const selectedStep = activeTab === 'all' ? null : stepMap[activeTab]

  const visibleLogs = useMemo(() => {
    if (activeTab !== 'all') {
      return [...(logsByStep[activeTab] ?? [])].sort((left, right) => left.line - right.line)
    }

    return Object.values(logsByStep)
      .flat()
      .sort((left, right) => {
        const timeDiff = (left.time || 0) - (right.time || 0)
        if (timeDiff !== 0) return timeDiff
        const positionDiff = (stepPositionMap[left.step_id] ?? 0) - (stepPositionMap[right.step_id] ?? 0)
        if (positionDiff != 0) return positionDiff
        return left.line - right.line
      })
  }, [activeTab, logsByStep, stepPositionMap])

  const liveStreamCount = useMemo(
    () => orderedSteps.filter(step => streamingSteps[step.step_id]).length,
    [orderedSteps, streamingSteps],
  )

  const suiteWaves = useMemo(() => inferWaves(orderedSteps), [orderedSteps])

  const summary = useMemo(() => ({
    steps: orderedSteps.length,
    services: orderedSteps.filter(step => step.type === 'service').length,
    mocks: orderedSteps.filter(step => isMockStep(step)).length,
  }), [orderedSteps])

  runStatusRef.current = run?.status ?? ''

  const closeStepStream = (stepID: string) => {
    const current = streamsRef.current[stepID]
    if (current) {
      current.close()
      delete streamsRef.current[stepID]
    }
    setStreamingSteps(existing => {
      if (!(stepID in existing)) return existing
      const next = { ...existing }
      delete next[stepID]
      return next
    })
  }

  const closeAllStreams = () => {
    Object.keys(streamsRef.current).forEach(closeStepStream)
    streamsRef.current = {}
    lastLineRef.current = {}
  }

  useEffect(() => {
    if (!id) return

    let cancelled = false
    const headers = { Authorization: `Bearer ${token}` }

    closeAllStreams()
    setRun(null)
    setSteps([])
    setLogsByStep({})
    setStreamingSteps({})
    setActiveTab('all')
    setErr('')

    const load = async () => {
      try {
        const [runRes, stepsRes] = await Promise.all([
          fetch(`${API}/api/runs/${id}`, { headers }),
          fetch(`${API}/api/runs/${id}/steps`, { headers }),
        ])

        const runData = await runRes.json().catch(() => null)
        const stepsData = await stepsRes.json().catch(() => null)

        if (!runRes.ok) {
          throw new Error((runData as { error?: string } | null)?.error || 'Failed to load run')
        }
        if (!stepsRes.ok) {
          throw new Error((stepsData as { error?: string } | null)?.error || 'Failed to load steps')
        }
        if (cancelled) return

        setRun(runData as Run)
        setSteps(Array.isArray(stepsData) ? stepsData : [])
      } catch (error) {
        if (cancelled) return
        setErr(error instanceof Error ? error.message : 'Failed to load run')
      }
    }

    void load()

    return () => {
      cancelled = true
      closeAllStreams()
    }
  }, [id, token])

  useEffect(() => {
    if (!id || !run || isTerminal(run.status)) return

    let cancelled = false
    const headers = { Authorization: `Bearer ${token}` }

    const interval = window.setInterval(async () => {
      try {
        const [runRes, stepsRes] = await Promise.all([
          fetch(`${API}/api/runs/${id}`, { headers }),
          fetch(`${API}/api/runs/${id}/steps`, { headers }),
        ])

        const nextRun = await runRes.json().catch(() => null)
        const nextSteps = await stepsRes.json().catch(() => null)
        if (cancelled) return

        if (runRes.ok && nextRun) setRun(nextRun as Run)
        if (stepsRes.ok && Array.isArray(nextSteps)) setSteps(nextSteps)
      } catch {
        // Keep the current snapshot on screen if a poll tick fails.
      }
    }, 3000)

    return () => {
      cancelled = true
      window.clearInterval(interval)
    }
  }, [id, run?.status, token])

  useEffect(() => {
    if (!id) return

    const activeStepIDs = new Set(orderedSteps.map(step => step.step_id))

    for (const step of orderedSteps) {
      if (streamsRef.current[step.step_id]) continue

      const url = `${API}/api/runs/${id}/logs/${step.step_id}?token=${encodeURIComponent(token)}`
      const source = new EventSource(url)
      streamsRef.current[step.step_id] = source
      setStreamingSteps(existing => ({ ...existing, [step.step_id]: true }))

      source.onmessage = event => {
        try {
          const entries: LogEntry[] = JSON.parse(event.data)
          const lastSeen = lastLineRef.current[step.step_id] ?? -1
          const nextLines = entries
            .filter(entry => entry.line > lastSeen)
            .map(entry => ({
              step_id: step.step_id,
              line: entry.line,
              text: entry.data,
              time: entry.time,
              stderr: entry.type === 1,
            }))

          if (nextLines.length === 0) return

          lastLineRef.current[step.step_id] = nextLines[nextLines.length - 1].line
          setLogsByStep(existing => ({
            ...existing,
            [step.step_id]: [...(existing[step.step_id] ?? []), ...nextLines],
          }))
        } catch {
          // Ignore malformed stream payloads.
        }
      }

      source.addEventListener('eof', () => {
        closeStepStream(step.step_id)
      })

      source.onerror = () => {
        if (isTerminal(runStatusRef.current)) {
          closeStepStream(step.step_id)
        }
      }
    }

    Object.keys(streamsRef.current).forEach(stepID => {
      if (!activeStepIDs.has(stepID)) {
        closeStepStream(stepID)
      }
    })
  }, [id, orderedSteps, token])

  useEffect(() => {
    if (activeTab === 'all') return
    if (!orderedSteps.some(step => step.step_id === activeTab)) {
      setActiveTab('all')
    }
  }, [activeTab, orderedSteps])

  useEffect(() => {
    if (!autoScroll || !logBoxRef.current) return
    logBoxRef.current.scrollTop = logBoxRef.current.scrollHeight
  }, [autoScroll, visibleLogs])

  const onScroll = () => {
    if (!logBoxRef.current) return
    const { scrollTop, scrollHeight, clientHeight } = logBoxRef.current
    setAutoScroll(scrollHeight - scrollTop - clientHeight < 40)
  }

  if (err) {
    return (
      <Layout>
        <Page title='Run'>
          <div className='auth-error'>{err}</div>
        </Page>
      </Layout>
    )
  }

  if (!run) {
    return (
      <Layout>
        <Page title='Run'>
          <div className='app-empty-state'>Loading run...</div>
        </Page>
      </Layout>
    )
  }

  const activeIsStreaming = activeTab === 'all'
    ? liveStreamCount > 0
    : Boolean(selectedStep && streamingSteps[selectedStep.step_id])

  return (
    <Layout>
      <Page
        title={`Run ${run.run_id.slice(0, 8)}`}
        toolbar={(
          <button className='app-button app-button--secondary' onClick={() => navigate('/runs')}>
            <FaArrowLeft />
            Back to runs
          </button>
        )}
      >
        <section className={styles.hero}>
          <div className={styles.heroPrimary}>
            <div className={styles.eyebrow}>Suite execution</div>
            <h1>{run.image_ref || run.package_id}</h1>
            <p>Follow the whole suite from one place: every container and mock in the tab bar, unified output on the left, and the execution graph on the right.</p>
          </div>
          <div className={styles.metricGrid}>
            <Metric label='Status' value={STATUS_LABEL[run.status] || run.status} tone={run.status} icon={<FaWaveSquare />} />
            <Metric label='Steps' value={String(summary.steps)} icon={<FaCube />} />
            <Metric label='Services' value={String(summary.services)} icon={<FaServer />} />
            <Metric label='Mocks' value={String(summary.mocks)} icon={<FaCircleNodes />} />
          </div>
        </section>

        <section className={styles.runMeta}>
          <div className={styles.metaItem}>
            <span>Package</span>
            <strong>{run.package_id || 'ad hoc run'}</strong>
          </div>
          <div className={styles.metaItem}>
            <span>Profile</span>
            <strong>{run.profile || 'default'}</strong>
          </div>
          <div className={styles.metaItem}>
            <span>Started</span>
            <strong>{formatTimestamp(run.started_at || run.created_at)}</strong>
          </div>
          <div className={styles.metaItem}>
            <span>Finished</span>
            <strong>{run.finished_at ? formatTimestamp(run.finished_at) : 'Still active'}</strong>
          </div>
          <div className={styles.metaItem}>
            <span>Live streams</span>
            <strong>{String(liveStreamCount)}</strong>
          </div>
        </section>

        <nav className={styles.tabBar} aria-label='Run steps'>
          <button
            className={`${styles.tab}${activeTab === 'all' ? ` ${styles.tabActive}` : ''}`}
            onClick={() => setActiveTab('all')}
          >
            <span>All</span>
            <small>{visibleCountLabel(totalLogCount(logsByStep), liveStreamCount)}</small>
          </button>

          {orderedSteps.map(step => (
            <button
              key={step.step_id}
              className={`${styles.tab}${activeTab === step.step_id ? ` ${styles.tabActive}` : ''}`}
              onClick={() => setActiveTab(step.step_id)}
            >
              <span>{step.name}</span>
              <small>{stepKind(step)} • {STATUS_LABEL[step.status] || step.status}</small>
            </button>
          ))}
        </nav>

        <section className={styles.workspace}>
          <div className='panel-card'>
            <div className='panel-card__header panel-card__header--padded'>
              <div>
                <div className='panel-card__eyebrow'>Logs</div>
                <h3>{activeTab === 'all' ? 'All activity' : selectedStep?.name || 'Step logs'}</h3>
              </div>
              <div className={styles.headerTools}>
                {selectedStep && <span className='app-chip'>{stepKind(selectedStep)}</span>}
                <span className='app-chip'>{activeIsStreaming ? 'Streaming' : 'History'}</span>
                <label className={styles.autoScrollToggle}>
                  <input type='checkbox' checked={autoScroll} onChange={event => setAutoScroll(event.target.checked)} />
                  Auto-scroll
                </label>
              </div>
            </div>

            <div className={styles.consoleSummary}>
              <span>{activeTab === 'all' ? `${visibleLogs.length} merged lines` : `${visibleLogs.length} lines`}</span>
              <span>{activeIsStreaming ? 'Live output is still coming in.' : 'No active stream for this view.'}</span>
            </div>

            <div ref={logBoxRef} onScroll={onScroll} className={styles.logViewport}>
              {visibleLogs.length === 0 ? (
                <div className='app-empty-state'>
                  {activeIsStreaming ? 'Waiting for output...' : 'No logs available for this selection yet.'}
                </div>
              ) : (
                visibleLogs.map((line, index) => {
                  const sourceStep = stepMap[line.step_id]
                  return (
                    <div key={`${line.step_id}-${line.line}-${index}`} className={styles.logRow}>
                      <div className={styles.logMeta}>
                        <span className={styles.logSource}>{sourceStep?.name || line.step_id}</span>
                        <span className={styles.logTime}><FaClock />{formatClock(line.time)}</span>
                        <span className={styles.logLineNo}>#{line.line + 1}</span>
                      </div>
                      <div className={`${styles.logText}${line.stderr ? ` ${styles.logTextError}` : ''}`}>
                        {line.text || ' '}
                      </div>
                    </div>
                  )
                })
              )}
            </div>
          </div>

          <div className='panel-card'>
            <div className='panel-card__header panel-card__header--padded'>
              <div>
                <div className='panel-card__eyebrow'>Suite DAG</div>
                <h3>Execution flow</h3>
              </div>
              <div className={styles.headerTools}>
                <span className='app-chip'>60 / 40 layout</span>
              </div>
            </div>

            <div className={styles.dagBody}>
              <div className={styles.legend}>
                <span><FaServer />Container</span>
                <span><FaCircleNodes />Mock</span>
                <span><FaWaveSquare />Checks</span>
              </div>

              {suiteWaves.length === 0 ? (
                <div className='app-empty-state'>This run has not created any steps yet.</div>
              ) : (
                <div className={styles.waveRail}>
                  {suiteWaves.map((wave, index) => (
                    <div key={wave.id} className={styles.waveSegment}>
                      <div className={styles.waveColumn}>
                        <div className={styles.waveLabel}>{wave.label}</div>
                        <div className={styles.waveStack}>
                          {wave.steps.map(step => {
                            const selected = activeTab === step.step_id
                            return (
                              <button
                                key={step.step_id}
                                className={`${styles.node}${selected ? ` ${styles.nodeActive}` : ''}`}
                                onClick={() => setActiveTab(step.step_id)}
                              >
                                <span className={styles.nodeTop}>
                                  <span className={styles.nodeName}>{step.name}</span>
                                  <span className={`status-pill status-pill--${step.status}`}>{STATUS_LABEL[step.status] || step.status}</span>
                                </span>
                                <span className={styles.nodeMeta}>
                                  <span>{stepKind(step)}</span>
                                  <span>{step.started_at ? formatShortTime(step.started_at) : 'queued'}</span>
                                </span>
                              </button>
                            )
                          })}
                        </div>
                      </div>

                      {index < suiteWaves.length - 1 && (
                        <div className={styles.waveConnector} aria-hidden='true'>
                          <span />
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        </section>
      </Page>
    </Layout>
  )
}

function Metric(
  { label, value, icon, tone }: { label: string; value: string; icon: React.ReactNode; tone?: string },
) {
  return (
    <div className={styles.metricCard}>
      <div className={styles.metricIcon}>{icon}</div>
      <div>
        <div className={styles.metricLabel}>{label}</div>
        <div className={styles.metricValue}>{value}</div>
        {tone && <div className={styles.metricTone}>{tone}</div>}
      </div>
    </div>
  )
}

function inferWaves(steps: Step[]): StepWave[] {
  const ordered = [...steps].sort((left, right) => left.position - right.position || left.name.localeCompare(right.name))
  const waves: StepWave[] = []

  for (const step of ordered) {
    const startedAt = step.started_at ? new Date(step.started_at).getTime() : null
    const previous = waves[waves.length - 1]

    const sameTimedWave = previous
      && startedAt != null
      && previous.anchor != null
      && Math.abs(startedAt - previous.anchor) <= WAVE_THRESHOLD_MS

    const sameBootstrapWave = previous
      && step.type === 'service'
      && previous.steps.every(current => current.type === 'service')

    const samePendingWave = previous
      && startedAt == null
      && previous.anchor == null
      && previous.steps.length < 3

    if (previous && (sameTimedWave || sameBootstrapWave || samePendingWave)) {
      previous.steps.push(step)
      if (previous.anchor == null) previous.anchor = startedAt
      continue
    }

    waves.push({
      id: `wave-${waves.length + 1}`,
      label: `Wave ${waves.length + 1}`,
      anchor: startedAt,
      steps: [step],
    })
  }

  return waves
}

function isTerminal(status: string): boolean {
  return ['success', 'failure', 'canceled', 'error'].includes(status)
}

function isMockStep(step: Step): boolean {
  return step.name.toLowerCase().includes('mock')
}

function stepKind(step: Step): string {
  if (isMockStep(step)) return 'mock'
  if (step.type === 'service') return 'container'
  if (step.type === 'commands') return 'check'
  return step.type || 'step'
}

function totalLogCount(logsByStep: Record<string, LogLine[]>): number {
  return Object.values(logsByStep).reduce((count, lines) => count + lines.length, 0)
}

function visibleCountLabel(logCount: number, liveCount: number): string {
  if (liveCount > 0) return `${liveCount} live`
  if (logCount > 0) return `${logCount} lines`
  return 'No output'
}

function formatClock(time: number): string {
  if (!time) return '--:--:--'
  return new Date(time).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

function formatShortTime(iso: string): string {
  return new Date(iso).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
  })
}

function formatTimestamp(iso: string): string {
  return new Date(iso).toLocaleString()
}
