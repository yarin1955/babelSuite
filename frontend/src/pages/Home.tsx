import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
    FaArrowTrendUp,
    FaBoxOpen,
    FaGear,
    FaLayerGroup,
    FaPlay,
    FaRobot,
    FaSliders,
    FaWandMagicSparkles,
} from 'react-icons/fa6'
import Layout from '../components/Layout'
import Page from '../components/Page'
import styles from './Home.module.css'

const API = 'http://localhost:8090'

interface Run {
    run_id: string
    package_id: string
    image_ref: string
    agent_id: string
    status: string
    started_at: string | null
    finished_at: string | null
    created_at: string
}

interface CatalogPackage {
    package_id: string
    display_name: string
    name: string
    description: string
    registry_kind: string
    version: string
}

interface Agent {
    agent_id: string
    name: string
    last_contact: string
    no_schedule: boolean
}

const STATUS_TONE: Record<string, string> = {
    pending: 'Pending',
    running: 'Running',
    success: 'Healthy',
    failure: 'Needs attention',
    canceled: 'Canceled',
    error: 'Errored',
}

function isOnline(lastContact: string): boolean {
    return Date.now() - new Date(lastContact).getTime() < 60_000
}

function timeAgo(iso: string): string {
    const diff = Math.floor((Date.now() - new Date(iso).getTime()) / 1000)
    if (diff < 60) return `${diff}s ago`
    if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
    if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`
    return `${Math.floor(diff / 86400)}d ago`
}

export default function Home() {
    const nav = useNavigate()
    const token = localStorage.getItem('token') || ''
    const isAdmin = (() => {
        try {
            return JSON.parse(localStorage.getItem('user') || '{}').is_admin === true
        } catch {
            return false
        }
    })()
    const [runs, setRuns] = useState<Run[]>([])
    const [packages, setPackages] = useState<CatalogPackage[]>([])
    const [agents, setAgents] = useState<Agent[]>([])
    const [loading, setLoading] = useState(true)

    useEffect(() => {
        const headers = { Authorization: `Bearer ${token}` }
        const agentRequest = isAdmin
            ? fetch(`${API}/api/agents`, { headers }).then(r => r.ok ? r.json() : [])
            : Promise.resolve([])

        Promise.all([
            fetch(`${API}/api/runs?page=1`, { headers }).then(r => r.ok ? r.json() : { runs: [] }),
            fetch(`${API}/api/catalog?page=1&page_size=6`, { headers }).then(r => r.ok ? r.json() : { packages: [] }),
            agentRequest,
        ])
            .then(([runsData, catalogData, agentsData]) => {
                setRuns(Array.isArray(runsData.runs) ? runsData.runs.slice(0, 6) : [])
                setPackages(Array.isArray(catalogData.packages) ? catalogData.packages.slice(0, 4) : [])
                setAgents(Array.isArray(agentsData) ? agentsData.slice(0, 4) : [])
            })
            .finally(() => setLoading(false))
    }, [isAdmin, token])

    const metrics = useMemo(() => {
        const running = runs.filter(run => run.status === 'running').length
        const successful = runs.filter(run => run.status === 'success').length
        const online = agents.filter(agent => isOnline(agent.last_contact)).length
        return { running, successful, online }
    }, [runs, agents])

    return (
        <Layout>
            <Page
                title='Overview'
                toolbar={
                    <button className='app-button app-button--primary' onClick={() => nav('/runs')}>
                        <FaPlay />
                        Open Runs
                    </button>
                }
            >
                <section className={styles.hero}>
                    <div className={styles.heroCopy}>
                        <div className={styles.eyebrow}>
                            <FaWandMagicSparkles />
                            BabelSuite orchestration
                        </div>
                        <h1>Run containerized validation suites with a control room built for fast feedback.</h1>
                        <p>
                            Compose infra, contracts, mocks, and test runners into one executable suite, then watch status,
                            agents, and package readiness from a single surface.
                        </p>
                        <div className={styles.heroActions}>
                            <button className='app-button app-button--primary' onClick={() => nav('/suites')}>
                                <FaBoxOpen />
                                Browse suites
                            </button>
                            {isAdmin ? (
                                <button className='app-button app-button--secondary' onClick={() => nav('/settings')}>
                                    <FaGear />
                                    Workspace settings
                                </button>
                            ) : (
                                <button className='app-button app-button--secondary' onClick={() => nav('/profiles')}>
                                    <FaSliders />
                                    Open profiles
                                </button>
                            )}
                        </div>
                    </div>

                    <div className={styles.signalCard}>
                        <div className={styles.signalLabel}>Current signal</div>
                        <div className={styles.signalValue}>{runs[0] ? STATUS_TONE[runs[0].status] ?? runs[0].status : 'Idle'}</div>
                        <div className={styles.signalMeta}>
                            {runs[0]
                                ? `Latest run ${runs[0].run_id.slice(0, 8)} created ${new Date(runs[0].created_at).toLocaleString()}`
                                : 'No suite activity yet. Start with an enabled suite.'}
                        </div>
                    </div>
                </section>

                <section className={styles.metrics}>
                    <MetricCard icon={<FaPlay />} label='Active runs' value={String(metrics.running)} hint='Suites currently executing' />
                    <MetricCard icon={<FaArrowTrendUp />} label='Recent passes' value={String(metrics.successful)} hint='Successful runs in the latest page' />
                    {isAdmin
                        ? <MetricCard icon={<FaRobot />} label='Agents online' value={`${metrics.online}/${agents.length}`} hint='Reachable execution workers' />
                        : <MetricCard icon={<FaSliders />} label='Shared profiles' value='Team' hint='Run inputs can come from shared profile presets.' />}
                    <MetricCard icon={<FaLayerGroup />} label='Suites ready' value={String(packages.length)} hint='Enabled suites loaded for launch' />
                </section>

                <section className={styles.grid}>
                    <div className='panel-card'>
                        <div className='panel-card__header'>
                            <div>
                                <div className='panel-card__eyebrow'>Recent runs</div>
                                <h3>Execution timeline</h3>
                            </div>
                            <button className='app-link-button' onClick={() => nav('/runs')}>View all</button>
                        </div>
                        {loading ? (
                            <div className='app-empty-state'>Loading activity...</div>
                        ) : runs.length === 0 ? (
                            <div className='app-empty-state'>No runs yet.</div>
                        ) : (
                            <div className={styles.runList}>
                                {runs.map(run => (
                                    <button key={run.run_id} className={styles.runItem} onClick={() => nav(`/runs/${run.run_id}`)}>
                                        <span className={`status-pill status-pill--${run.status}`}>{run.status}</span>
                                        <div className={styles.runBody}>
                                            <strong>{run.package_id.slice(0, 8)}</strong>
                                            <span>{run.image_ref || 'Image pending'}</span>
                                        </div>
                                        <span className={styles.runTimestamp}>{new Date(run.created_at).toLocaleDateString()}</span>
                                    </button>
                                ))}
                            </div>
                        )}
                    </div>

                    <div className='panel-card'>
                        <div className='panel-card__header'>
                            <div>
                                <div className='panel-card__eyebrow'>Suites</div>
                                <h3>Launch-ready suites</h3>
                            </div>
                            <button className='app-link-button' onClick={() => nav('/suites')}>Open suites</button>
                        </div>
                        {loading ? (
                            <div className='app-empty-state'>Loading suites...</div>
                        ) : packages.length === 0 ? (
                            <div className='app-empty-state'>No enabled suites available.</div>
                        ) : (
                            <div className={styles.packageList}>
                                {packages.map(pkg => (
                                    <div key={pkg.package_id} className={styles.packageItem}>
                                        <div>
                                            <strong>{pkg.display_name || pkg.name}</strong>
                                            <p>{pkg.description || 'Containerized suite package ready for orchestration.'}</p>
                                        </div>
                                        <span className='app-chip'>{pkg.registry_kind || 'registry'}</span>
                                    </div>
                                ))}
                            </div>
                        )}
                    </div>

                    {isAdmin ? (
                        <div className='panel-card'>
                            <div className='panel-card__header'>
                                <div>
                                    <div className='panel-card__eyebrow'>Agents</div>
                                    <h3>Worker heartbeat</h3>
                                </div>
                                <button className='app-link-button' onClick={() => nav('/settings/agents')}>Manage</button>
                            </div>
                            {loading ? (
                                <div className='app-empty-state'>Loading agents...</div>
                            ) : agents.length === 0 ? (
                                <div className='app-empty-state'>No agents registered.</div>
                            ) : (
                                <div className={styles.agentList}>
                                    {agents.map(agent => {
                                        const online = isOnline(agent.last_contact)
                                        return (
                                            <div key={agent.agent_id} className={styles.agentItem}>
                                                <div className={styles.agentIdentity}>
                                                    <span className={`${styles.agentDot} ${online ? styles.agentDotOnline : styles.agentDotOffline}`} />
                                                    <div>
                                                        <strong>{agent.name}</strong>
                                                        <p>{agent.no_schedule ? 'Paused for scheduling' : 'Accepting work'}</p>
                                                    </div>
                                                </div>
                                                <span className='app-chip'>{agent.last_contact ? timeAgo(agent.last_contact) : 'never'}</span>
                                            </div>
                                        )
                                    })}
                                </div>
                            )}
                        </div>
                    ) : (
                        <div className='panel-card'>
                            <div className='panel-card__header'>
                                <div>
                                    <div className='panel-card__eyebrow'>Profiles</div>
                                    <h3>Shared run inputs</h3>
                                </div>
                                <button className='app-link-button' onClick={() => nav('/profiles')}>Open</button>
                            </div>
                            <div className='app-empty-state'>
                                Shared profiles let teams reuse the same environment inputs across suite launches without passing files around.
                            </div>
                        </div>
                    )}
                </section>
            </Page>
        </Layout>
    )
}

function MetricCard({ icon, label, value, hint }: { icon: React.ReactNode; label: string; value: string; hint: string }) {
    return (
        <div className={styles.metricCard}>
            <div className={styles.metricIcon}>{icon}</div>
            <div>
                <div className={styles.metricLabel}>{label}</div>
                <div className={styles.metricValue}>{value}</div>
                <div className={styles.metricHint}>{hint}</div>
            </div>
        </div>
    )
}
