import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { FaArrowRight } from 'react-icons/fa6'
import Layout from '../components/Layout'
import Page from '../components/Page'
import styles from './Runs.module.css'

const API = 'http://localhost:8090'

interface Run {
  run_id: string
  package_id: string
  image_ref: string
  profile: string
  agent_id: string
  status: string
  started_at: string | null
  finished_at: string | null
  created_at: string
}

const STATUS_COLOR: Record<string, string> = {
  pending: '#818d94',
  running: '#4da6ff',
  success: '#4caf50',
  failure: '#e57373',
  canceled: '#888',
  error: '#ff9800',
}

export default function Runs() {
  const nav = useNavigate()
  const [runs, setRuns] = useState<Run[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const token = localStorage.getItem('token') || ''

  const runningCount = runs.filter(run => run.status === 'running').length
  const successCount = runs.filter(run => run.status === 'success').length
  const failureCount = runs.filter(run => ['failure', 'error'].includes(run.status)).length

  const load = (p = page) => {
    setLoading(true)
    fetch(`${API}/api/runs?page=${p}`, { headers: { Authorization: `Bearer ${token}` } })
      .then(r => r.json())
      .then(d => {
        setRuns(d.runs ?? [])
        setTotal(d.total ?? 0)
        setLoading(false)
      })
      .catch(() => setLoading(false))
  }

  useEffect(() => { load(page) }, [page])

  return (
    <Layout>
      <Page title='Runs'>
        <section className={styles.summary}>
          <div className={styles.summaryCard}>
            <span className={styles.summaryLabel}>Recent activity</span>
            <strong>{runs[0] ? runs[0].status : 'idle'}</strong>
            <p>
              {runs[0]
                ? `Latest suite ${runs[0].run_id.slice(0, 8)} was created at ${new Date(runs[0].created_at).toLocaleString()}.`
                : 'Launch an enabled suite to start building execution history.'}
            </p>
          </div>

          <div className={styles.statStrip}>
            <Stat label='Running' value={String(runningCount)} tone='running' />
            <Stat label='Successful' value={String(successCount)} tone='success' />
            <Stat label='Needs attention' value={String(failureCount)} tone='failure' />
          </div>
        </section>

        {loading ? (
          <div className='app-empty-state'>Loading runs...</div>
        ) : runs.length === 0 ? (
          <div className='panel-card'>
            <div className='app-empty-state'>
              <div className='app-empty-state__title'>No runs yet</div>
              <div className='app-empty-state__desc'>Open the suites page, pick an enabled suite, and launch a run with the profile you want.</div>
            </div>
          </div>
        ) : (
          <>
            <div className='panel-card panel-card--flush'>
              <div className='panel-card__header panel-card__header--padded'>
                <div>
                  <div className='panel-card__eyebrow'>Execution history</div>
                  <h3>Latest runs</h3>
                </div>
              </div>
              <table className={styles.table}>
                <thead>
                  <tr>
                    {['Status', 'Package', 'Profile', 'Image', 'Agent', 'Started', 'Duration', ''].map(h => (
                      <th key={h}>{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {runs.map(r => {
                    const duration = r.started_at && r.finished_at
                      ? formatDuration(new Date(r.started_at), new Date(r.finished_at))
                      : r.started_at ? 'Running...' : '-'

                    return (
                      <tr key={r.run_id} className={styles.row} onClick={() => nav(`/runs/${r.run_id}`)}>
                        <td>
                          <span className={styles.status}>
                            <StatusDot status={r.status} />
                            {r.status}
                          </span>
                        </td>
                        <td className={styles.mono}>{r.package_id.slice(0, 8)}...</td>
                        <td className={styles.muted}>{r.profile || 'default'}</td>
                        <td className={styles.monoMuted}>{r.image_ref || '-'}</td>
                        <td className={styles.monoMuted}>{r.agent_id ? r.agent_id.slice(0, 8) + '...' : '-'}</td>
                        <td className={styles.muted}>{r.started_at ? new Date(r.started_at).toLocaleString() : '-'}</td>
                        <td className={styles.muted}>{duration}</td>
                        <td>
                          <span className={styles.linkCell}>
                            View logs <FaArrowRight />
                          </span>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>

            {total > 20 && (
              <div className={styles.pagination}>
                <button className='app-button app-button--secondary' disabled={page === 1} onClick={() => setPage(p => p - 1)}>Prev</button>
                <span>Page {page}</span>
                <button className='app-button app-button--secondary' disabled={page * 20 >= total} onClick={() => setPage(p => p + 1)}>Next</button>
              </div>
            )}
          </>
        )}
      </Page>
    </Layout>
  )
}

function StatusDot({ status }: { status: string }) {
  const color = STATUS_COLOR[status] || '#818d94'
  return (
    <span style={{
      display: 'inline-block',
      width: 8,
      height: 8,
      borderRadius: '50%',
      background: color,
      boxShadow: status === 'running' ? `0 0 0 2px ${color}44` : 'none',
    }} />
  )
}

function Stat({ label, value, tone }: { label: string; value: string; tone: 'running' | 'success' | 'failure' }) {
  return (
    <div className={styles.stat}>
      <span className={styles.statLabel}>{label}</span>
      <strong className={`${styles.statValue} ${styles[`statValue--${tone}`]}`}>{value}</strong>
    </div>
  )
}

function formatDuration(start: Date, end: Date): string {
  const s = Math.floor((end.getTime() - start.getTime()) / 1000)
  if (s < 60) return `${s}s`
  if (s < 3600) return `${Math.floor(s / 60)}m ${s % 60}s`
  return `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`
}
