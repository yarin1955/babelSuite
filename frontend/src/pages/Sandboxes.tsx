import { useMemo, useState } from 'react'
import {
  FaArrowRotateRight,
  FaBoxArchive,
  FaCircleXmark,
  FaDocker,
  FaHardDrive,
  FaLayerGroup,
  FaMagnifyingGlass,
  FaServer,
  FaTrash,
  FaTriangleExclamation,
} from 'react-icons/fa6'
import AppShell from '../components/AppShell'
import SlidingPanel from '../components/SlidingPanel'
import { ApiError, reapAllSandboxes, reapSandbox } from '../lib/api'
import { useSandboxStream } from '../hooks/useSandboxStream'
import './Sandboxes.css'

const STATUS_FILTERS = ['All', 'Running', 'Zombie', 'Degraded', 'Residual'] as const

type ActiveTab = 'containers' | 'networks' | 'volumes'

export default function Sandboxes() {
  const {
    snapshot,
    loading,
    refreshing,
    error,
    streamState,
    refresh,
  } = useSandboxStream()
  const [selectedSandboxId, setSelectedSandboxId] = useState('')
  const [panelOpen, setPanelOpen] = useState(false)
  const [activeTab, setActiveTab] = useState<ActiveTab>('containers')
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<(typeof STATUS_FILTERS)[number]>('All')
  const [busyTarget, setBusyTarget] = useState('')
  const [confirmNuke, setConfirmNuke] = useState('')
  const [confirmGlobal, setConfirmGlobal] = useState(false)
  const [message, setMessage] = useState<{ tone: 'success' | 'error' | 'warning'; text: string } | null>(null)

  const visibleSandboxes = useMemo(() => {
    const sandboxes = snapshot?.sandboxes ?? []
    return sandboxes.filter((sandbox) => {
      if (statusFilter !== 'All' && sandbox.status !== statusFilter) {
        return false
      }
      const haystack = [
        sandbox.suite,
        sandbox.runId,
        sandbox.owner,
        sandbox.profile,
        sandbox.summary,
      ].join(' ').toLowerCase()
      return haystack.includes(search.trim().toLowerCase())
    })
  }, [search, snapshot?.sandboxes, statusFilter])

  const selectedSandbox = snapshot?.sandboxes.find((s) => s.sandboxId === selectedSandboxId) ?? null

  const runAction = async (
    action: () => Promise<{ warnings: string[]; removedContainers: number; removedNetworks: number; removedVolumes: number }>,
    busyKey: string,
    successText: string,
  ) => {
    setBusyTarget(busyKey)
    setMessage(null)
    try {
      const result = await action()
      setMessage({
        tone: result.warnings.length > 0 ? 'warning' : 'success',
        text: result.warnings.length > 0
          ? `${successText} Some resources still reported issues: ${result.warnings.join(' ')}`
          : `${successText} Removed ${result.removedContainers} containers, ${result.removedNetworks} networks, and ${result.removedVolumes} volumes.`,
      })
      await refresh(true)
      setPanelOpen(false)
    } catch (reason) {
      setMessage({
        tone: 'error',
        text: reason instanceof ApiError ? reason.message : 'The cleanup action could not be completed.',
      })
    } finally {
      setBusyTarget('')
      setConfirmNuke('')
      setConfirmGlobal(false)
    }
  }

  const openSandbox = (sandboxId: string) => {
    setSelectedSandboxId(sandboxId)
    setActiveTab('containers')
    setConfirmNuke('')
    setPanelOpen(true)
  }

  const isLive = streamState === 'live'
  const isReconnecting = streamState === 'reconnecting'

  if (loading && !snapshot) {
    return (
      <AppShell
        section='Environments'
        title='Active Environments'
        description='Track every BabelSuite-managed Docker resource, spot zombie environments, and reclaim ports or memory before local development drifts.'
      >
        <div className='sandboxes-page'>
          <div className='sb-loading'>
            {[...Array(6)].map((_, i) => <div key={i} className='sb-loading__card' />)}
          </div>
        </div>
      </AppShell>
    )
  }

  return (
    <AppShell
      section='Environments'
      title='Active Environments'
      description='Track every BabelSuite-managed Docker resource, spot zombie environments, and reclaim ports or memory before local development drifts.'
      actions={(
        <>
          <span
            className={`sb-stream-dot${isLive ? ' sb-stream-dot--live' : isReconnecting ? ' sb-stream-dot--warn' : ''}`}
            title={isLive ? 'Live stream active' : isReconnecting ? 'Reconnecting stream' : 'Connecting stream'}
          />
          <button className='sb-btn sb-btn--ghost' onClick={() => void refresh(true)} disabled={refreshing}>
            <FaArrowRotateRight />
            <span>{refreshing ? 'Refreshing...' : 'Refresh'}</span>
          </button>
          {confirmGlobal ? (
            <div className='sb-confirm'>
              <span>Nuke all environments?</span>
              <button
                className='sb-btn sb-btn--danger'
                onClick={() => void runAction(() => reapAllSandboxes(), 'global', 'Global clean finished.')}
                disabled={busyTarget === 'global'}
              >
                {busyTarget === 'global' ? 'Cleaning...' : 'Confirm'}
              </button>
              <button className='sb-btn sb-btn--ghost' onClick={() => setConfirmGlobal(false)}>Cancel</button>
            </div>
          ) : (
            <button
              className='sb-btn sb-btn--danger'
              onClick={() => setConfirmGlobal(true)}
              disabled={!snapshot?.dockerAvailable || (snapshot?.sandboxes.length ?? 0) === 0}
            >
              <FaTrash />
              <span>Global Clean</span>
            </button>
          )}
        </>
      )}
    >
      <div className='sandboxes-page'>
        {message && (
          <div className={`sb-alert sb-alert--${message.tone}`}>{message.text}</div>
        )}
        {error && (
          <div className='sb-alert sb-alert--warning'>{error}</div>
        )}
        {snapshot?.warnings.map((warning) => (
          <div key={warning} className='sb-alert sb-alert--warning'>{warning}</div>
        ))}

        {/* Toolbar */}
        <div className='sb-toolbar'>
          <div className='sb-filters'>
            {STATUS_FILTERS.map((item) => (
              <button
                key={item}
                type='button'
                className={`sb-filter${statusFilter === item ? ' sb-filter--active' : ''}`}
                onClick={() => setStatusFilter(item)}
              >
                {item}
              </button>
            ))}
          </div>
          <label className='sb-search'>
            <FaMagnifyingGlass />
            <input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder='Search suite, run ID, owner, profile…'
            />
          </label>
        </div>

        {/* Card grid */}
        {snapshot && snapshot.sandboxes.length > 0 ? (
          <div className='sb-grid'>
            {visibleSandboxes.map((sandbox) => {
              const tone = statusTone(sandbox.status)
              const isAlarm = sandbox.status === 'Zombie' || sandbox.status === 'Degraded'
              return (
                <button
                  key={sandbox.sandboxId}
                  type='button'
                  className={`sb-card sb-card--${tone}${isAlarm ? ' sb-card--alarm' : ''}`}
                  onClick={() => openSandbox(sandbox.sandboxId)}
                >
                  <div className='sb-card__head'>
                    <strong>{sandbox.suite}</strong>
                    <span className={`sb-status sb-status--${tone}`}>{sandbox.status}</span>
                  </div>
                  <p className='sb-card__run'>{sandbox.runId || sandbox.sandboxId}</p>
                  <div className='sb-card__chips'>
                    <span className='sb-chip'><FaLayerGroup />{sandbox.profile}</span>
                    <span className='sb-chip'><FaServer />{sandbox.owner}</span>
                  </div>
                  <div className='sb-card__footer'>
                    <span><FaDocker />{sandbox.containers.length}c</span>
                    <span><FaBoxArchive />{sandbox.networks.length}n</span>
                    <span><FaHardDrive />{sandbox.volumes.length}v</span>
                    <span className='sb-card__mem'>{formatBytes(sandbox.resourceUsage.memoryBytes)}</span>
                  </div>
                </button>
              )
            })}

            {visibleSandboxes.length === 0 && (
              <div className='sb-empty-filter'>No environments match the current filter.</div>
            )}
          </div>
        ) : (
          <div className='sb-empty'>
            <p className='sb-empty__eyebrow'>No Managed Resources Yet</p>
            <h2>The reaper is ready, but nothing is tagged yet.</h2>
          </div>
        )}
      </div>

      {/* Sliding detail panel */}
      <SlidingPanel
        isOpen={panelOpen}
        onClose={() => { setPanelOpen(false); setConfirmNuke('') }}
        width='520px'
        header={selectedSandbox ? (
          <div className='sb-panel-header'>
            <div className='sb-panel-header__info'>
              <div className='sb-panel-header__title'>
                <strong>{selectedSandbox.suite}</strong>
                <span className={`sb-status sb-status--${statusTone(selectedSandbox.status)}`}>{selectedSandbox.status}</span>
              </div>
              <p className='sb-panel-header__run'>{selectedSandbox.runId || selectedSandbox.sandboxId}</p>
            </div>
            <div className='sb-panel-header__actions'>
              {confirmNuke === selectedSandbox.sandboxId ? (
                <div className='sb-confirm'>
                  <span>Sure?</span>
                  <button
                    className='sb-btn sb-btn--danger'
                    onClick={() => void runAction(() => reapSandbox(selectedSandbox.sandboxId), selectedSandbox.sandboxId, 'Environment reaped.')}
                    disabled={busyTarget === selectedSandbox.sandboxId}
                  >
                    {busyTarget === selectedSandbox.sandboxId ? 'Reaping...' : 'Nuke'}
                  </button>
                  <button className='sb-btn sb-btn--ghost sb-btn--icon' onClick={() => setConfirmNuke('')}>
                    <FaCircleXmark />
                  </button>
                </div>
              ) : (
                <button
                  className='sb-btn sb-btn--danger'
                  onClick={() => setConfirmNuke(selectedSandbox.sandboxId)}
                  disabled={!snapshot?.dockerAvailable || !selectedSandbox.canReap}
                >
                  <FaTrash />
                  <span>Nuke</span>
                </button>
              )}
              <button className='sb-btn sb-btn--ghost sb-btn--icon' onClick={() => { setPanelOpen(false); setConfirmNuke('') }}>
                <FaCircleXmark />
              </button>
            </div>
          </div>
        ) : null}
      >
        {selectedSandbox && (
          <div className='sb-panel-body'>
            {/* Pressure bars */}
            <div className='sb-pressure-section'>
              <PressureBar
                label='CPU'
                value={`${selectedSandbox.resourceUsage.cpuPercent.toFixed(1)}%`}
                percent={Math.min(selectedSandbox.resourceUsage.cpuPercent, 100)}
              />
              <PressureBar
                label='Memory'
                value={formatBytes(selectedSandbox.resourceUsage.memoryBytes)}
                percent={selectedSandbox.resourceUsage.memoryPercent}
              />
            </div>

            {/* Facts */}
            <dl className='sb-facts'>
              <div><dt>Profile</dt><dd>{selectedSandbox.profile}</dd></div>
              <div><dt>Owner</dt><dd>{selectedSandbox.owner}</dd></div>
              <div><dt>Orchestrator PID</dt><dd>{selectedSandbox.orchestratorPid || 'Unknown'}</dd></div>
              <div><dt>Orchestrator state</dt><dd>{selectedSandbox.orchestratorState}</dd></div>
              <div><dt>Started</dt><dd>{selectedSandbox.startedAt ? new Date(selectedSandbox.startedAt).toLocaleString() : 'Unknown'}</dd></div>
              <div><dt>Heartbeat</dt><dd>{selectedSandbox.lastHeartbeatAt ? new Date(selectedSandbox.lastHeartbeatAt).toLocaleString() : 'No heartbeat'}</dd></div>
            </dl>

            {/* Warnings */}
            {selectedSandbox.warnings.length > 0 && (
              <div className='sb-panel-warnings'>
                <div className='sb-panel-warnings__title'><FaTriangleExclamation /> Warnings</div>
                <ul>
                  {selectedSandbox.warnings.map((w) => <li key={w}>{w}</li>)}
                </ul>
              </div>
            )}

            {/* Resource tabs */}
            <div className='sb-tabs'>
              <button type='button' className={`sb-tab${activeTab === 'containers' ? ' sb-tab--active' : ''}`} onClick={() => setActiveTab('containers')}>
                <FaDocker /> Containers <span className='sb-tab__count'>{selectedSandbox.containers.length}</span>
              </button>
              <button type='button' className={`sb-tab${activeTab === 'networks' ? ' sb-tab--active' : ''}`} onClick={() => setActiveTab('networks')}>
                <FaBoxArchive /> Networks <span className='sb-tab__count'>{selectedSandbox.networks.length}</span>
              </button>
              <button type='button' className={`sb-tab${activeTab === 'volumes' ? ' sb-tab--active' : ''}`} onClick={() => setActiveTab('volumes')}>
                <FaHardDrive /> Volumes <span className='sb-tab__count'>{selectedSandbox.volumes.length}</span>
              </button>
            </div>

            {activeTab === 'containers' && (
              <div className='sb-resource-list'>
                {selectedSandbox.containers.length === 0 && <p className='sb-empty-tab'>No containers.</p>}
                {selectedSandbox.containers.map((container) => (
                  <div key={container.id} className='sb-container-row'>
                    <span className={`sb-dot sb-dot--${statusTone(container.state)}`} />
                    <div className='sb-container-row__info'>
                      <strong>{container.name}</strong>
                      <span className='sb-container-row__image'>{container.image}</span>
                    </div>
                    <div className='sb-container-row__ports'>
                      {container.ports.length > 0
                        ? container.ports.map((port) => <span key={port} className='sb-port-chip'>{port}</span>)
                        : <span className='sb-port-chip sb-port-chip--none'>no ports</span>
                      }
                    </div>
                  </div>
                ))}
              </div>
            )}

            {activeTab === 'networks' && (
              <div className='sb-resource-list'>
                {selectedSandbox.networks.length === 0 && <p className='sb-empty-tab'>No networks.</p>}
                {selectedSandbox.networks.map((network) => (
                  <div key={network.id} className='sb-network-row'>
                    <FaBoxArchive className='sb-network-row__icon' />
                    <div className='sb-network-row__info'>
                      <strong>{network.name}</strong>
                      <span>{network.driver} · {network.scope} scope</span>
                    </div>
                  </div>
                ))}
              </div>
            )}

            {activeTab === 'volumes' && (
              <div className='sb-resource-list'>
                {selectedSandbox.volumes.length === 0 && <p className='sb-empty-tab'>No volumes.</p>}
                {selectedSandbox.volumes.map((volume) => (
                  <div key={volume.name} className='sb-volume-row'>
                    <FaHardDrive className='sb-volume-row__icon' />
                    <div className='sb-volume-row__info'>
                      <strong>{volume.name}</strong>
                      <span className='sb-volume-row__mount'>{volume.mountpoint}</span>
                    </div>
                    <span className='sb-chip'>{volume.driver}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </SlidingPanel>
    </AppShell>
  )
}

function PressureBar(props: { label: string; value: string; percent: number }) {
  const tone = props.percent >= 85 ? 'danger' : props.percent >= 60 ? 'warn' : 'ok'
  return (
    <div className='sb-pressure'>
      <div className='sb-pressure__head'>
        <span>{props.label}</span>
        <strong>{props.value}</strong>
      </div>
      <div className='sb-pressure__track'>
        <div
          className={`sb-pressure__fill sb-pressure__fill--${tone}`}
          style={{ width: props.percent <= 0 ? '0%' : `${Math.max(4, Math.min(props.percent, 100))}%` }}
        />
      </div>
    </div>
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
