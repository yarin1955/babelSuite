import { useEffect, useMemo, useRef, useState } from 'react'
import {
  FaArrowsRotate,
  FaBoxOpen,
  FaCopy,
  FaDiagramProject,
  FaFlask,
  FaMagnifyingGlass,
  FaPause,
  FaPlay,
  FaXmark,
} from 'react-icons/fa6'
import { useNavigate, useParams } from 'react-router-dom'
import AppShell from '../components/AppShell'
import { ExecutionDag } from '../components/execution/ExecutionDag'
import { ArtifactsDialog, MockDialog } from '../components/execution/ExecutionDialogs'
import { createExecution, type ExecutionLogLine, type TrafficMetricSnapshot } from '../lib/api'
import { useClipboardFeedback } from '../hooks/useClipboardFeedback'
import { useExecutionStream } from '../hooks/useExecutionStream'
import {
  deriveRuntimeStatus,
  groupTopologyByLevel,
  parseSuiteTopology,
  type RuntimeStatus,
} from '../lib/suites'
import './LiveExecution.css'

export default function LiveExecution() {
  const navigate = useNavigate()
  const params = useParams()
  const executionId = params.executionId ?? ''
  const {
    execution,
    logs,
    loading,
    error,
    paused,
    setPaused,
    executionStreamState,
    logStreamState,
  } = useExecutionStream(executionId)
  const [selectedSource, setSelectedSource] = useState<'all' | string>('all')
  const [selectedMockPreviewId, setSelectedMockPreviewId] = useState('')
  const [showDag, setShowDag] = useState(false)
  const [showMockDialog, setShowMockDialog] = useState(false)
  const [showArtifactsDialog, setShowArtifactsDialog] = useState(false)
  const [logSearch, setLogSearch] = useState('')
  const [notice, setNotice] = useState('')
  const [actionError, setActionError] = useState('')
  const [restarting, setRestarting] = useState(false)
  const { copyToClipboard } = useClipboardFeedback(1600)
  const logRef = useRef<HTMLDivElement | null>(null)

  const topology = useMemo(
    () => execution
      ? groupTopologyByLevel(execution.suite.topology?.length ? execution.suite.topology : parseSuiteTopology(execution.suite.suiteStar))
      : [],
    [execution],
  )
  const flatTopology = useMemo(() => topology.flat(), [topology])
  const statusMap = useMemo(
    () => deriveRuntimeStatus(flatTopology, execution?.events ?? []),
    [execution?.events, flatTopology],
  )
  const filteredLogs = useMemo(() => {
    let result = selectedSource === 'all' ? logs : logs.filter((line) => line.source === selectedSource)
    // Never render metric payloads in the terminal — they go to the stats panel
    result = result.filter((line) => line.kind !== 'metric')
    if (logSearch.trim()) {
      const term = logSearch.toLowerCase()
      result = result.filter((line) => line.text.toLowerCase().includes(term))
    }
    return result
  }, [logs, selectedSource, logSearch])

  // Latest metric snapshot per step source, derived from metric-kind lines
  const latestMetrics = useMemo(() => {
    const map = new Map<string, TrafficMetricSnapshot>()
    for (const line of logs) {
      if (line.kind !== 'metric') continue
      try { map.set(line.source, JSON.parse(line.text) as TrafficMetricSnapshot) } catch { /* ignore */ }
    }
    return map
  }, [logs])

  const selectedNodeKind = useMemo(
    () => flatTopology.find((n) => n.id === selectedSource)?.kind ?? '',
    [flatTopology, selectedSource],
  )
  const showMetricsPanel = selectedNodeKind === 'traffic' && latestMetrics.has(selectedSource)
  const mockPreviews = useMemo(
    () => (execution?.suite.apiSurfaces ?? []).flatMap((surface) => (
      surface.operations.flatMap((operation) => (
        operation.exchanges.map((exchange, index) => ({
          id: `${surface.id}:${operation.id}:${exchange.name}:${index}`,
          label: mockLabelFromPath(exchange.sourceArtifact || operation.mockPath || operation.id),
          language: languageFromMediaType(exchange.responseMediaType),
          content: exchange.responseBody || '(empty body)',
        }))
      ))
    )),
    [execution?.suite.apiSurfaces],
  )
  const artifacts = execution?.artifacts ?? []

  useEffect(() => {
    if (!logRef.current || paused) return
    logRef.current.scrollTop = logRef.current.scrollHeight
  }, [filteredLogs, paused])

  useEffect(() => {
    if (!execution) return
    setSelectedMockPreviewId(mockPreviews[0]?.id ?? '')
  }, [execution?.id])

  const copyVisibleLogs = async () => {
    const text = filteredLogs
      .map((line) => (selectedSource === 'all' ? `[${line.source}] ` : '') + line.text)
      .join('\n')
    await copyToClipboard('visible-logs', text)
    setNotice('Copied to clipboard.')
    window.setTimeout(() => setNotice(''), 1600)
  }

  const restartExecution = async () => {
    if (!execution) return
    setRestarting(true)
    setActionError('')
    try {
      const next = await createExecution({ suiteId: execution.suite.id, profile: execution.profile, backend: execution.backendId })
      navigate(`/executions/${next.id}`)
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Could not restart execution.')
    } finally {
      setRestarting(false)
    }
  }

  if (!execution) {
    return (
      <AppShell section='Live Execution' title='Loading execution' description=''>
        <div className='execution-page execution-page--empty'>
          <div className='exec-splash'>
            <div className={`exec-splash__dot${error ? ' exec-splash__dot--error' : ''}`} />
            <span>{error || (loading ? 'Connecting to execution stream…' : 'Waiting for execution data…')}</span>
          </div>
        </div>
      </AppShell>
    )
  }

  const readyNodes = flatTopology.filter((n) => statusMap[n.id] === 'healthy').length
  const activeNodes = flatTopology.filter((n) => statusMap[n.id] === 'running').length
  const failedNodes = flatTopology.filter((n) => statusMap[n.id] === 'failed').length
  const skippedNodes = flatTopology.filter((n) => statusMap[n.id] === 'skipped').length
  const pendingNodes = flatTopology.filter((n) => statusMap[n.id] === 'pending').length
  const progress = flatTopology.length === 0
    ? (execution.status === 'Healthy' ? 100 : 0)
    : Math.round(((readyNodes + activeNodes + failedNodes + skippedNodes) / flatTopology.length) * 100)

  const alert = notice || actionError || error
  const activeMockPreview = mockPreviews.find((preview) => preview.id === selectedMockPreviewId) ?? mockPreviews[0]

  const selectMockPreview = (id: string) => {
    setSelectedMockPreviewId(id)
  }

  const copyMockPreview = async () => {
    if (!activeMockPreview) return
    await copyToClipboard(activeMockPreview.id, activeMockPreview.content)
    setNotice(`${activeMockPreview.label} copied.`)
    window.setTimeout(() => setNotice(''), 1600)
  }

  const focusMockPreview = () => {
    const nextPreview = mockPreviews[0]
    if (nextPreview) {
      selectMockPreview(nextPreview.id)
    }
  }

  return (
    <AppShell
      section='Live Execution'
      title={execution.suite.title}
      description=''
      actions={(
        <>
          <button
            type='button'
            className='exec-toolbar-btn exec-toolbar-btn--ghost'
            disabled={artifacts.length === 0}
            onClick={() => setShowArtifactsDialog(true)}
          >
            <FaBoxOpen />
            <span>Artifacts</span>
          </button>
          <button
            type='button'
            className='exec-toolbar-btn exec-toolbar-btn--ghost'
            disabled={mockPreviews.length === 0}
            onClick={() => setShowMockDialog(true)}
          >
            <FaFlask />
            <span>Mock</span>
          </button>
          <button
            type='button'
            className='exec-toolbar-btn exec-toolbar-btn--ghost'
            onClick={() => setShowDag(true)}
          >
            <FaDiagramProject />
            <span>Graph</span>
          </button>
          <button
            type='button'
            className='exec-toolbar-btn exec-toolbar-btn--ghost'
            onClick={() => void restartExecution()}
            disabled={restarting}
          >
            <FaArrowsRotate />
            <span>{restarting ? 'Restarting…' : 'Restart'}</span>
          </button>
          <button
            type='button'
            className={`exec-toolbar-btn${paused ? ' exec-toolbar-btn--paused' : ''}`}
            onClick={() => setPaused((p) => !p)}
          >
            {paused ? <FaPlay /> : <FaPause />}
            <span>{paused ? 'Resume' : 'Pause'}</span>
          </button>
        </>
      )}
    >
      <div className='execution-page'>

        {/* ── Execution header strip ── */}
        <div className='exec-header'>
          <div className='exec-header__left'>
            <div className='exec-header__row'>
              <code className='exec-header__id'>{execution.id.slice(0, 12)}</code>
              <ExecStatusBadge status={execution.status} />
            </div>
            <p className='exec-header__sub'>
              {execution.suite.repository}
              {execution.message ? ` · ${execution.message}` : ''}
            </p>
          </div>

          <div className='exec-header__meta'>
            <ExecMeta label='Profile' value={execution.profile} />
            <ExecMeta label='Backend' value={execution.backend} />
            <ExecMeta label='Trigger' value={execution.trigger} />
            {execution.branch && <ExecMeta label='Branch' value={execution.branch} />}
            <ExecMeta label='Started' value={formatDateTime(execution.startedAt)} />
            {execution.duration && <ExecMeta label='Duration' value={execution.duration} />}
          </div>
        </div>

        {/* ── Progress bar ── */}
        <div className='exec-progress'>
          <div
            className={`exec-progress__fill${execution.status === 'Booting' ? ' exec-progress__fill--live' : ''}`}
            style={{ width: `${progress}%` }}
          />
        </div>

        {/* ── Alert banner ── */}
        {alert && (
          <div className={`exec-alert${actionError || error ? ' exec-alert--error' : ''}`}>
            {alert}
          </div>
        )}

        {/* ── Body: terminal + sidebar ── */}
        <div className='exec-body'>

          {/* ── Terminal panel ── */}
          <section className='exec-terminal'>

            {/* Log search bar */}
            <div className='exec-tabs'>
              <div className='exec-tabs__lead' />
              <div className='exec-log-search'>
                <FaMagnifyingGlass className='exec-log-search__icon' />
                <input
                  type='text'
                  className='exec-log-search__input'
                  placeholder='Search logs…'
                  value={logSearch}
                  onChange={(e) => setLogSearch(e.target.value)}
                />
                {logSearch && (
                  <button type='button' className='exec-log-search__clear' onClick={() => setLogSearch('')}>
                    <FaXmark />
                  </button>
                )}
              </div>

              <div className='exec-tabs__actions'>
                <button
                  type='button'
                  className='exec-tab-action'
                  title='Copy visible logs'
                  onClick={() => void copyVisibleLogs()}
                >
                  <FaCopy />
                </button>
                <button
                  type='button'
                  className='exec-tab-action'
                  title={paused ? 'Resume stream' : 'Pause stream'}
                  onClick={() => setPaused((p) => !p)}
                >
                  {paused ? <FaPlay /> : <FaPause />}
                </button>
              </div>
              <div className='exec-tabs__spacer' />
            </div>

            {/* Log body — replaced by metrics panel for traffic steps with live data */}
            {showMetricsPanel ? (
              <TrafficMetricsPanel snapshot={latestMetrics.get(selectedSource)!} />
            ) : (
              <div className='exec-log' ref={logRef}>
                {filteredLogs.length === 0 ? (
                  <div className='exec-log__empty'>
                    {loading ? 'Connecting…' : 'Waiting for log output from the execution stream.'}
                  </div>
                ) : (
                  filteredLogs.map((line, index) => (
                    <LogLine
                      key={`${line.timestamp}-${line.source}-${index}`}
                      line={line}
                      showPrefix={selectedSource === 'all'}
                      index={index + 1}
                    />
                  ))
                )}
              </div>
            )}

            {/* Stream status bar */}
            <div className='exec-stream-bar'>
              <StreamPill state={logStreamState} paused={paused} label='logs' />
              <span className='exec-stream-bar__sep'>·</span>
              <span className='exec-stream-bar__count'>{filteredLogs.length} lines</span>
              <div className='exec-stream-bar__right'>
                <StreamPill state={executionStreamState} paused={paused} label='events' />
              </div>
            </div>
          </section>

          {/* ── Topology sidebar ── */}
          <aside className='exec-sidebar'>
            <div className='exec-sidebar__head'>
              <span>Topology</span>
              {selectedSource !== 'all' && (
                <button type='button' className='exec-sidebar__reset' onClick={() => setSelectedSource('all')}>
                  clear
                </button>
              )}
            </div>

            <div className='exec-sidebar__nodes'>
              {topology.map((wave, wi) => (
                <div key={`wave-${wi}`} className='exec-wave'>
                  <span className='exec-wave__label'>Wave {wi + 1}</span>
                  {wave.map((node) => {
                    const st = statusMap[node.id]
                    const cnt = logs.filter((l) => l.source === node.id).length
                    return (
                      <button
                        key={node.id}
                        type='button'
                        className={`exec-node exec-node--${st}${selectedSource === node.id ? ' exec-node--selected' : ''}`}
                        onClick={() => {
                          setSelectedSource(node.id)
                          if (node.kind === 'mock') {
                            focusMockPreview()
                          }
                        }}
                      >
                        <ExecDot status={st} />
                        <div className='exec-node__info'>
                          <strong>{node.name}</strong>
                          <span>{node.kind}</span>
                        </div>
                        {cnt > 0 && <em className='exec-node__count'>{cnt}</em>}
                      </button>
                    )
                  })}
                </div>
              ))}

            </div>

            {/* Stats footer */}
            <div className='exec-sidebar__stats'>
              <div className='exec-stat'>
                <strong data-color='healthy'>{readyNodes}</strong>
                <small>healthy</small>
              </div>
              <div className='exec-stat'>
                <strong data-color='running'>{activeNodes}</strong>
                <small>running</small>
              </div>
              <div className='exec-stat'>
                <strong data-color='failed'>{failedNodes}</strong>
                <small>failed</small>
              </div>
              <div className='exec-stat'>
                <strong data-color='skipped'>{skippedNodes}</strong>
                <small>skipped</small>
              </div>
              <div className='exec-stat'>
                <strong>{pendingNodes}</strong>
                <small>pending</small>
              </div>
            </div>
          </aside>
        </div>
      </div>

      {showMockDialog && (
        <MockDialog
          mockPreviews={mockPreviews}
          activeMockPreview={activeMockPreview}
          onSelectMockPreview={selectMockPreview}
          onCopy={copyMockPreview}
          onClose={() => setShowMockDialog(false)}
        />
      )}
      {showArtifactsDialog && (
        <ArtifactsDialog
          artifacts={artifacts}
          onClose={() => setShowArtifactsDialog(false)}
        />
      )}
      {showDag && (
        <ExecutionDag
          topology={topology}
          flatTopology={flatTopology}
          statusMap={statusMap}
          selectedSource={selectedSource}
          onSelectSource={(id) => {
            setSelectedSource(id)
            if (flatTopology.find((node) => node.id === id)?.kind === 'mock') {
              focusMockPreview()
            }
            setShowDag(false)
          }}
          onClose={() => setShowDag(false)}
        />
      )}
    </AppShell>
  )
}

function mockLabelFromPath(path: string): string {
  return path.replace(/^mock\//, '')
}

function languageFromMediaType(mediaType: string): string {
  const normalized = mediaType.toLowerCase()
  if (normalized.includes('json')) return 'json'
  if (normalized.includes('xml')) return 'xml'
  if (normalized.includes('yaml') || normalized.includes('yml')) return 'yaml'
  if (normalized.includes('protobuf') || normalized.includes('grpc')) return 'protobuf'
  return 'text'
}

function ExecDot({ status }: { status: RuntimeStatus }) {
  return <span className={`exec-dot exec-dot--${status}`} />
}

function ExecStatusBadge({ status }: { status: 'Booting' | 'Healthy' | 'Failed' }) {
  return (
    <span className={`exec-status-badge exec-status-badge--${status.toLowerCase()}`}>
      <ExecDot status={statusFromExecution(status)} />
      {status}
    </span>
  )
}

function ExecMeta({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className='exec-meta'>
      <span>{label}</span>
      <strong className={mono ? 'exec-meta__mono' : undefined}>{value}</strong>
    </div>
  )
}

function StreamPill({
  state,
  paused,
  label,
}: {
  state: 'connecting' | 'live' | 'reconnecting' | 'closed'
  paused: boolean
  label: string
}) {
  if (paused) return <span className='exec-stream-pill'>paused</span>
  const live = state === 'live'
  return (
    <span className={`exec-stream-pill${live ? ' exec-stream-pill--live' : ''}`}>
      <span className={`exec-stream-dot${live ? ' exec-stream-dot--live' : ''}`} />
      {live
        ? `${label} live`
        : state === 'reconnecting'
          ? `${label} reconnecting`
          : state === 'closed'
            ? `${label} closed`
            : `${label} connecting`}
    </span>
  )
}

function TrafficMetricsPanel({ snapshot }: { snapshot: TrafficMetricSnapshot }) {
  const errPct = (snapshot.errorRate * 100).toFixed(2)
  const stat = (label: string, value: string, sub?: string) => (
    <div className='exec-metric-stat'>
      <span className='exec-metric-stat__value'>{value}</span>
      <span className='exec-metric-stat__label'>{label}</span>
      {sub && <span className='exec-metric-stat__sub'>{sub}</span>}
    </div>
  )
  return (
    <div className='exec-metrics-panel'>
      <div className='exec-metrics-panel__row exec-metrics-panel__row--primary'>
        {stat('Requests', snapshot.requests.toLocaleString())}
        {stat('Failures', snapshot.failures.toLocaleString(), `${errPct}%`)}
        {stat('RPS', snapshot.rps.toFixed(1))}
        {stat('Active Users', snapshot.users.toLocaleString())}
      </div>
      <div className='exec-metrics-panel__divider' />
      <div className='exec-metrics-panel__row'>
        {stat('Min', `${snapshot.minMs.toFixed(0)} ms`)}
        {stat('Avg', `${snapshot.avgMs.toFixed(0)} ms`)}
        {stat('P50', `${snapshot.p50Ms.toFixed(0)} ms`)}
        {stat('P95', `${snapshot.p95Ms.toFixed(0)} ms`)}
        {stat('P99', `${snapshot.p99Ms.toFixed(0)} ms`)}
        {stat('Max', `${snapshot.maxMs.toFixed(0)} ms`)}
      </div>
    </div>
  )
}

function LogLine({
  line,
  showPrefix,
  index,
}: {
  line: ExecutionLogLine
  showPrefix: boolean
  index: number
}) {
  const isOutput = line.kind === 'output'
  const text = showPrefix
    ? line.text.replace(new RegExp(`^\\[${line.source}\\]\\s*`), '')
    : line.text

  return (
    <div className={[
      'exec-log-line',
      `exec-log-line--${line.level}`,
      isOutput ? 'exec-log-line--output' : 'exec-log-line--system',
      showPrefix ? 'exec-log-line--multi' : '',
    ].filter(Boolean).join(' ')}>
      <span className='exec-log-line__num'>{index}</span>
      {!isOutput && <span className='exec-log-line__time'>{line.timestamp}</span>}
      {isOutput && <span className='exec-log-line__stream'>stdout</span>}
      {showPrefix && <span className='exec-log-line__src'>[{line.source}]</span>}
      <code className='exec-log-line__text'>{text}</code>
    </div>
  )
}

/* ── DAG overlay ────────────────────────────────────────── */

/* ── Helpers ────────────────────────────────────────────── */

function statusFromExecution(status: 'Booting' | 'Healthy' | 'Failed'): RuntimeStatus {
  if (status === 'Healthy') return 'healthy'
  if (status === 'Failed') return 'failed'
  return 'running'
}

function formatDateTime(value: string) {
  return new Date(value).toLocaleString()
}
