import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'

export default function SSOCallback() {
  const nav = useNavigate()

  useEffect(() => {
    const hash = window.location.hash.slice(1)
    const params = new URLSearchParams(hash)
    const token = params.get('token')
    const returnURL = params.get('return_url') || '/'

    if (!token) {
      nav('/login?sso_error=missing_token', { replace: true })
      return
    }

    // Fetch user info with the token
    fetch('http://localhost:8090/auth/me', {
      headers: { Authorization: `Bearer ${token}` },
    })
      .then(r => r.json())
      .then(user => {
        localStorage.setItem('token', token)
        localStorage.setItem('user', JSON.stringify(user))
        nav(returnURL, { replace: true })
      })
      .catch(() => nav('/login?sso_error=fetch_failed', { replace: true }))
  }, [nav])

  return (
    <div className='auth-page'>
      <div className='auth-card'>
        <p style={{ textAlign: 'center', color: 'var(--text-2)' }}>Signing you in…</p>
      </div>
    </div>
  )
}
