import { useNavigate } from 'react-router-dom'

export default function Home() {
  const nav = useNavigate()
  const raw  = localStorage.getItem('user')
  const user = raw ? JSON.parse(raw) : null

  const logout = () => {
    localStorage.clear()
    nav('/login')
  }

  return (
    <div style={{ minHeight: '100vh', background: '#f0f2f5', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <div style={{ background: '#fff', borderRadius: 12, boxShadow: '0 4px 24px rgba(0,0,0,.10)', padding: '40px 48px', textAlign: 'center' }}>
        <h2 style={{ fontSize: 22, fontWeight: 700, marginBottom: 8 }}>Welcome, {user?.name || user?.username} 👋</h2>
        <p style={{ color: '#666', marginBottom: 24 }}>@{user?.username}</p>
        <button
          onClick={logout}
          style={{ background: '#1677ff', color: '#fff', border: 'none', borderRadius: 8, padding: '10px 28px', fontSize: 14, fontWeight: 500, cursor: 'pointer' }}
        >
          Sign out
        </button>
      </div>
    </div>
  )
}
