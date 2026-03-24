import { useEffect, useRef, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import Layout from '../components/Layout'
import Page   from '../components/Page'

const API = 'http://localhost:8090'

interface Run {
  run_id:      string
  package_id:  string
  image_ref:   string
  status:      string
  started_at:  string | null
  finished_at: string | null
  created_at:  string
}

interface Step {
  step_id:     string
  run_id:      string
  name:        string
  position:    number
  status:      string
  exit_code:   number
  error:       string
  started_at:  string | null
  finished_at: string | null
}

interface LogEntry {
  log_id:  string
  step_id: string
  line:    number
  data:    string
  time:    number
  type:    number  // 0=stdout 1=stderr 2=exitcode
}

interface LogLine {
  line:   number
  text:   string
  time:   number
  stderr: boolean
}

const STATUS_COLOR: Record<string, string> = {
  pending:  '#818d94',
  running:  '#4da6ff',
  success:  '#4caf50',
  failure:  '#e57373',
  canceled: '#888',
  error:    '#ff9800',
}

export default function RunDetail() {
  const { id } = useParams<{ id: string }>()
  const nav     = useNavigate()
  const token   = localStorage.getItem('token') || ''

  const [run,          setRun]          = useState<Run | null>(null)
  const [steps,        setSteps]        = useState<Step[]>([])
  const [selectedStep, setSelectedStep] = useState<string | null>(null)
  const [lines,        setLines]        = useState<LogLine[]>([])
  const [streaming,    setStreaming]     = useState(false)
  const [autoScroll,   setAutoScroll]   = useState(true)
  const [err,          setErr]          = useState('')

  const logBoxRef  = useRef<HTMLDivElement>(null)
  const esRef      = useRef<EventSource | null>(null)
  const linesRef   = useRef<LogLine[]>([])

  // Load run + steps; pick initial selected step once (first main step, else first step)
  useEffect(() => {
    if (!id) return
    const headers = { Authorization: `Bearer ${token}` }

    Promise.all([
      fetch(`${API}/api/runs/${id}`, { headers }).then(r => r.json()),
      fetch(`${API}/api/runs/${id}/steps`, { headers }).then(r => r.json()),
    ]).then(([r, s]) => {
      setRun(r)
      const list: Step[] = Array.isArray(s) ? s : []
      setSteps(list)
      if (list.length > 0) {
        const main = list.find(st => st.position > 0)
        setSelectedStep((main ?? list[0]).step_id)
      }
    }).catch(() => setErr('Failed to load run'))
  }, [id])

  // Poll steps while run is active
  useEffect(() => {
    if (!run || isTerminal(run.status)) return
    const iv = setInterval(() => {
      fetch(`${API}/api/runs/${id}/steps`, { headers: { Authorization: `Bearer ${token}` } })
        .then(r => r.json())
        .then(s => { if (Array.isArray(s)) setSteps(s) })
      fetch(`${API}/api/runs/${id}`, { headers: { Authorization: `Bearer ${token}` } })
        .then(r => r.json())
        .then(r => setRun(r))
    }, 3000)
    return () => clearInterval(iv)
  }, [run?.status])

  // Stream logs via SSE when selected step changes
  useEffect(() => {
    if (!selectedStep || !id) return

    // Close previous stream
    if (esRef.current) {
      esRef.current.close()
      esRef.current = null
    }
    linesRef.current = []
    setLines([])
    setStreaming(true)

    const url = `${API}/api/runs/${id}/logs/${selectedStep}`
    const es = new EventSource(url + `?token=${encodeURIComponent(token)}`)
    esRef.current = es

    es.onmessage = (e) => {
      try {
        const entries: LogEntry[] = JSON.parse(e.data)
        const newLines = entries.map(entry => ({
          line:   entry.line,
          text:   entry.data,
          time:   entry.time,
          stderr: entry.type === 1,
        }))
        linesRef.current = [...linesRef.current, ...newLines]
        setLines([...linesRef.current])
      } catch { /* ignore parse errors */ }
    }

    es.addEventListener('eof', () => {
      setStreaming(false)
      es.close()
      esRef.current = null
    })

    es.onerror = () => {
      setStreaming(false)
      es.close()
      esRef.current = null
    }

    return () => {
      es.close()
      esRef.current = null
    }
  }, [selectedStep, id])

  // Auto-scroll
  useEffect(() => {
    if (!autoScroll || !logBoxRef.current) return
    logBoxRef.current.scrollTop = logBoxRef.current.scrollHeight
  }, [lines, autoScroll])

  const onScroll = () => {
    if (!logBoxRef.current) return
    const { scrollTop, scrollHeight, clientHeight } = logBoxRef.current
    setAutoScroll(scrollHeight - scrollTop - clientHeight < 40)
  }

  if (err) return (
    <Layout><Page title='Run'><div className='auth-error'>{err}</div></Page></Layout>
  )
  if (!run) return (
    <Layout><Page title='Run'><div className='empty-state'>Loading…</div></Page></Layout>
  )

  return (
    <Layout>
      <Page title={`Run ${run.run_id.slice(0, 8)}`}>
        {/* Run header */}
        <div className='white-box' style={{ marginBottom: 16, display: 'flex', gap: 24, alignItems: 'center', flexWrap: 'wrap' }}>
          <div>
            <div style={{ fontSize: 11, color: '#818d94', marginBottom: 2 }}>STATUS</div>
            <span style={{ color: STATUS_COLOR[run.status] || '#818d94', fontWeight: 600 }}>
              {run.status.toUpperCase()}
            </span>
          </div>
          <div>
            <div style={{ fontSize: 11, color: '#818d94', marginBottom: 2 }}>IMAGE</div>
            <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{run.image_ref || '—'}</span>
          </div>
          {run.started_at && (
            <div>
              <div style={{ fontSize: 11, color: '#818d94', marginBottom: 2 }}>STARTED</div>
              <span style={{ fontSize: 13 }}>{new Date(run.started_at).toLocaleString()}</span>
            </div>
          )}
          <button className='btn' style={{ marginLeft: 'auto' }} onClick={() => nav('/runs')}>← Back</button>
        </div>

        <div style={{ display: 'flex', gap: 16, height: 'calc(100vh - 260px)', minHeight: 400 }}>
          {/* Steps sidebar */}
          <div style={{ width: 220, flexShrink: 0, display: 'flex', flexDirection: 'column', gap: 4 }}>
            {steps.length === 0 ? (
              <div style={{ color: '#818d94', fontSize: 13, padding: 8 }}>
                {run.status === 'pending' ? 'Waiting for agent…' : 'No steps yet'}
              </div>
            ) : steps.map(s => (
              <div
                key={s.step_id}
                onClick={() => setSelectedStep(s.step_id)}
                style={{
                  padding: '8px 12px',
                  borderRadius: 6,
                  cursor: 'pointer',
                  background: selectedStep === s.step_id ? '#1e3a4a' : 'transparent',
                  border: `1px solid ${selectedStep === s.step_id ? '#2a4a5a' : 'transparent'}`,
                  display: 'flex', alignItems: 'center', gap: 8,
                }}
              >
                <span style={{
                  display: 'inline-block', width: 8, height: 8, borderRadius: '50%',
                  background: STATUS_COLOR[s.status] || '#818d94', flexShrink: 0,
                }} />
                <span style={{ fontSize: 13, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{s.name}</span>
              </div>
            ))}
          </div>

          {/* Log viewer */}
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0 }}>
            <div style={{
              display: 'flex', alignItems: 'center', gap: 12,
              padding: '6px 12px', background: '#0a1e2a',
              borderRadius: '6px 6px 0 0', borderBottom: '1px solid #1e3a4a',
              fontSize: 12, color: '#818d94',
            }}>
              <span>{streaming ? '● Streaming' : lines.length > 0 ? `${lines.length} lines` : 'No output'}</span>
              <label style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 6, cursor: 'pointer' }}>
                <input type='checkbox' checked={autoScroll} onChange={e => setAutoScroll(e.target.checked)} />
                Auto-scroll
              </label>
            </div>
            <div
              ref={logBoxRef}
              onScroll={onScroll}
              style={{
                flex: 1,
                overflow: 'auto',
                background: '#071520',
                borderRadius: '0 0 6px 6px',
                padding: '12px 0',
                fontFamily: "'Courier New', Courier, monospace",
                fontSize: 12,
                lineHeight: '1.6',
              }}
            >
              {lines.length === 0 ? (
                <div style={{ color: '#444', padding: '8px 16px' }}>
                  {streaming ? 'Waiting for output…' : 'No logs available'}
                </div>
              ) : lines.map((l, i) => (
                <div
                  key={i}
                  style={{ display: 'flex', padding: '0 16px', minHeight: 20 }}
                  id={`L${l.line}`}
                >
                  <span style={{ color: '#2a4a5a', userSelect: 'none', minWidth: 48, paddingRight: 16, textAlign: 'right' }}>
                    {l.line + 1}
                  </span>
                  <span style={{ color: l.stderr ? '#e57373' : '#c8d8e4', whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                    {l.text}
                  </span>
                </div>
              ))}
              {streaming && (
                <div style={{ display: 'flex', padding: '4px 16px', color: '#4da6ff' }}>
                  <span style={{ minWidth: 48 }} />
                  <span>▋</span>
                </div>
              )}
            </div>
          </div>
        </div>
      </Page>
    </Layout>
  )
}

function isTerminal(status: string): boolean {
  return ['success', 'failure', 'canceled', 'error'].includes(status)
}
