import { useEffect, useState } from 'react'
import Layout from '../components/Layout'
import Page   from '../components/Page'

const API = 'http://localhost:8090'

interface Agent {
  agent_id:     string
  name:         string
  platform:     string
  backend:      string
  capacity:     number
  version:      string
  labels:       Record<string, string> | null
  last_contact: string
  no_schedule:  boolean
  created_at:   string
}

function isOnline(lastContact: string): boolean {
  const diff = Date.now() - new Date(lastContact).getTime()
  return diff < 60_000
}

export default function Agents() {
  const [agents,  setAgents]  = useState<Agent[]>([])
  const [loading, setLoading] = useState(true)
  const [err,     setErr]     = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [newToken, setNewToken]     = useState('')
  const [form, setForm] = useState({ name: '', capacity: '1' })
  const [creating, setCreating] = useState(false)

  const token = localStorage.getItem('token') || ''

  const load = () => {
    setLoading(true)
    fetch(`${API}/api/agents`, { headers: { Authorization: `Bearer ${token}` } })
      .then(r => r.json())
      .then(data => { setAgents(Array.isArray(data) ? data : []); setLoading(false) })
      .catch(() => { setErr('Failed to load agents'); setLoading(false) })
  }

  useEffect(load, [])

  const createAgent = async (e: React.FormEvent) => {
    e.preventDefault()
    setCreating(true)
    try {
      const res = await fetch(`${API}/api/agents`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({ name: form.name, capacity: parseInt(form.capacity) || 1 }),
      })
      const data = await res.json()
      if (!res.ok) { setErr(data.error || 'Create failed'); return }
      setNewToken(data.token)
      setForm({ name: '', capacity: '1' })
      load()
    } finally {
      setCreating(false)
    }
  }

  const deleteAgent = async (id: string) => {
    if (!confirm('Delete this agent?')) return
    await fetch(`${API}/api/agents/${id}`, {
      method: 'DELETE',
      headers: { Authorization: `Bearer ${token}` },
    })
    load()
  }

  const toggleSchedule = async (a: Agent) => {
    await fetch(`${API}/api/agents/${a.agent_id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      body: JSON.stringify({ no_schedule: !a.no_schedule }),
    })
    load()
  }

  return (
    <Layout>
      <Page title='Agents'>
        <div className='page__toolbar'>
          <button className='btn btn--primary' onClick={() => { setShowCreate(s => !s); setNewToken('') }}>
            {showCreate ? 'Cancel' : '+ New Agent'}
          </button>
        </div>

        {err && <div className='auth-error' style={{ margin: '0 0 16px' }}>{err}</div>}

        {showCreate && (
          <div className='white-box' style={{ marginBottom: 24 }}>
            <h3 style={{ marginTop: 0, marginBottom: 16, fontSize: 14, fontWeight: 600 }}>Create agent</h3>
            {newToken ? (
              <div>
                <p style={{ fontSize: 13, marginBottom: 8 }}>
                  Agent created. Copy this token — it won't be shown again:
                </p>
                <code style={{
                  display: 'block', padding: '10px 14px', background: '#0a1e2a',
                  borderRadius: 6, fontSize: 12, wordBreak: 'break-all',
                  border: '1px solid #1e3a4a', color: '#fe733f',
                }}>
                  {newToken}
                </code>
                <button className='btn btn--primary' style={{ marginTop: 12 }} onClick={() => { setShowCreate(false); setNewToken('') }}>
                  Done
                </button>
              </div>
            ) : (
              <form onSubmit={createAgent} style={{ display: 'flex', gap: 12, alignItems: 'flex-end' }}>
                <div className='field' style={{ flex: 1, marginBottom: 0 }}>
                  <input className='field__input' type='text' placeholder=' '
                    value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))} />
                  <label className='field__label'>Agent name</label>
                </div>
                <div className='field' style={{ width: 100, marginBottom: 0 }}>
                  <input className='field__input' type='number' placeholder=' ' min={1} max={32}
                    value={form.capacity} onChange={e => setForm(f => ({ ...f, capacity: e.target.value }))} />
                  <label className='field__label'>Capacity</label>
                </div>
                <button className='btn btn--primary' type='submit' disabled={creating || !form.name}>
                  {creating ? 'Creating…' : 'Create'}
                </button>
              </form>
            )}
          </div>
        )}

        {loading ? (
          <div className='empty-state'>Loading…</div>
        ) : agents.length === 0 ? (
          <div className='empty-state'>
            <div className='empty-state__title'>No agents registered</div>
            <div className='empty-state__desc'>Create an agent and run the agent binary on your execution host.</div>
          </div>
        ) : (
          <div className='white-box' style={{ padding: 0 }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
              <thead>
                <tr style={{ borderBottom: '1px solid #1e3a4a' }}>
                  {['Status', 'Name', 'Platform', 'Backend', 'Capacity', 'Version', 'Last contact', ''].map(h => (
                    <th key={h} style={{ padding: '10px 16px', textAlign: 'left', color: '#818d94', fontWeight: 500 }}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {agents.map(a => {
                  const online = isOnline(a.last_contact)
                  return (
                    <tr key={a.agent_id} style={{ borderBottom: '1px solid #1e3a4a' }}>
                      <td style={{ padding: '10px 16px' }}>
                        <span style={{
                          display: 'inline-block', width: 8, height: 8, borderRadius: '50%',
                          background: online ? '#4caf50' : '#555',
                          marginRight: 6,
                        }} />
                        {online ? 'Online' : 'Offline'}
                      </td>
                      <td style={{ padding: '10px 16px', fontWeight: 500 }}>{a.name}</td>
                      <td style={{ padding: '10px 16px', color: '#818d94' }}>{a.platform || '—'}</td>
                      <td style={{ padding: '10px 16px', color: '#818d94' }}>{a.backend  || '—'}</td>
                      <td style={{ padding: '10px 16px', color: '#818d94' }}>{a.capacity}</td>
                      <td style={{ padding: '10px 16px', color: '#818d94' }}>{a.version  || '—'}</td>
                      <td style={{ padding: '10px 16px', color: '#818d94' }}>
                        {a.last_contact ? new Date(a.last_contact).toLocaleString() : '—'}
                      </td>
                      <td style={{ padding: '10px 16px', display: 'flex', gap: 8 }}>
                        <button
                          className='btn'
                          style={{ fontSize: 12, padding: '4px 10px', opacity: 0.8 }}
                          onClick={() => toggleSchedule(a)}
                          title={a.no_schedule ? 'Enable scheduling' : 'Pause scheduling'}
                        >
                          {a.no_schedule ? 'Resume' : 'Pause'}
                        </button>
                        <button
                          className='btn'
                          style={{ fontSize: 12, padding: '4px 10px', color: '#e57373' }}
                          onClick={() => deleteAgent(a.agent_id)}
                        >
                          Delete
                        </button>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </Page>
    </Layout>
  )
}
