import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'

const API = 'http://localhost:8090'

interface SSOProvider { provider_id: string; name: string }

export default function Login() {
  const nav = useNavigate()
  const [form, setForm]             = useState({ username: '', password: '' })
  const [show, setShow]             = useState(false)
  const [err, setErr]               = useState('')
  const [loading, setLoading]       = useState(false)
  const [ssoProviders, setSSOProviders] = useState<SSOProvider[]>([])

  useEffect(() => {
    fetch(`${API}/auth/sso/providers`)
      .then(r => r.ok ? r.json() : [])
      .then(data => setSSOProviders(Array.isArray(data) ? data : []))
      .catch(() => {})

    const params = new URLSearchParams(window.location.search)
    const ssoErr = params.get('sso_error')
    if (ssoErr) setErr(`SSO error: ${ssoErr}`)
  }, [])

  const set = (k: string) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm(f => ({ ...f, [k]: e.target.value }))

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setErr('')
    setLoading(true)
    try {
      const res = await fetch(`${API}/auth/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(form),
      })
      const data = await res.json()
      if (!res.ok) { setErr(data.error || 'Login failed'); return }
      localStorage.setItem('token', data.token)
      localStorage.setItem('user',  JSON.stringify(data.user))
      nav('/runs')
    } catch {
      setErr('Cannot reach server')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className='auth-page'>
      <div className='auth-card'>
        <h1 className='auth-card__title'>Sign in</h1>

        {err && <div className='auth-error'>{err}</div>}

        <form onSubmit={submit} noValidate>
          <div className='field'>
            <input
              className='field__input'
              type='text'
              placeholder=' '
              value={form.username}
              onChange={set('username')}
              autoComplete='username'
            />
            <label className='field__label'>Username or email</label>
          </div>

          <div className='field'>
            <input
              className='field__input'
              type={show ? 'text' : 'password'}
              placeholder=' '
              value={form.password}
              onChange={set('password')}
              autoComplete='current-password'
            />
            <label className='field__label'>Password</label>
            <span className='field__icon' onClick={() => setShow(s => !s)}>
              {show ? <EyeOff /> : <Eye />}
            </span>
          </div>

          <button className='btn btn--primary' type='submit' disabled={loading} style={{ marginTop: 8 }}>
            {loading ? 'Signing in…' : 'Sign In'}
          </button>
        </form>

        <div className='auth-switch'>
          Don't have an account? <Link to='/signup'>Sign Up</Link>
        </div>

        {ssoProviders.length > 0 && (
          <>
            <div className='divider'>or</div>
            {ssoProviders.map(p => (
              <a
                key={p.provider_id}
                className='btn btn--google'
                href={`${API}/auth/sso/login?provider_id=${p.provider_id}&return_url=/runs`}
                style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 8, textDecoration: 'none', marginTop: 8 }}
              >
                Sign in with {p.name}
              </a>
            ))}
          </>
        )}
      </div>
    </div>
  )
}

function Eye() {
  return (
    <svg width='18' height='18' viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'>
      <path d='M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z'/>
      <circle cx='12' cy='12' r='3'/>
    </svg>
  )
}

function EyeOff() {
  return (
    <svg width='18' height='18' viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'>
      <path d='M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19m-6.72-1.07a3 3 0 11-4.24-4.24'/>
      <line x1='1' y1='1' x2='23' y2='23'/>
    </svg>
  )
}
