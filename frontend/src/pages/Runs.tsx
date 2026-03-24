import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { FaPlay } from 'react-icons/fa6'
import Layout from '../components/Layout'
import Page   from '../components/Page'

const API = 'http://localhost:8090'

interface Run {
  run_id:      string
  package_id:  string
  image_ref:   string
  agent_id:    string
  status:      string
  started_at:  string | null
  finished_at: string | null
  created_at:  string
}

const STATUS_COLOR: Record<string, string> = {
  pending:  '#818d94',
  running:  '#4da6ff',
  success:  '#4caf50',
  failure:  '#e57373',
  canceled: '#888',
  error:    '#ff9800',
}

export default function Runs() {
  const nav = useNavigate()
  const [runs, setRuns]       = useState<Run[]>([])
  const [total, setTotal]     = useState(0)
  const [page, setPage]       = useState(1)
  const [loading,       setLoading]       = useState(true)
  const [demoRunning,   setDemoRunning]   = useState(false)
  const token = localStorage.getItem('token') || ''

  const load = (p = page) => {
    setLoading(true)
    fetch(`${API}/api/runs?page=${p}`, { headers: { Authorization: `Bearer ${token}` } })
      .then(r => r.json())
      .then(d => { setRuns(d.runs ?? []); setTotal(d.total ?? 0); setLoading(false) })
      .catch(() => setLoading(false))
  }

  useEffect(() => { load(page) }, [page])

  const startDemo = async () => {
    setDemoRunning(true)
    try {
      const res = await fetch(`${API}/api/demo/run`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
      })
      const data = await res.json()
      if (res.ok) nav(`/runs/${data.run_id}`)
    } finally {
      setDemoRunning(false)
    }
  }

  return (
    <Layout>
      <Page title='Runs'>
        <div className='page__toolbar'>
          <button className='btn btn--primary' onClick={startDemo} disabled={demoRunning}>
            <FaPlay style={{ marginRight: 6 }} />
            {demoRunning ? 'Starting…' : 'Run Demo Suite'}
          </button>
        </div>
        {loading ? (
          <div className='empty-state'>Loading…</div>
        ) : runs.length === 0 ? (
          <div className='empty-state'>
            <div className='empty-state__title'>No runs yet</div>
            <div className='empty-state__desc'>Trigger a suite run from the Catalog page.</div>
          </div>
        ) : (
          <>
            <div className='white-box' style={{ padding: 0 }}>
              <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
                <thead>
                  <tr style={{ borderBottom: '1px solid #1e3a4a' }}>
                    {['Status', 'Package', 'Image', 'Agent', 'Started', 'Duration', ''].map(h => (
                      <th key={h} style={{ padding: '10px 16px', textAlign: 'left', color: '#818d94', fontWeight: 500 }}>{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {runs.map(r => {
                    const duration = r.started_at && r.finished_at
                      ? formatDuration(new Date(r.started_at), new Date(r.finished_at))
                      : r.started_at ? 'Running…' : '—'
                    return (
                      <tr
                        key={r.run_id}
                        style={{ borderBottom: '1px solid #1e3a4a', cursor: 'pointer' }}
                        onClick={() => nav(`/runs/${r.run_id}`)}
                      >
                        <td style={{ padding: '10px 16px' }}>
                          <span style={{
                            display: 'inline-flex', alignItems: 'center', gap: 6,
                            color: STATUS_COLOR[r.status] || '#818d94',
                            fontWeight: 500,
                          }}>
                            <StatusDot status={r.status} />
                            {r.status}
                          </span>
                        </td>
                        <td style={{ padding: '10px 16px', fontFamily: 'monospace', fontSize: 12 }}>{r.package_id.slice(0, 8)}…</td>
                        <td style={{ padding: '10px 16px', color: '#818d94', fontFamily: 'monospace', fontSize: 12, maxWidth: 200, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{r.image_ref || '—'}</td>
                        <td style={{ padding: '10px 16px', color: '#818d94', fontFamily: 'monospace', fontSize: 12 }}>{r.agent_id ? r.agent_id.slice(0, 8) + '…' : '—'}</td>
                        <td style={{ padding: '10px 16px', color: '#818d94' }}>{r.started_at ? new Date(r.started_at).toLocaleString() : '—'}</td>
                        <td style={{ padding: '10px 16px', color: '#818d94' }}>{duration}</td>
                        <td style={{ padding: '10px 16px' }}>
                          <span style={{ color: '#4da6ff', fontSize: 12 }}>View logs →</span>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>

            {total > 20 && (
              <div style={{ display: 'flex', gap: 8, marginTop: 16, justifyContent: 'flex-end' }}>
                <button className='btn' disabled={page === 1} onClick={() => setPage(p => p - 1)}>Prev</button>
                <span style={{ lineHeight: '32px', color: '#818d94', fontSize: 13 }}>Page {page}</span>
                <button className='btn' disabled={page * 20 >= total} onClick={() => setPage(p => p + 1)}>Next</button>
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
  const pulse = status === 'running'
  return (
    <span style={{
      display: 'inline-block', width: 8, height: 8, borderRadius: '50%',
      background: color,
      boxShadow: pulse ? `0 0 0 2px ${color}44` : 'none',
    }} />
  )
}

function formatDuration(start: Date, end: Date): string {
  const s = Math.floor((end.getTime() - start.getTime()) / 1000)
  if (s < 60)  return `${s}s`
  if (s < 3600) return `${Math.floor(s / 60)}m ${s % 60}s`
  return `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`
}
